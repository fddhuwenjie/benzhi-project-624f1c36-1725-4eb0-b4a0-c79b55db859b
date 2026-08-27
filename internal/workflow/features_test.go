package workflow

import (
	"context"
	"testing"
	"time"

	"mastergate/internal/domain"
	"mastergate/internal/store"
)

const testSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func featureProfile(duration int64) domain.StandardProfile {
	return domain.StandardProfile{Name: "EBU-R128", TargetIntegratedLUFS: -23, IntegratedToleranceLU: 1, MaxLoudnessRangeLU: 20, MaxTruePeakDBTP: -1, ExpectedDurationMillis: duration}
}

func createFeatureCase(t *testing.T, service *Service, id string) {
	t.Helper()
	_, err := service.CreateCase(context.Background(), CreateCaseCommand{Metadata: Metadata{RequestID: "create-" + id}, CaseID: id, ProgramCode: "PROGRAM-01", MasterVersion: "version-01", EngineerID: "engineer-01", MasterSHA256: testSHA, Standard: featureProfile(2000)})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDraftDuplicateRevisionAndFreezeProtection(t *testing.T) {
	repo, _ := store.Open("")
	service := New(repo)
	createFeatureCase(t, service, "case-01")
	duplicate := CreateCaseCommand{Metadata: Metadata{RequestID: "create-duplicate"}, CaseID: "case-02", ProgramCode: "PROGRAM-01", MasterVersion: "version-01", EngineerID: "engineer-02", MasterSHA256: testSHA, Standard: featureProfile(2000)}
	if _, err := service.CreateCase(context.Background(), duplicate); domain.ErrorCodeOf(err) != domain.CodeConflict {
		t.Fatalf("重复草稿应冲突: %v", err)
	}
	cases, _ := service.ListCases(context.Background())
	if len(cases) != 1 {
		t.Fatalf("重复建档改变了案件数量: %d", len(cases))
	}
	revise := ReviseCaseCommand{Metadata: Metadata{RequestID: "revise-01", ExpectedRevision: 1}, CaseID: "case-01", ActorID: "engineer-01", ProgramCode: "PROGRAM-01", MasterVersion: "version-02", EngineerID: "engineer-01", MasterSHA256: testSHA, Standard: featureProfile(2000)}
	first, err := service.ReviseCase(context.Background(), revise)
	if err != nil || first.Case.Revision != 2 {
		t.Fatalf("草稿修订失败: %#v %v", first, err)
	}
	if _, err := service.FreezeBaseline(context.Background(), FreezeBaselineCommand{Metadata: Metadata{RequestID: "freeze-01", ExpectedRevision: 2}, CaseID: "case-01", ActorID: "engineer-01"}); err != nil {
		t.Fatal(err)
	}
	replayed, err := service.ReviseCase(context.Background(), revise)
	if err != nil || replayed.Case.Revision != 2 {
		t.Fatalf("冻结后原 request_id 未重放原响应: %#v %v", replayed, err)
	}
	revise.RequestID = "revise-after-freeze"
	revise.ExpectedRevision = 3
	if _, err := service.ReviseCase(context.Background(), revise); domain.ErrorCodeOf(err) != domain.CodeState {
		t.Fatalf("冻结后新修订应被拒绝: %v", err)
	}
}

func TestCoverageAndMeasurementBatchRollback(t *testing.T) {
	repo, _ := store.Open("")
	service := New(repo)
	createFeatureCase(t, service, "case-01")
	ctx := context.Background()
	_, _ = service.FreezeBaseline(ctx, FreezeBaselineCommand{Metadata: Metadata{RequestID: "freeze-01", ExpectedRevision: 1}, CaseID: "case-01", ActorID: "engineer-01"})
	segment := func(request, id string, start, end int64, revision int64) {
		_, err := service.AddSegment(ctx, AddSegmentCommand{Metadata: Metadata{RequestID: request, ExpectedRevision: revision}, CaseID: "case-01", ActorID: "engineer-01", Segment: domain.ProgramSegment{SegmentID: id, Title: id, StartMillis: start, EndMillis: end, ChannelLayout: "stereo", AudioSHA256: testSHA, CalibrationRef: "CAL-01"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	segment("segment-01", "segment-01", 0, 1000, 2)
	segment("segment-02", "segment-02", 1200, 2000, 3)
	report, _ := service.SegmentPreflight(ctx, "case-01")
	if report.Valid || len(report.Gaps) != 1 || report.Gaps[0].StartMillis != 1000 || report.Gaps[0].EndMillis != 1200 {
		t.Fatalf("未报告预期覆盖缺口: %#v", report)
	}
	measurement := func(id string, scope domain.ScopeType, scopeID string) domain.MeasurementSet {
		return domain.MeasurementSet{MeasurementID: id, ScopeType: scope, ScopeID: scopeID, IntegratedLUFS: -23, LoudnessRangeLU: 10, TruePeakDBTP: -2, GateThresholdLU: -10, IntegratedUnit: "LUFS", RangeUnit: "LU", PeakUnit: "dBTP", GateUnit: "LU", EvidenceSHA256: testSHA}
	}
	batch := SubmitMeasurementBatchCommand{Metadata: Metadata{RequestID: "batch-01", ExpectedRevision: 4}, CaseID: "case-01", ActorID: "engineer-01", Measurements: []domain.MeasurementSet{measurement("measurement-01", domain.ScopeProgram, "case-01"), measurement("measurement-02", domain.ScopeSegment, "segment-01")}}
	batch.Measurements[1].RangeUnit = "wrong"
	if _, err := service.SubmitMeasurementBatch(ctx, batch); domain.ErrorCodeOf(err) != domain.CodeInvalid {
		t.Fatalf("非法批次应失败: %v", err)
	}
	view, _ := service.GetCase(ctx, "case-01")
	if view.Case.Revision != 4 || len(view.Measurements) != 0 || len(view.Events) != 4 {
		t.Fatalf("失败批次未完整回滚: revision=%d measurements=%d events=%d", view.Case.Revision, len(view.Measurements), len(view.Events))
	}
}

func TestDeviationPriorityAndSnapshotDigest(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	measurement := domain.MeasurementSet{MeasurementID: "measurement-01", CaseID: "case-01", ScopeType: domain.ScopeProgram, ScopeID: "case-01", IntegratedLUFS: -20, LoudnessRangeLU: 30, TruePeakDBTP: 1}
	results := domain.EvaluateMeasurement(measurement, featureProfile(2000), now)
	one, err := domain.NewRuleSnapshot("case-01", results, nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	two, _ := domain.NewRuleSnapshot("case-01", domain.EvaluateMeasurement(measurement, featureProfile(2000), now.Add(time.Hour)), nil, nil, now.Add(time.Hour))
	if one.ResultDigest != two.ResultDigest {
		t.Fatalf("相同标准与测量生成了不同摘要: %s %s", one.ResultDigest, two.ResultDigest)
	}
	repo, _ := store.Open("")
	service := NewWithDeadlines(repo, time.Hour, time.Hour)
	service.now = func() time.Time { return now.Add(2 * time.Hour) }
	deviations := []domain.Deviation{{DeviationID: "range-dev", RuleCode: domain.RuleRange, State: domain.DeviationOpen, CreatedAt: now}, {DeviationID: "peak-dev", RuleCode: domain.RuleTruePeak, State: domain.DeviationOpen, CreatedAt: now}}
	queue, summary := service.deviationQueue(deviations)
	if queue[0].DeviationID != "peak-dev" || !queue[0].Overdue || summary.OverdueCount != 2 {
		t.Fatalf("偏差优先级或逾期派生错误: %#v %#v", queue, summary)
	}
}

func featureMeasurement(id string, scope domain.ScopeType, scopeID string, integrated, peak float64) domain.MeasurementSet {
	return domain.MeasurementSet{MeasurementID: id, ScopeType: scope, ScopeID: scopeID, IntegratedLUFS: integrated, LoudnessRangeLU: 10, TruePeakDBTP: peak, GateThresholdLU: -10, IntegratedUnit: "LUFS", RangeUnit: "LU", PeakUnit: "dBTP", GateUnit: "LU", EvidenceSHA256: testSHA}
}

func TestSegmentRevisionReplayAndReferencedWithdrawalRollback(t *testing.T) {
	repo, _ := store.Open("")
	service := New(repo)
	createFeatureCase(t, service, "case-01")
	ctx := context.Background()
	_, _ = service.FreezeBaseline(ctx, FreezeBaselineCommand{Metadata: Metadata{RequestID: "freeze-01", ExpectedRevision: 1}, CaseID: "case-01", ActorID: "engineer-01"})
	add := func(request, id string, start, end, revision int64) {
		t.Helper()
		_, err := service.AddSegment(ctx, AddSegmentCommand{Metadata: Metadata{RequestID: request, ExpectedRevision: revision}, CaseID: "case-01", ActorID: "engineer-01", Segment: domain.ProgramSegment{SegmentID: id, Title: id, StartMillis: start, EndMillis: end, ChannelLayout: "stereo", AudioSHA256: testSHA, CalibrationRef: "CAL-01"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	add("segment-01", "segment-01", 0, 1000, 2)
	add("segment-02", "segment-02", 1100, 2000, 3)
	revise := ReviseSegmentCommand{Metadata: Metadata{RequestID: "revise-segment-02", ExpectedRevision: 4}, CaseID: "case-01", ActorID: "engineer-01", SegmentID: "segment-02", Segment: domain.ProgramSegment{SegmentID: "segment-02", Title: "修订后的第二段", StartMillis: 1000, EndMillis: 2000, ChannelLayout: "stereo", AudioSHA256: testSHA, CalibrationRef: "CAL-02"}}
	first, err := service.ReviseSegment(ctx, revise)
	if err != nil || first.Case.Revision != 5 {
		t.Fatalf("分段修订失败: %#v %v", first, err)
	}
	replayed, err := service.ReviseSegment(ctx, revise)
	if err != nil || replayed.Case.Revision != 5 {
		t.Fatalf("分段修订未幂等重放首次结果: %#v %v", replayed, err)
	}
	view, _ := service.GetCase(ctx, "case-01")
	if !view.Coverage.Valid || len(view.Events) != 5 || view.Events[4].Type != "segment.revised" {
		t.Fatalf("修订后覆盖或事件错误: %#v", view)
	}
	_, err = service.SubmitMeasurement(ctx, SubmitMeasurementCommand{Metadata: Metadata{RequestID: "measure-segment", ExpectedRevision: 5}, CaseID: "case-01", ActorID: "engineer-01", Measurement: featureMeasurement("measurement-01", domain.ScopeSegment, "segment-02", -23, -2)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.WithdrawSegment(ctx, WithdrawSegmentCommand{Metadata: Metadata{RequestID: "withdraw-referenced", ExpectedRevision: 6}, CaseID: "case-01", ActorID: "engineer-01", SegmentID: "segment-02"})
	if domain.ErrorCodeOf(err) != domain.CodeState {
		t.Fatalf("已有测量后撤销分段应冲突: %v", err)
	}
	after, _ := service.GetCase(ctx, "case-01")
	if after.Case.Revision != 6 || len(after.Segments) != 2 || len(after.Measurements) != 1 || len(after.Events) != 6 {
		t.Fatalf("失败撤销未原子回滚: revision=%d segments=%d measurements=%d events=%d", after.Case.Revision, len(after.Segments), len(after.Measurements), len(after.Events))
	}
}

func TestJointCorrectionRetestReadinessAndReview(t *testing.T) {
	repo, _ := store.Open("")
	service := New(repo)
	createFeatureCase(t, service, "case-01")
	ctx := context.Background()
	_, _ = service.FreezeBaseline(ctx, FreezeBaselineCommand{Metadata: Metadata{RequestID: "freeze-01", ExpectedRevision: 1}, CaseID: "case-01", ActorID: "engineer-01"})
	_, _ = service.AddSegment(ctx, AddSegmentCommand{Metadata: Metadata{RequestID: "segment-01", ExpectedRevision: 2}, CaseID: "case-01", ActorID: "engineer-01", Segment: domain.ProgramSegment{SegmentID: "segment-01", Title: "全节目", StartMillis: 0, EndMillis: 2000, ChannelLayout: "stereo", AudioSHA256: testSHA, CalibrationRef: "CAL-01"}})
	batch := SubmitMeasurementBatchCommand{Metadata: Metadata{RequestID: "measurements-01", ExpectedRevision: 3}, CaseID: "case-01", ActorID: "engineer-01", Measurements: []domain.MeasurementSet{featureMeasurement("measurement-program-01", domain.ScopeProgram, "case-01", -20, 0), featureMeasurement("measurement-segment-01", domain.ScopeSegment, "segment-01", -23, -2)}}
	if _, err := service.SubmitMeasurementBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Evaluate(ctx, EvaluateCommand{Metadata: Metadata{RequestID: "evaluate-01", ExpectedRevision: 4}, CaseID: "case-01", ActorID: "engineer-01"}); err != nil {
		t.Fatal(err)
	}
	view, _ := service.GetCase(ctx, "case-01")
	if len(view.Deviations) != 2 {
		t.Fatalf("预期同范围两项偏差，实际 %d", len(view.Deviations))
	}
	ids := []string{view.Deviations[0].DeviationID, view.Deviations[1].DeviationID}
	correct := CorrectDeviationsCommand{Metadata: Metadata{RequestID: "joint-correct-01", ExpectedRevision: 5}, CaseID: "case-01", ActorID: "engineer-01", DeviationIDs: ids, RootCause: "共同增益链偏移", CorrectionSummary: "统一校准并重新导出", ReplacementAudioSHA256: testSHA}
	if _, err := service.CorrectDeviations(ctx, correct); err != nil {
		t.Fatal(err)
	}
	retestMeasurement := featureMeasurement("measurement-program-02", domain.ScopeProgram, "case-01", -23, -2)
	retestMeasurement.SupersedesID = "measurement-program-01"
	if _, err := service.JointRetest(ctx, JointRetestCommand{Metadata: Metadata{RequestID: "joint-retest-01", ExpectedRevision: 6}, CaseID: "case-01", ActorID: "engineer-01", DeviationIDs: ids, Measurement: retestMeasurement}); err != nil {
		t.Fatal(err)
	}
	view, _ = service.GetCase(ctx, "case-01")
	if view.Case.State != domain.StateReview || view.Case.Revision != 7 || len(view.Measurements) != 3 {
		t.Fatalf("联合复测案件结果错误: state=%s revision=%d measurements=%d", view.Case.State, view.Case.Revision, len(view.Measurements))
	}
	for _, deviation := range view.Deviations {
		if deviation.State != domain.DeviationReviewDue || len(deviation.RetestHistory) != 1 || deviation.RetestMeasurementID != "measurement-program-02" {
			t.Fatalf("偏差未独立记录联合复测结果: %#v", deviation)
		}
	}
	affected := 0
	for _, difference := range view.RuleSnapshots[len(view.RuleSnapshots)-1].Differences {
		if difference.Affected {
			affected++
		}
	}
	if affected != 2 {
		t.Fatalf("联合快照受影响规则数量应为 2，实际 %d", affected)
	}
	readiness, err := service.Readiness(ctx, "case-01")
	if err != nil || !readiness.Ready || len(readiness.Categories) != 4 || readiness.Revision != 7 {
		t.Fatalf("完整案件就绪度错误: %#v %v", readiness, err)
	}
	annotations := []domain.ReviewAnnotation{{CheckType: "baseline", Comment: "冻结基线已核对"}, {CheckType: "evidence", Comment: "最终测量已核对"}, {CheckType: "rules", Comment: "规则快照已核对"}, {CheckType: "remediation", Comment: "联合整改历史已核对"}}
	stale := ReviewCommand{Metadata: Metadata{RequestID: "review-stale", ExpectedRevision: 6}, CaseID: "case-01", ReviewerID: "reviewer-01", Decision: "approve", Annotations: annotations}
	if _, err := service.Review(ctx, stale); domain.ErrorCodeOf(err) != domain.CodeConflict {
		t.Fatalf("陈旧复核 revision 应冲突: %v", err)
	} else if conflict := err.(*domain.Error); conflict.CurrentRevision != 7 {
		t.Fatalf("陈旧复核未返回当前 revision: %#v", conflict)
	}
	stale.RequestID = "review-approve"
	stale.ExpectedRevision = 7
	approved, err := service.Review(ctx, stale)
	if err != nil || approved.Case.State != domain.StateApproved || approved.Manifest == nil {
		t.Fatalf("就绪案件批准失败: %#v %v", approved, err)
	}
}

func TestJointRetestRejectsCrossScopeAtomically(t *testing.T) {
	repo, _ := store.Open("")
	service := New(repo)
	createFeatureCase(t, service, "case-01")
	ctx := context.Background()
	_, _ = service.FreezeBaseline(ctx, FreezeBaselineCommand{Metadata: Metadata{RequestID: "freeze-01", ExpectedRevision: 1}, CaseID: "case-01", ActorID: "engineer-01"})
	_, _ = service.AddSegment(ctx, AddSegmentCommand{Metadata: Metadata{RequestID: "segment-01", ExpectedRevision: 2}, CaseID: "case-01", ActorID: "engineer-01", Segment: domain.ProgramSegment{SegmentID: "segment-01", Title: "全节目", StartMillis: 0, EndMillis: 2000, ChannelLayout: "stereo", AudioSHA256: testSHA, CalibrationRef: "CAL-01"}})
	batch := SubmitMeasurementBatchCommand{Metadata: Metadata{RequestID: "measurements-01", ExpectedRevision: 3}, CaseID: "case-01", ActorID: "engineer-01", Measurements: []domain.MeasurementSet{featureMeasurement("measurement-program-01", domain.ScopeProgram, "case-01", -20, -2), featureMeasurement("measurement-segment-01", domain.ScopeSegment, "segment-01", -20, -2)}}
	_, _ = service.SubmitMeasurementBatch(ctx, batch)
	_, _ = service.Evaluate(ctx, EvaluateCommand{Metadata: Metadata{RequestID: "evaluate-01", ExpectedRevision: 4}, CaseID: "case-01", ActorID: "engineer-01"})
	view, _ := service.GetCase(ctx, "case-01")
	if len(view.Deviations) != 2 {
		t.Fatalf("预期跨范围两项偏差，实际 %d", len(view.Deviations))
	}
	for i, deviation := range view.Deviations {
		_, err := service.CorrectDeviation(ctx, CorrectDeviationCommand{Metadata: Metadata{RequestID: "correct-0" + string(rune('1'+i)), ExpectedRevision: int64(5 + i)}, CaseID: "case-01", ActorID: "engineer-01", DeviationID: deviation.DeviationID, RootCause: "共同问题", CorrectionSummary: "分别登记后验证范围", ReplacementAudioSHA256: testSHA})
		if err != nil {
			t.Fatal(err)
		}
	}
	measurement := featureMeasurement("measurement-program-02", domain.ScopeProgram, "case-01", -23, -2)
	measurement.SupersedesID = "measurement-program-01"
	ids := []string{view.Deviations[0].DeviationID, view.Deviations[1].DeviationID}
	_, err := service.JointRetest(ctx, JointRetestCommand{Metadata: Metadata{RequestID: "cross-scope-retest", ExpectedRevision: 7}, CaseID: "case-01", ActorID: "engineer-01", DeviationIDs: ids, Measurement: measurement})
	if domain.ErrorCodeOf(err) != domain.CodeInvalid {
		t.Fatalf("跨范围联合复测应整体拒绝: %v", err)
	}
	after, _ := service.GetCase(ctx, "case-01")
	if after.Case.Revision != 7 || len(after.Measurements) != 2 || len(after.Events) != 7 {
		t.Fatalf("跨范围联合复测改变了案件: revision=%d measurements=%d events=%d", after.Case.Revision, len(after.Measurements), len(after.Events))
	}
}
