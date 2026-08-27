package workflow

import (
	"context"
	"sort"

	"mastergate/internal/domain"
	"mastergate/internal/store"
)

func validateMeasurementScope(data *store.CaseData, m domain.MeasurementSet) error {
	if m.ScopeType == domain.ScopeProgram {
		if m.ScopeID != data.Case.CaseID {
			return domain.NewError(domain.CodeInvalid, "节目整体测量的 scope_id 必须等于 case_id")
		}
		return nil
	}
	for _, segment := range data.Segments {
		if segment.SegmentID == m.ScopeID {
			return nil
		}
	}
	return domain.NewError(domain.CodeInvalid, "分段测量引用了不存在的分段")
}

func appendMeasurement(data *store.CaseData, m domain.MeasurementSet) error {
	if err := domain.ValidateMeasurement(m); err != nil {
		return err
	}
	if err := validateMeasurementScope(data, m); err != nil {
		return err
	}
	for _, existing := range data.Measurements {
		if existing.MeasurementID == m.MeasurementID {
			return domain.NewError(domain.CodeConflict, "测量标识已存在")
		}
	}
	latest := store.LatestMeasurements(data)
	var current *domain.MeasurementSet
	for i := range latest {
		if latest[i].ScopeType == m.ScopeType && latest[i].ScopeID == m.ScopeID {
			current = &latest[i]
			break
		}
	}
	if current == nil && m.SupersedesID != "" {
		return domain.NewError(domain.CodeInvalid, "首次测量不能声明 supersedes_id")
	}
	if current != nil && m.SupersedesID != current.MeasurementID {
		return domain.NewError(domain.CodeInvalid, "新测量必须覆盖该范围的当前最新测量")
	}
	data.Measurements = append(data.Measurements, m)
	return nil
}

func (s *Service) SubmitMeasurement(ctx context.Context, c SubmitMeasurementCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "measurement.submitted", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if data.Case.State != domain.StateBaseline && data.Case.State != domain.StateMeasuring {
			return nil, domain.NewError(domain.CodeState, "当前状态不能提交初始测量")
		}
		c.Measurement.CaseID = c.CaseID
		c.Measurement.SubmittedBy = c.ActorID
		c.Measurement.SubmittedAt = s.now().UTC()
		if err := appendMeasurement(data, c.Measurement); err != nil {
			return nil, err
		}
		if err := data.Case.MarkMeasuring(); err != nil {
			return nil, err
		}
		return commandResult(data), nil
	})
}

func (s *Service) SubmitMeasurementBatch(ctx context.Context, c SubmitMeasurementBatchCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "measurements.batch_submitted", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if data.Case.State != domain.StateBaseline && data.Case.State != domain.StateMeasuring {
			return nil, domain.NewError(domain.CodeState, "当前状态不能提交初始测量批次")
		}
		if len(c.Measurements) == 0 || len(c.Measurements) > 500 {
			return nil, domain.NewError(domain.CodeInvalid, "测量批次必须包含 1 至 500 个条目")
		}
		seenScopes := make(map[string]bool)
		now := s.now().UTC()
		for i, measurement := range c.Measurements {
			measurement.CaseID = c.CaseID
			measurement.SubmittedBy = c.ActorID
			measurement.SubmittedAt = now
			key := string(measurement.ScopeType) + ":" + measurement.ScopeID
			if seenScopes[key] {
				return nil, domain.NewError(domain.CodeInvalid, "批次第 %d 项与其他条目使用了相同范围", i+1)
			}
			seenScopes[key] = true
			if err := appendMeasurement(data, measurement); err != nil {
				return nil, domain.NewError(domain.ErrorCodeOf(err), "批次第 %d 项无效：%s", i+1, err.Error())
			}
		}
		if err := data.Case.MarkMeasuring(); err != nil {
			return nil, err
		}
		return commandResult(data), nil
	})
}

func latestByScope(data *store.CaseData) map[string]domain.MeasurementSet {
	result := make(map[string]domain.MeasurementSet)
	for _, m := range store.LatestMeasurements(data) {
		result[string(m.ScopeType)+":"+m.ScopeID] = m
	}
	return result
}

func requiredScopeKeys(data *store.CaseData) []string {
	keys := []string{string(domain.ScopeProgram) + ":" + data.Case.CaseID}
	for _, segment := range data.Segments {
		keys = append(keys, string(domain.ScopeSegment)+":"+segment.SegmentID)
	}
	sort.Strings(keys)
	return keys
}

func (s *Service) Evaluate(ctx context.Context, c EvaluateCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "rules.evaluated", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if data.Case.State != domain.StateMeasuring {
			return nil, domain.NewError(domain.CodeState, "仅测量状态可执行首次判定")
		}
		if err := domain.ValidateSegmentsComplete(data.Segments, data.Case.StandardProfile.ExpectedDurationMillis); err != nil {
			return nil, err
		}
		latest := latestByScope(data)
		for _, key := range requiredScopeKeys(data) {
			if _, ok := latest[key]; !ok {
				return nil, domain.NewError(domain.CodeInvalid, "节目整体和每个分段都必须具备测量")
			}
		}
		failed := false
		now := s.now()
		allResults := make([]domain.RuleResult, 0, len(requiredScopeKeys(data))*3)
		for _, key := range requiredScopeKeys(data) {
			m := latest[key]
			results := domain.EvaluateMeasurement(m, data.Case.StandardProfile, now)
			allResults = append(allResults, results...)
			data.RuleResults = append(data.RuleResults, results...)
			for _, result := range results {
				if !result.Passed {
					failed = true
					d := domain.NewDeviation(c.CaseID, result, now)
					data.Deviations = append(data.Deviations, d)
				}
			}
		}
		snapshot, err := domain.NewRuleSnapshot(c.CaseID, allResults, nil, nil, now)
		if err != nil {
			return nil, err
		}
		data.RuleSnapshots = append(data.RuleSnapshots, snapshot)
		if err := data.Case.MarkEvaluation(failed); err != nil {
			return nil, err
		}
		return store.Change{Response: commandResult(data), EventData: map[string]any{"snapshot_id": snapshot.SnapshotID, "result_digest": snapshot.ResultDigest, "failed": failed}}, nil
	})
}
