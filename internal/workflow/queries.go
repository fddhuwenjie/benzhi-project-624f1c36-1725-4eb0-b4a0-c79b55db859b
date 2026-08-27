package workflow

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"

	"mastergate/internal/domain"
	"mastergate/internal/store"
)

func (s *Service) GetCase(ctx context.Context, caseID string) (CaseView, error) {
	data, err := s.repo.GetCase(ctx, caseID)
	if err != nil {
		return CaseView{}, err
	}
	integrity := "valid"
	if err := s.repo.Verify(ctx, caseID); err != nil {
		integrity = err.Error()
	}
	queue, summary := s.deviationQueue(data.Deviations)
	return CaseView{
		Case: data.Case, Segments: data.Segments, Measurements: data.Measurements,
		RuleResults: data.RuleResults, RuleSnapshots: data.RuleSnapshots,
		Deviations: data.Deviations, DeviationQueue: queue, DeviationSummary: summary,
		Coverage: domain.InspectSegmentCoverage(data.Segments, data.Case.StandardProfile.ExpectedDurationMillis),
		Events:   data.Events, Manifest: data.Manifest, Integrity: integrity,
	}, nil
}

func (s *Service) deviationQueue(deviations []domain.Deviation) ([]DeviationQueueItem, DeviationSummary) {
	now := s.now()
	queue := make([]DeviationQueueItem, 0)
	summary := DeviationSummary{}
	for _, deviation := range deviations {
		if deviation.State == domain.DeviationReviewDue {
			continue
		}
		priority := rulePriority(deviation.RuleCode)
		item := DeviationQueueItem{Deviation: deviation, Priority: priority, PriorityLabel: priorityLabel(priority), BlocksApproval: true}
		summary.OpenCount++
		var deadline time.Time
		if deviation.State == domain.DeviationOpen {
			deadline = deviation.CreatedAt.Add(s.correctionDeadline)
			item.OverdueReason = "整改登记超过时限"
		} else {
			summary.RetestDueCount++
			base := deviation.CreatedAt
			if deviation.CorrectionRecordedAt != nil {
				base = *deviation.CorrectionRecordedAt
			}
			if deviation.LastRetestAt != nil {
				base = *deviation.LastRetestAt
			}
			deadline = base.Add(s.retestDeadline)
			item.OverdueReason = "复测提交超过时限"
		}
		item.Overdue = !deadline.IsZero() && now.After(deadline)
		if item.Overdue {
			summary.OverdueCount++
		} else {
			item.OverdueReason = ""
		}
		queue = append(queue, item)
	}
	sort.Slice(queue, func(i, j int) bool {
		if queue[i].Priority != queue[j].Priority {
			return queue[i].Priority > queue[j].Priority
		}
		if !queue[i].CreatedAt.Equal(queue[j].CreatedAt) {
			return queue[i].CreatedAt.Before(queue[j].CreatedAt)
		}
		return queue[i].DeviationID < queue[j].DeviationID
	})
	return queue, summary
}

func rulePriority(rule string) int {
	switch rule {
	case domain.RuleTruePeak:
		return 300
	case domain.RuleIntegrated:
		return 200
	case domain.RuleRange:
		return 100
	default:
		return 50
	}
}

func priorityLabel(priority int) string {
	if priority >= 300 {
		return "紧急"
	}
	if priority >= 200 {
		return "高"
	}
	return "普通"
}

func (s *Service) ListCases(ctx context.Context) ([]domain.DeliveryCase, error) {
	return s.repo.ListCases(ctx)
}

func (s *Service) SearchCases(ctx context.Context, filter CaseListFilter) ([]CaseListItem, error) {
	cases, err := s.repo.ListCases(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]CaseListItem, 0, len(cases))
	for _, deliveryCase := range cases {
		if filter.ProgramCode != "" && !strings.EqualFold(deliveryCase.ProgramCode, filter.ProgramCode) {
			continue
		}
		if filter.State != "" && deliveryCase.State != filter.State {
			continue
		}
		if filter.EngineerID != "" && deliveryCase.EngineerID != filter.EngineerID {
			continue
		}
		data, err := s.repo.GetCase(ctx, deliveryCase.CaseID)
		if err != nil {
			return nil, err
		}
		if filter.ApprovedFrom != nil || filter.ApprovedTo != nil {
			outside := data.Manifest == nil || filter.ApprovedFrom != nil && data.Manifest.ApprovedAt.Before(*filter.ApprovedFrom) || filter.ApprovedTo != nil && data.Manifest.ApprovedAt.After(*filter.ApprovedTo)
			if outside {
				continue
			}
		}
		validManifest := data.Manifest != nil && domain.VerifyManifest(*data.Manifest) == nil && s.repo.Verify(ctx, deliveryCase.CaseID) == nil
		result = append(result, CaseListItem{DeliveryCase: deliveryCase, HasValidManifest: validManifest})
	}
	return result, nil
}

func (s *Service) SegmentPreflight(ctx context.Context, caseID string) (domain.SegmentCoverageReport, error) {
	data, err := s.repo.GetCase(ctx, caseID)
	if err != nil {
		return domain.SegmentCoverageReport{}, err
	}
	if encoded, ok := s.preflightCache.Load(caseID); ok {
		var cached domain.SegmentCoverageReport
		if err := json.Unmarshal(encoded.([]byte), &cached); err != nil {
			return domain.SegmentCoverageReport{}, err
		}
		return cached, nil
	}
	report := domain.InspectSegmentCoverage(data.Segments, data.Case.StandardProfile.ExpectedDurationMillis)
	encoded, err := json.Marshal(report)
	if err != nil {
		return domain.SegmentCoverageReport{}, err
	}
	s.preflightCache.Store(caseID, encoded)
	return report, nil
}

