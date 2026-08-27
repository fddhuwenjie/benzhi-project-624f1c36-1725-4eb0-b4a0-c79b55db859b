package workflow

import (
	"context"

	"mastergate/internal/domain"
	"mastergate/internal/store"
)

func (s *Service) CreateCase(ctx context.Context, c CreateCaseCommand) (CommandResult, error) {
	identity := store.DraftIdentity(domain.DeliveryCase{ProgramCode: c.ProgramCode, MasterVersion: c.MasterVersion, MasterSHA256: c.MasterSHA256})
	mutation := store.Mutation{CaseID: c.CaseID, ActorID: c.EngineerID, EventType: "case.created", Create: true, StatusCode: 201, DraftIdentity: identity}
	return s.executeMutation(ctx, c.Metadata, mutation, c, func(data *store.CaseData) (any, error) {
		created, err := domain.NewDeliveryCase(c.CaseID, c.ProgramCode, c.MasterVersion, c.EngineerID, c.MasterSHA256, c.Standard, s.now())
		if err != nil {
			return nil, err
		}
		data.Case = *created
		return commandResult(data), nil
	})
}

func (s *Service) ReviseCase(ctx context.Context, c ReviseCaseCommand) (CommandResult, error) {
	identity := store.DraftIdentity(domain.DeliveryCase{ProgramCode: c.ProgramCode, MasterVersion: c.MasterVersion, MasterSHA256: c.MasterSHA256})
	mutation := store.Mutation{CaseID: c.CaseID, ActorID: c.ActorID, EventType: "case.metadata_revised", DraftIdentity: identity}
	return s.executeMutation(ctx, c.Metadata, mutation, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if err := data.Case.ReviseMetadata(c.ProgramCode, c.MasterVersion, c.EngineerID, c.MasterSHA256, c.Standard); err != nil {
			return nil, err
		}
		return commandResult(data), nil
	})
}

func (s *Service) FreezeBaseline(ctx context.Context, c FreezeBaselineCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "baseline.frozen", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if err := data.Case.FreezeBaseline(s.now()); err != nil {
			return nil, err
		}
		return commandResult(data), nil
	})
}

func (s *Service) AddSegment(ctx context.Context, c AddSegmentCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "segment.registered", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireEngineer(data, c.ActorID); err != nil {
			return nil, err
		}
		if data.Case.State != domain.StateBaseline {
			return nil, domain.NewError(domain.CodeState, "仅冻结基线后、测量前可登记分段")
		}
		c.Segment.CaseID = c.CaseID
		next := append(append([]domain.ProgramSegment(nil), data.Segments...), c.Segment)
		if err := domain.ValidateSegmentCollection(next, c.CaseID, data.Case.StandardProfile.ExpectedDurationMillis); err != nil {
			return nil, err
		}
		data.Segments = next
		data.Case.Revision++
		return commandResult(data), nil
	})
}

func (s *Service) ReviseSegment(ctx context.Context, c ReviseSegmentCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "segment.revised", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireSegmentPlanMutable(data, c.ActorID); err != nil {
			return nil, err
		}
		if c.Segment.SegmentID != "" && c.Segment.SegmentID != c.SegmentID {
			return nil, domain.NewError(domain.CodeInvalid, "修订后的 segment_id 不得改变")
		}
		index := -1
		for i := range data.Segments {
			if data.Segments[i].SegmentID == c.SegmentID {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, domain.NewError(domain.CodeNotFound, "分段不存在")
		}
		before := data.Segments[index]
		c.Segment.SegmentID = c.SegmentID
		c.Segment.CaseID = c.CaseID
		next := append([]domain.ProgramSegment(nil), data.Segments...)
		next[index] = c.Segment
		if err := domain.ValidateSegmentCollection(next, c.CaseID, data.Case.StandardProfile.ExpectedDurationMillis); err != nil {
			return nil, err
		}
		data.Segments = next
		data.Case.Revision++
		return store.Change{Response: commandResult(data), EventData: map[string]any{"segment_id": c.SegmentID, "before": before, "after": c.Segment}}, nil
	})
}

func (s *Service) WithdrawSegment(ctx context.Context, c WithdrawSegmentCommand) (CommandResult, error) {
	return s.execute(ctx, c.Metadata, c.CaseID, c.ActorID, "segment.withdrawn", false, 200, c, func(data *store.CaseData) (any, error) {
		if err := requireSegmentPlanMutable(data, c.ActorID); err != nil {
			return nil, err
		}
		index := -1
		for i := range data.Segments {
			if data.Segments[i].SegmentID == c.SegmentID {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, domain.NewError(domain.CodeNotFound, "分段不存在")
		}
		for _, measurement := range data.Measurements {
			if measurement.ScopeType == domain.ScopeSegment && measurement.ScopeID == c.SegmentID {
				return nil, domain.NewError(domain.CodeState, "分段已有测量引用，不能撤销")
			}
		}
		for _, result := range data.RuleResults {
			if result.ScopeType == domain.ScopeSegment && result.ScopeID == c.SegmentID {
				return nil, domain.NewError(domain.CodeState, "分段已有规则结果引用，不能撤销")
			}
		}
		for _, deviation := range data.Deviations {
			if deviation.ScopeType == domain.ScopeSegment && deviation.ScopeID == c.SegmentID {
				return nil, domain.NewError(domain.CodeState, "分段已有偏差引用，不能撤销")
			}
		}
		next := append([]domain.ProgramSegment(nil), data.Segments[:index]...)
		next = append(next, data.Segments[index+1:]...)
		if err := domain.ValidateSegmentCollection(next, c.CaseID, data.Case.StandardProfile.ExpectedDurationMillis); err != nil {
			return nil, err
		}
		data.Segments = next
		data.Case.Revision++
		return store.Change{Response: commandResult(data), EventData: map[string]any{"segment_id": c.SegmentID}}, nil
	})
}

func requireSegmentPlanMutable(data *store.CaseData, actor string) error {
	if err := requireEngineer(data, actor); err != nil {
		return err
	}
	if data.Case.State != domain.StateBaseline || len(data.Measurements) != 0 {
		return domain.NewError(domain.CodeState, "仅基线已冻结且尚未登记任何测量时可变更分段方案")
	}
	return nil
}
