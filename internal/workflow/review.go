package workflow

import (
	"context"
	"sort"

	"mastergate/internal/domain"
	"mastergate/internal/store"
)

var requiredChecks = map[string]bool{"baseline": true, "evidence": true, "rules": true, "remediation": true}

func validateAnnotations(annotations []domain.ReviewAnnotation) error {
	if len(annotations) != len(requiredChecks) {
		return domain.NewError(domain.CodeInvalid, "baseline、evidence、rules、remediation 均必须提供批注")
	}
	seen := make(map[string]bool)
	for _, annotation := range annotations {
		if !requiredChecks[annotation.CheckType] || seen[annotation.CheckType] {
			return domain.NewError(domain.CodeInvalid, "复核检查项无效或重复")
		}
		if err := domain.ValidateText("复核批注", annotation.Comment, 500); err != nil {
			return err
		}
		if annotation.EvidenceRef != "" {
			if err := domain.ValidateIdentifier("证据引用", annotation.EvidenceRef); err != nil {
				return err
			}
		}
		seen[annotation.CheckType] = true
	}
	return nil
}

func validateAnnotationReferences(data *store.CaseData, annotations []domain.ReviewAnnotation) error {
	valid := make(map[string]bool)
	for _, measurement := range data.Measurements {
		valid[measurement.MeasurementID] = true
	}
	for _, event := range data.Events {
		valid[event.Digest] = true
	}
	for _, annotation := range annotations {
		if annotation.EvidenceRef != "" && !valid[annotation.EvidenceRef] {
			return domain.NewError(domain.CodeInvalid, "复核批注引用的事件或测量标识不存在：%s", annotation.EvidenceRef)
		}
	}
	return nil
}

func finalRuleResults(data *store.CaseData) []domain.RuleResult {
	byKey := make(map[string]domain.RuleResult)
	for _, r := range data.RuleResults {
		byKey[string(r.ScopeType)+":"+r.ScopeID+":"+r.RuleCode] = r
	}
	result := make([]domain.RuleResult, 0, len(byKey))
	for _, r := range byKey {
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool {
		left := string(result[i].ScopeType) + ":" + result[i].ScopeID + ":" + result[i].RuleCode
		right := string(result[j].ScopeType) + ":" + result[j].ScopeID + ":" + result[j].RuleCode
		return left < right
	})
	return result
}

func (s *Service) Review(ctx context.Context, c ReviewCommand) (CommandResult, error) {
	eventType := "review.rejected"
	if c.Decision == "approve" {
		eventType = "review.approved"
	}
	result, err := s.execute(ctx, c.Metadata, c.CaseID, c.ReviewerID, eventType, false, 200, c, func(data *store.CaseData) (any, error) {
		if c.ReviewerID == data.Case.EngineerID {
			return nil, domain.NewError(domain.CodeForbidden, "复核员必须与提交工程师不同")
		}
		if err := validateAnnotations(c.Annotations); err != nil {
			return nil, err
		}
		if err := validateAnnotationReferences(data, c.Annotations); err != nil {
			return nil, err
		}
		if c.Decision == "reject" {
			if err := data.Case.Reject(c.ReviewerID, c.RejectionCode, c.RejectionDetail, c.Annotations); err != nil {
				return nil, err
			}
			return commandResult(data), nil
		}
		if c.Decision != "approve" {
			return nil, domain.NewError(domain.CodeInvalid, "复核结论仅支持 approve 或 reject")
		}
		readiness := readinessForData(data)
		if !readiness.Ready {
			return nil, domain.NewError(domain.CodeState, "复核就绪度存在 %d 项阻断，请刷新矩阵", len(readiness.Blockers))
		}
		if err := data.Case.Approve(c.ReviewerID, c.Annotations); err != nil {
			return nil, err
		}
		eventData := map[string]any{"decision": "approve", "reviewer_id": c.ReviewerID, "annotations": c.Annotations}
		return store.Change{Response: commandResult(data), EventData: eventData, Finalize: func(final *store.CaseData, event domain.Event) (any, error) {
			manifest, err := domain.CreateManifest(final.Case, store.LatestMeasurements(final), finalRuleResults(final), c.ReviewerID, event.Digest, event.OccurredAt)
			if err != nil {
				return nil, err
			}
			if final.Manifest != nil {
				return nil, domain.NewError(domain.CodeConflict, "交付清单已经封存")
			}
			final.Manifest = &manifest
			return commandResult(final), nil
		}}, nil
	})
	if domain.ErrorCodeOf(err) == domain.CodeConflict {
		if readiness, queryErr := s.Readiness(ctx, c.CaseID); queryErr == nil {
			return CommandResult{}, domain.ConflictWithReadiness(readiness.Revision, readiness.Blockers)
		}
	}
	return result, err
}