func (s *Service) Readiness(ctx context.Context, caseID string) (domain.ReadinessReport, error) {
	data, err := s.repo.GetCase(ctx, caseID)
	if err != nil {
		return domain.ReadinessReport{}, err
	}
	return readinessForData(&data), nil
}

func readinessForData(data *store.CaseData) domain.ReadinessReport {
	return domain.BuildReadinessReport(data.Case, data.Segments, store.LatestMeasurements(data), finalRuleResults(data), data.RuleSnapshots, data.Deviations, data.Events)
}

func (s *Service) Timeline(ctx context.Context, caseID string) ([]domain.Event, error) {
	data, err := s.repo.GetCase(ctx, caseID)
	if err != nil {
		return nil, err
	}
	return data.Events, nil
}

func (s *Service) VerifyManifest(ctx context.Context, caseID string) (ManifestVerification, error) {
	data, err := s.repo.GetCase(ctx, caseID)
	if err != nil {
		return ManifestVerification{}, err
	}
	if data.Manifest == nil {
		return ManifestVerification{}, domain.NewError(domain.CodeNotFound, "案件尚未生成交付清单")
	}
	m := *data.Manifest
	report := ManifestVerification{CaseID: caseID, Valid: true, ManifestSHA256: m.ManifestSHA256, EventChainHead: m.EventChainHead, CanonicalPayload: domain.CanonicalManifestPayload(m)}
	addCheck := func(field string, valid bool, message string) {
		report.Checks = append(report.Checks, VerificationCheck{Field: field, Valid: valid, Message: message})
		if !valid && report.FailureLocation == "" {
			report.Valid = false
			report.FailureLocation = field
			report.Message = message
		}
	}
	addCheck("case_id", m.CaseID == data.Case.CaseID, chooseMessage(m.CaseID == data.Case.CaseID, "案件标识一致", "清单 case_id 与案件不一致"))
	addCheck("program_code", m.ProgramCode == data.Case.ProgramCode, chooseMessage(m.ProgramCode == data.Case.ProgramCode, "节目编号一致", "清单 program_code 与案件不一致"))
	addCheck("master_version", m.MasterVersion == data.Case.MasterVersion, chooseMessage(m.MasterVersion == data.Case.MasterVersion, "制作版本一致", "清单 master_version 与案件不一致"))
	addCheck("master_sha256", m.MasterSHA256 == data.Case.MasterSHA256, chooseMessage(m.MasterSHA256 == data.Case.MasterSHA256, "母版摘要一致", "清单 master_sha256 与案件不一致"))
	measurementsValid := reflect.DeepEqual(m.FinalMeasurements, sortedMeasurements(data))
	addCheck("final_measurements", measurementsValid, chooseMessage(measurementsValid, "最终测量集合与版本链一致", "清单 final_measurements 与当前版本链不一致"))
	resultsValid := reflect.DeepEqual(m.RuleResults, finalRuleResults(&data))
	addCheck("rule_results", resultsValid, chooseMessage(resultsValid, "最终规则结果一致", "清单 rule_results 与最终规则快照不一致"))
	addCheck("reviewer_id", m.ReviewerID == data.Case.ReviewedBy, chooseMessage(m.ReviewerID == data.Case.ReviewedBy, "复核员标识一致", "清单 reviewer_id 与复核记录不一致"))
	addCheck("approved_at", !m.ApprovedAt.IsZero(), chooseMessage(!m.ApprovedAt.IsZero(), "批准时间有效", "清单 approved_at 缺失"))
	chainErr := domain.VerifyEventChain(data.Events)
	addCheck("event_sequence_and_digest", chainErr == nil, errorMessage(chainErr, "事件序号与摘要连续"))
	head := ""
	if len(data.Events) > 0 {
		head = data.Events[len(data.Events)-1].Digest
	}
	headValid := head == m.EventChainHead
	addCheck("event_chain_head", headValid, chooseMessage(headValid, "事件链尾与清单一致", "清单 event_chain_head 与事件链尾不一致"))
	ruleErr := domain.VerifyRuleResultDigest(m)
	addCheck("rule_result_digest", ruleErr == nil, errorMessage(ruleErr, "规则结果摘要有效"))
	digest, digestErr := domain.ManifestDigest(m)
	digestValid := digestErr == nil && digest == m.ManifestSHA256
	addCheck("manifest_sha256", digestValid, chooseMessage(digestValid, "清单 SHA-256 可复算且一致", errorMessage(digestErr, "交付清单 SHA-256 校验失败")))
	if report.Valid {
		report.Message = "清单规范载荷、逐字段摘要与事件链连续性均有效"
	}
	return report, nil
}

func sortedMeasurements(data store.CaseData) []domain.MeasurementSet {
	measurements := store.LatestMeasurements(&data)
	sort.Slice(measurements, func(i, j int) bool {
		if measurements[i].ScopeType == measurements[j].ScopeType {
			return measurements[i].ScopeID < measurements[j].ScopeID
		}
		return measurements[i].ScopeType < measurements[j].ScopeType
	})
	return measurements
}

func chooseMessage(ok bool, success, failure string) string {
	if ok {
		return success
	}
	return failure
}

func errorMessage(err error, success string) string {
	if err == nil {
		return success
	}
	return err.Error()
}
