package workflow

import (
	"context"
	"sort"

	"mastergate/internal/domain"
	"mastergate/internal/store"
)

func deviationIndexes(data *store.CaseData, ids []string) ([]int, error) {
	if len(ids) < 2 || len(ids) > 100 {
		return nil, domain.NewError(domain.CodeInvalid, "联合操作必须选择 2 至 100 项偏差")
	}
	byID := make(map[string]int, len(data.Deviations))
	for i := range data.Deviations {
		byID[data.Deviations[i].DeviationID] = i
	}
	seen := make(map[string]bool, len(ids))
	indexes := make([]int, 0, len(ids))
	for _, id := range ids {
		if err := domain.ValidateIdentifier("偏差标识", id); err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, domain.NewError(domain.CodeInvalid, "deviation_ids 不得重复")
		}
		seen[id] = true
		index, ok := byID[id]
		if !ok {
			return nil, domain.NewError(domain.CodeNotFound, "偏差 %s 不存在", id)
		}
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(i, j int) bool {
		return data.Deviations[indexes[i]].DeviationID < data.Deviations[indexes[j]].DeviationID
	})
	return indexes, nil
}

func validateSameScope(data *store.CaseData, indexes []int, state domain.DeviationState) (domain.ScopeType, string, error) {
	first := data.Deviations[indexes[0]]
	rules := make(map[string]bool, len(indexes))
	for _, index := range indexes {
		deviation := data.Deviations[index]
		if deviation.ScopeType != first.ScopeType || deviation.ScopeID != first.ScopeID {
			return "", "", domain.NewError(domain.CodeInvalid, "联合操作中的偏差必须属于同一测量范围")
		}
		if deviation.State != state {
			return "", "", domain.NewError(domain.CodeInvalid, "联合操作中的偏差状态必须一致且为 %s", state)
		}
		if rules[deviation.RuleCode] {
			return "", "", domain.NewError(domain.CodeInvalid, "同一规则不能在联合操作中重复")
		}
		rules[deviation.RuleCode] = true
	}
	return first.ScopeType, first.ScopeID, nil
}

func findDeviation(data *store.CaseData, id string) (*domain.Deviation, error) {
	for i := range data.Deviations {
		if data.Deviations[i].DeviationID == id {
			return &data.Deviations[i], nil
		}
	}
	return nil, domain.NewError(domain.CodeNotFound, "偏差不存在")
}

func (s *Service) CorrectDeviation(ctx context.Context, c CorrectDeviationCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "deviation.corrected", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if data.Case.State != domain.StateRemediation {
			return nil, domain.NewError(domain.CodeState, "案件不在整改状态")
		}
		deviation, err := findDeviation(data, c.DeviationID)
		if err != nil {
			return nil, err
		}
		if err := deviation.RecordCorrection(c.RootCause, c.CorrectionSummary, c.ReplacementAudioSHA256, s.now()); err != nil {
			return nil, err
		}
		if err := data.Case.MarkRemediation(); err != nil {
			return nil, err
		}
		return commandResult(data), nil
	})
}

func (s *Service) CorrectDeviations(ctx context.Context, c CorrectDeviationsCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "deviations.jointly_corrected", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if data.Case.State != domain.StateRemediation {
			return nil, domain.NewError(domain.CodeState, "案件不在整改状态")
		}
		indexes, err := deviationIndexes(data, c.DeviationIDs)
		if err != nil {
			return nil, err
		}
		if _, _, err := validateSameScope(data, indexes, domain.DeviationOpen); err != nil {
			return nil, err
		}
		now := s.now()
		for _, index := range indexes {
			if err := data.Deviations[index].RecordCorrection(c.RootCause, c.CorrectionSummary, c.ReplacementAudioSHA256, now); err != nil {
				return nil, err
			}
		}
		if err := data.Case.MarkRemediation(); err != nil {
			return nil, err
		}
		ids := append([]string(nil), c.DeviationIDs...)
		sort.Strings(ids)
		return store.Change{Response: commandResult(data), EventData: map[string]any{"deviation_ids": ids, "root_cause": c.RootCause, "correction_summary": c.CorrectionSummary, "replacement_audio_sha256": c.ReplacementAudioSHA256}}, nil
	})
}

