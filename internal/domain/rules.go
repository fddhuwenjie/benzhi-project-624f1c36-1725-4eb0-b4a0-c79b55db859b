package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

const (
	RuleIntegrated = "integrated_loudness"
	RuleRange      = "loudness_range"
	RuleTruePeak   = "true_peak"
)

func stableResultID(measurementID, rule string) string {
	sum := sha256.Sum256([]byte(measurementID + ":" + rule))
	return "rr-" + hex.EncodeToString(sum[:8])
}

func EvaluateMeasurement(m MeasurementSet, profile StandardProfile, now time.Time) []RuleResult {
	min := profile.TargetIntegratedLUFS - profile.IntegratedToleranceLU
	max := profile.TargetIntegratedLUFS + profile.IntegratedToleranceLU
	results := []RuleResult{
		{ResultID: stableResultID(m.MeasurementID, RuleIntegrated), CaseID: m.CaseID, MeasurementID: m.MeasurementID, ScopeType: m.ScopeType, ScopeID: m.ScopeID, RuleCode: RuleIntegrated, Passed: m.IntegratedLUFS >= min && m.IntegratedLUFS <= max, Actual: m.IntegratedLUFS, Minimum: &min, Maximum: &max, Unit: "LUFS", Explanation: fmt.Sprintf("%.2f LUFS 必须位于 %.2f 至 %.2f LUFS", m.IntegratedLUFS, min, max), EvaluatedAt: now.UTC()},
		{ResultID: stableResultID(m.MeasurementID, RuleRange), CaseID: m.CaseID, MeasurementID: m.MeasurementID, ScopeType: m.ScopeType, ScopeID: m.ScopeID, RuleCode: RuleRange, Passed: m.LoudnessRangeLU <= profile.MaxLoudnessRangeLU, Actual: m.LoudnessRangeLU, Maximum: floatPtr(profile.MaxLoudnessRangeLU), Unit: "LU", Explanation: fmt.Sprintf("%.2f LU 不得超过 %.2f LU", m.LoudnessRangeLU, profile.MaxLoudnessRangeLU), EvaluatedAt: now.UTC()},
		{ResultID: stableResultID(m.MeasurementID, RuleTruePeak), CaseID: m.CaseID, MeasurementID: m.MeasurementID, ScopeType: m.ScopeType, ScopeID: m.ScopeID, RuleCode: RuleTruePeak, Passed: m.TruePeakDBTP <= profile.MaxTruePeakDBTP, Actual: m.TruePeakDBTP, Maximum: floatPtr(profile.MaxTruePeakDBTP), Unit: "dBTP", Explanation: fmt.Sprintf("%.2f dBTP 不得超过 %.2f dBTP", m.TruePeakDBTP, profile.MaxTruePeakDBTP), EvaluatedAt: now.UTC()},
	}
	sort.Slice(results, func(i, j int) bool { return results[i].RuleCode < results[j].RuleCode })
	return results
}

func EvaluateRule(m MeasurementSet, profile StandardProfile, rule string, now time.Time) (RuleResult, error) {
	for _, result := range EvaluateMeasurement(m, profile, now) {
		if result.RuleCode == rule {
			return result, nil
		}
	}
	return RuleResult{}, NewError(CodeInvalid, "未知规则 %s", rule)
}

func floatPtr(v float64) *float64 { return &v }

func NewDeviation(caseID string, result RuleResult, times ...time.Time) Deviation {
	sum := sha256.Sum256([]byte(caseID + ":" + result.ScopeID + ":" + result.RuleCode + ":" + result.MeasurementID))
	createdAt := result.EvaluatedAt
	if len(times) > 0 {
		createdAt = times[0]
	}
	return Deviation{DeviationID: "dev-" + hex.EncodeToString(sum[:8]), CaseID: caseID, RuleCode: result.RuleCode, ScopeType: result.ScopeType, ScopeID: result.ScopeID, FailedMeasurementID: result.MeasurementID, State: DeviationOpen, CreatedAt: createdAt.UTC()}
}

