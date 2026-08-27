package store

import "mastergate/internal/domain"

func validateSnapshot(s snapshot) error {
	for key, data := range s.Cases {
		if data.Case.CaseID != key {
			return domain.NewError(domain.CodeIntegrity, "案件索引与案件标识不一致")
		}
		if err := validateCase(data); err != nil {
			return err
		}
	}
	for key, record := range s.Idempotency {
		if record.RequestID != key || record.Fingerprint == "" || len(record.Response) == 0 {
			return domain.NewError(domain.CodeIntegrity, "幂等记录无效")
		}
	}
	return nil
}

func validateCase(data *CaseData) error {
	if data == nil {
		return domain.NewError(domain.CodeIntegrity, "案件数据为空")
	}
	if err := domain.VerifyEventChain(data.Events); err != nil {
		return err
	}
	if len(data.Events) == 0 {
		return domain.NewError(domain.CodeIntegrity, "案件缺少事件")
	}
	if data.Events[len(data.Events)-1].Revision != data.Case.Revision {
		return domain.NewError(domain.CodeIntegrity, "聚合修订号与事件链尾不一致")
	}
	measurementIDs := make(map[string]bool)
	measurementScope := make(map[string]string)
	for _, m := range data.Measurements {
		if measurementIDs[m.MeasurementID] {
			return domain.NewError(domain.CodeIntegrity, "测量版本标识重复")
		}
		measurementIDs[m.MeasurementID] = true
		key := string(m.ScopeType) + ":" + m.ScopeID
		if m.SupersedesID != "" {
			previousScope, ok := measurementScope[m.SupersedesID]
			if !ok || previousScope != key {
				return domain.NewError(domain.CodeIntegrity, "测量 %s 的 supersedes 链无效", m.MeasurementID)
			}
		}
		measurementScope[m.MeasurementID] = key
	}
	for _, snapshot := range data.RuleSnapshots {
		rebuilt, err := domain.NewRuleSnapshot(snapshot.CaseID, snapshot.Results, nil, nil, snapshot.EvaluatedAt)
		if err != nil || rebuilt.ResultDigest != snapshot.ResultDigest {
			return domain.NewError(domain.CodeIntegrity, "规则快照 %s 摘要无效", snapshot.SnapshotID)
		}
	}
	if data.Manifest != nil {
		if data.Case.State != domain.StateApproved {
			return domain.NewError(domain.CodeIntegrity, "非批准案件存在清单")
		}
		if data.Manifest.EventChainHead != data.Events[len(data.Events)-1].Digest {
			return domain.NewError(domain.CodeIntegrity, "清单链尾与事件链不一致")
		}
		if err := domain.VerifyManifest(*data.Manifest); err != nil {
			return err
		}
	}
	return nil
}