func (s *Service) Retest(ctx context.Context, c RetestCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "deviation.retested", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if data.Case.State != domain.StateRemediation {
			return nil, domain.NewError(domain.CodeState, "案件不在整改状态")
		}
		deviation, err := findDeviation(data, c.DeviationID)
		if err != nil {
			return nil, err
		}
		if deviation.State != domain.DeviationRetestDue {
			return nil, domain.NewError(domain.CodeState, "偏差必须先登记整改说明")
		}
		failed, ok := store.FindMeasurement(data, deviation.FailedMeasurementID)
		if !ok {
			return nil, domain.NewError(domain.CodeIntegrity, "偏差引用的失败测量不存在")
		}
		if c.FailedMeasurementID != deviation.FailedMeasurementID || c.RuleCode != deviation.RuleCode || c.ScopeType != deviation.ScopeType || c.ScopeID != deviation.ScopeID {
			return nil, domain.NewError(domain.CodeInvalid, "复测必须准确匹配偏差的 failed_measurement_id、scope_type、scope_id 和 rule_code")
		}
		if failed.ScopeType != deviation.ScopeType || failed.ScopeID != deviation.ScopeID {
			return nil, domain.NewError(domain.CodeIntegrity, "偏差范围与失败测量不一致")
		}
		c.Measurement.CaseID = c.CaseID
		c.Measurement.SubmittedBy = c.ActorID
		c.Measurement.SubmittedAt = s.now().UTC()
		if c.Measurement.ScopeType != failed.ScopeType || c.Measurement.ScopeID != failed.ScopeID {
			return nil, domain.NewError(domain.CodeInvalid, "复测测量范围必须与偏差范围一致")
		}
		latestID := store.LatestMeasurementsForScope(data, failed.ScopeType, failed.ScopeID)
		if c.Measurement.SupersedesID != latestID {
			return nil, domain.NewError(domain.CodeConflict, "复测必须以当前最新测量 %s 作为 supersedes_id", latestID)
		}
		if err := appendMeasurement(data, c.Measurement); err != nil {
			return nil, err
		}
		now := s.now()
		result, err := domain.EvaluateRule(c.Measurement, data.Case.StandardProfile, deviation.RuleCode, now)
		if err != nil {
			return nil, err
		}
		data.RuleResults = append(data.RuleResults, result)
		if err := deviation.ApplyRetest(c.Measurement.MeasurementID, result.Passed, now); err != nil {
			return nil, err
		}
		allResolved := true
		for _, d := range data.Deviations {
			if d.State != domain.DeviationReviewDue {
				allResolved = false
			}
		}
		if allResolved {
			if err := data.Case.MarkReadyForReview(); err != nil {
				return nil, err
			}
		} else {
			if err := data.Case.MarkRemediation(); err != nil {
				return nil, err
			}
		}
		var previous *domain.RuleSnapshot
		if n := len(data.RuleSnapshots); n > 0 {
			previous = &data.RuleSnapshots[n-1]
		}
		affectedKey := string(result.ScopeType) + ":" + result.ScopeID + ":" + result.RuleCode
		snapshot, err := domain.NewRuleSnapshot(c.CaseID, finalRuleResults(data), previous, map[string]bool{affectedKey: true}, now)
		if err != nil {
			return nil, err
		}
		data.RuleSnapshots = append(data.RuleSnapshots, snapshot)
		return store.Change{Response: commandResult(data), EventData: map[string]any{"deviation_id": deviation.DeviationID, "measurement_id": c.Measurement.MeasurementID, "passed": result.Passed, "snapshot_id": snapshot.SnapshotID, "result_digest": snapshot.ResultDigest}}, nil
	})
}