func (d *Deviation) RecordCorrection(rootCause, summary, replacementSHA string, times ...time.Time) error {
	if d.State != DeviationOpen {
		return NewError(CodeState, "仅开放偏差可登记整改")
	}
	if err := ValidateText("根本原因", rootCause, 500); err != nil {
		return err
	}
	if err := ValidateText("整改说明", summary, 500); err != nil {
		return err
	}
	if err := ValidateSHA256("替代母版摘要", replacementSHA); err != nil {
		return err
	}
	d.RootCause = rootCause
	d.CorrectionSummary = summary
	d.ReplacementAudioSHA256 = replacementSHA
	d.State = DeviationRetestDue
	now := time.Now()
	if len(times) > 0 {
		now = times[0]
	}
	t := now.UTC()
	d.CorrectionRecordedAt = &t
	return nil
}

func (d *Deviation) ApplyRetest(measurementID string, passed bool, now time.Time) error {
	if d.State != DeviationRetestDue {
		return NewError(CodeState, "偏差尚未登记整改")
	}
	if err := ValidateIdentifier("复测标识", measurementID); err != nil {
		return err
	}
	d.RetestMeasurementID = measurementID
	t := now.UTC()
	d.LastRetestAt = &t
	d.RetestHistory = append(d.RetestHistory, RetestAttempt{MeasurementID: measurementID, Passed: passed, SubmittedAt: t})
	if passed {
		d.ResolvedAt = &t
		d.State = DeviationReviewDue
	} else {
		d.FailedRetestIDs = append(d.FailedRetestIDs, measurementID)
		d.State = DeviationRetestDue
	}
	return nil
}

type snapshotResult struct {
	MeasurementID string    `json:"measurement_id"`
	ScopeType     ScopeType `json:"scope_type"`
	ScopeID       string    `json:"scope_id"`
	RuleCode      string    `json:"rule_code"`
	Passed        bool      `json:"passed"`
	Actual        float64   `json:"actual"`
	Minimum       *float64  `json:"minimum,omitempty"`
	Maximum       *float64  `json:"maximum,omitempty"`
	Unit          string    `json:"unit"`
	Explanation   string    `json:"explanation"`
}

func NewRuleSnapshot(caseID string, results []RuleResult, previous *RuleSnapshot, affected map[string]bool, now time.Time) (RuleSnapshot, error) {
	ordered := append([]RuleResult(nil), results...)
	sort.Slice(ordered, func(i, j int) bool {
		left := string(ordered[i].ScopeType) + ":" + ordered[i].ScopeID + ":" + ordered[i].RuleCode
		right := string(ordered[j].ScopeType) + ":" + ordered[j].ScopeID + ":" + ordered[j].RuleCode
		return left < right
	})
	canonical := make([]snapshotResult, 0, len(ordered))
	for _, result := range ordered {
		canonical = append(canonical, snapshotResult{MeasurementID: result.MeasurementID, ScopeType: result.ScopeType, ScopeID: result.ScopeID, RuleCode: result.RuleCode, Passed: result.Passed, Actual: result.Actual, Minimum: result.Minimum, Maximum: result.Maximum, Unit: result.Unit, Explanation: result.Explanation})
	}
	digest, err := digestJSON(canonical)
	if err != nil {
		return RuleSnapshot{}, err
	}
	snapshot := RuleSnapshot{SnapshotID: "snapshot-" + digest[:16], CaseID: caseID, ResultDigest: digest, Results: ordered, EvaluatedAt: now.UTC()}
	previousByKey := make(map[string]RuleResult)
	if previous != nil {
		snapshot.PreviousDigest = previous.ResultDigest
		for _, result := range previous.Results {
			previousByKey[string(result.ScopeType)+":"+result.ScopeID+":"+result.RuleCode] = result
		}
	}
	for _, result := range ordered {
		key := string(result.ScopeType) + ":" + result.ScopeID + ":" + result.RuleCode
		difference := RuleDifference{ScopeType: result.ScopeType, ScopeID: result.ScopeID, RuleCode: result.RuleCode, CurrentPassed: result.Passed, Affected: previous == nil || affected[key]}
		if old, ok := previousByKey[key]; ok {
			passed := old.Passed
			difference.PreviousPassed = &passed
		}
		snapshot.Differences = append(snapshot.Differences, difference)
	}
	return snapshot, nil
}
