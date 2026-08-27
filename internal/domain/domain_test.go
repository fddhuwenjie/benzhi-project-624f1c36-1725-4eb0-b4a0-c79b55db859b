package domain

import (
	"testing"
	"time"
)

func testProfile() StandardProfile {
	return StandardProfile{Name: "test", TargetIntegratedLUFS: -23, IntegratedToleranceLU: 1, MaxLoudnessRangeLU: 20, MaxTruePeakDBTP: -1, ExpectedDurationMillis: 1000}
}
func testMeasurement(id string, value float64) MeasurementSet {
	return MeasurementSet{MeasurementID: id, CaseID: "case01", ScopeType: ScopeProgram, ScopeID: "case01", IntegratedLUFS: value, LoudnessRangeLU: 10, TruePeakDBTP: -2, GateThresholdLU: -10, IntegratedUnit: "LUFS", RangeUnit: "LU", PeakUnit: "dBTP", GateUnit: "LU", EvidenceSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", SubmittedBy: "eng01", SubmittedAt: time.Now()}
}

func TestEvaluateMeasurementDeterministic(t *testing.T) {
	m := testMeasurement("m1", -23)
	a := EvaluateMeasurement(m, testProfile(), time.Unix(0, 0))
	b := EvaluateMeasurement(m, testProfile(), time.Unix(0, 0))
	if len(a) != 3 || a[0].ResultID != b[0].ResultID || !a[0].Passed {
		t.Fatalf("规则结果不稳定或错误: %#v %#v", a, b)
	}
	m.IntegratedLUFS = -20
	results := EvaluateMeasurement(m, testProfile(), time.Now())
	for _, r := range results {
		if r.RuleCode == RuleIntegrated && r.Passed {
			t.Fatal("超出容差的集成响度不应通过")
		}
	}
}

func TestValidateSegmentsComplete(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	segments := []ProgramSegment{{SegmentID: "s1", CaseID: "case01", Title: "A", StartMillis: 0, EndMillis: 400, ChannelLayout: "stereo", AudioSHA256: sha, CalibrationRef: "CAL"}, {SegmentID: "s2", CaseID: "case01", Title: "B", StartMillis: 400, EndMillis: 1000, ChannelLayout: "stereo", AudioSHA256: sha, CalibrationRef: "CAL"}}
	if err := ValidateSegmentsComplete(segments, 1000); err != nil {
		t.Fatal(err)
	}
	segments[1].StartMillis = 500
	if err := ValidateSegmentsComplete(segments, 1000); err == nil {
		t.Fatal("存在空隙时应失败")
	}
}

func TestManifestVerification(t *testing.T) {
	ruleDigest, _ := digestJSON([]RuleResult(nil))
	m := DeliveryManifest{ManifestID: "manifest-case01", CaseID: "case01", ProgramCode: "P", MasterVersion: "v1", MasterSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", RuleResultDigest: ruleDigest, ReviewerID: "review01", ApprovedAt: time.Unix(0, 0), EventChainHead: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	digest, err := ManifestDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	m.ManifestSHA256 = digest
	if err := VerifyManifest(m); err != nil {
		t.Fatal(err)
	}
	m.MasterVersion = "v2"
	if err := VerifyManifest(m); err == nil {
		t.Fatal("篡改清单应被发现")
	}
}