func (s *Service) JointRetest(ctx context.Context, c JointRetestCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "deviations.jointly_retested", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if data.Case.State != domain.StateRemediation {
			return nil, domain.NewError(domain.CodeState, "案件不在整改状态")
		}
		indexes, err := deviationIndexes(data, c.DeviationIDs)
		if err != nil {
			return nil, err
		}
		scopeType, scopeID, err := validateSameScope(data, indexes, domain.DeviationRetestDue)
		if err != nil {
			return nil, err
		}
		replacement := data.Deviations[indexes[0]].ReplacementAudioSHA256
		if replacement == "" {
			return nil, domain.NewError(domain.CodeState, "所选偏差尚未完整登记整改")
		}
		for _, index := range indexes {
			deviation := data.Deviations[index]
			if deviation.ReplacementAudioSHA256 != replacement {
				return nil, domain.NewError(domain.CodeInvalid, "联合复测偏差必须引用同一替代母版")
			}
			if err := domain.ValidateSHA256("替代母版摘要", deviation.ReplacementAudioSHA256); err != nil {
				return nil, domain.NewError(domain.CodeIntegrity, "偏差 %s 的替代母版摘要无效", deviation.DeviationID)
			}
			failed, ok := store.FindMeasurement(data, deviation.FailedMeasurementID)
			if !ok || failed.ScopeType != scopeType || failed.ScopeID != scopeID {
				return nil, domain.NewError(domain.CodeIntegrity, "偏差 %s 引用的失败测量无效", deviation.DeviationID)
			}
			failureFound := false
			for _, result := range data.RuleResults {
				if result.MeasurementID == deviation.FailedMeasurementID && result.ScopeType == scopeType && result.ScopeID == scopeID && result.RuleCode == deviation.RuleCode && !result.Passed {
					failureFound = true
					break
				}
			}
			if !failureFound {
				return nil, domain.NewError(domain.CodeIntegrity, "偏差 %s 引用的失败规则结论无效", deviation.DeviationID)
			}
		}
		latestID := store.LatestMeasurementsForScope(data, scopeType, scopeID)
		if latestID == "" || c.Measurement.SupersedesID != latestID {
			return nil, domain.NewError(domain.CodeConflict, "联合复测必须以当前最新测量 %s 作为 supersedes_id", latestID)
		}
		c.Measurement.CaseID = c.CaseID
		c.Measurement.SubmittedBy = c.ActorID
		now := s.now()
		c.Measurement.SubmittedAt = now.UTC()
		if c.Measurement.ScopeType != scopeType || c.Measurement.ScopeID != scopeID {
			return nil, domain.NewError(domain.CodeInvalid, "联合复测测量范围必须与所选偏差范围一致")
		}
		if err := appendMeasurement(data, c.Measurement); err != nil {
			return nil, err
		}
		affected := make(map[string]bool, len(indexes))
		outcomes := make(map[string]bool, len(indexes))
		for _, index := range indexes {
			deviation := &data.Deviations[index]
			result, err := domain.EvaluateRule(c.Measurement, data.Case.StandardProfile, deviation.RuleCode, now)
			if err != nil {
				return nil, err
			}
			data.RuleResults = append(data.RuleResults, result)
			if err := deviation.ApplyRetest(c.Measurement.MeasurementID, result.Passed, now); err != nil {
				return nil, err
			}
			key := string(result.ScopeType) + ":" + result.ScopeID + ":" + result.RuleCode
			affected[key] = true
			outcomes[deviation.DeviationID] = result.Passed
		}
		allResolved := true
		for _, deviation := range data.Deviations {
			if deviation.State != domain.DeviationReviewDue {
				allResolved = false
				break
			}
		}
		if allResolved {
			if err := data.Case.MarkReadyForReview(); err != nil {
				return nil, err
			}
		} else if err := data.Case.MarkRemediation(); err != nil {
			return nil, err
		}
		var previous *domain.RuleSnapshot
		if n := len(data.RuleSnapshots); n > 0 {
			previous = &data.RuleSnapshots[n-1]
		}
		snapshot, err := domain.NewRuleSnapshot(c.CaseID, finalRuleResults(data), previous, affected, now)
		if err != nil {
			return nil, err
		}
		data.RuleSnapshots = append(data.RuleSnapshots, snapshot)
		ids := append([]string(nil), c.DeviationIDs...)
		sort.Strings(ids)
		return store.Change{Response: commandResult(data), EventData: map[string]any{"deviation_ids": ids, "measurement_id": c.Measurement.MeasurementID, "outcomes": outcomes, "snapshot_id": snapshot.SnapshotID, "result_digest": snapshot.ResultDigest}}, nil
	})
}
