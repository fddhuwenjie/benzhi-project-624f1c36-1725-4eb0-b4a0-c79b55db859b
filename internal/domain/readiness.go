package domain

import (
	"fmt"
	"sort"
)

var readinessCategories = []struct {
	code  string
	label string
}{{"baseline", "冻结基线"}, {"evidence", "测量证据"}, {"rules", "规则结论"}, {"remediation", "整改闭环"}}

// BuildReadinessReport is the single completeness rule set shared by the
// readiness query and review approval command.
func BuildReadinessReport(c DeliveryCase, segments []ProgramSegment, latest []MeasurementSet, finalResults []RuleResult, snapshots []RuleSnapshot, deviations []Deviation, events []Event) ReadinessReport {
	report := ReadinessReport{CaseID: c.CaseID, Revision: c.Revision, Ready: true, Categories: make([]ReadinessCategory, 0, len(readinessCategories)), Blockers: []ReadinessCheck{}, Hints: []ReadinessCheck{}}
	checks := make(map[string][]ReadinessCheck, len(readinessCategories))
	add := func(check ReadinessCheck) {
		checks[check.Category] = append(checks[check.Category], check)
		if !check.Blocking {
			report.Hints = append(report.Hints, check)
		} else if !check.Passed {
			report.Ready = false
			report.Blockers = append(report.Blockers, check)
		}
	}

	baselineEvent := eventByType(events, "baseline.frozen")
	baselineOK := c.BaselineFrozenAt != nil && baselineEvent != nil
	baseline := ReadinessCheck{Category: "baseline", Code: "BASELINE_FROZEN", Passed: baselineOK, Blocking: true, Message: "冻结标准及基线事件完整"}
	if baselineEvent != nil {
		baseline.EventDigest = baselineEvent.Digest
		baseline.EventSummary = fmt.Sprintf("#%d baseline.frozen", baselineEvent.Sequence)
	}
	if !baselineOK {
		baseline.Message = "缺少冻结标准时间或 baseline.frozen 事件"
	}
	add(baseline)
	standardErr := ValidateStandard(c.StandardProfile)
	standard := ReadinessCheck{Category: "baseline", Code: "BASELINE_STANDARD_VALID", Passed: standardErr == nil, Blocking: true, Message: "冻结响度标准字段有效", EventDigest: baseline.EventDigest, EventSummary: baseline.EventSummary}
	if standardErr != nil {
		standard.Message = standardErr.Error()
	}
	add(standard)

	coverageErr := ValidateSegmentsComplete(segments, c.StandardProfile.ExpectedDurationMillis)
	coverage := ReadinessCheck{Category: "baseline", Code: "SEGMENT_COVERAGE_COMPLETE", Passed: coverageErr == nil, Blocking: true, Message: "节目分段连续覆盖总时长且字段有效"}
	if coverageErr != nil {
		coverage.Message = coverageErr.Error()
	}
	add(coverage)

	chainErr := VerifyEventChain(events)
	chain := ReadinessCheck{Category: "baseline", Code: "EVENT_CHAIN_CONTINUOUS", Passed: chainErr == nil, Blocking: true, Message: "事件链序号与摘要连续"}
	if len(events) > 0 {
		chain.EventDigest = events[len(events)-1].Digest
		chain.EventSummary = fmt.Sprintf("事件链共 %d 项，链尾 #%d", len(events), events[len(events)-1].Sequence)
	}
	if chainErr != nil {
		chain.Message = chainErr.Error()
	}
	add(chain)

	latestByScope := make(map[string]MeasurementSet, len(latest))
	for _, measurement := range latest {
		latestByScope[scopeKey(measurement.ScopeType, measurement.ScopeID)] = measurement
	}
	for _, scope := range readinessScopes(c.CaseID, segments) {
		measurement, ok := latestByScope[scopeKey(scope.scopeType, scope.scopeID)]
		check := ReadinessCheck{Category: "evidence", Code: "FINAL_MEASUREMENT_PRESENT", Passed: ok, Blocking: true, ScopeType: scope.scopeType, ScopeID: scope.scopeID}
		if ok {
			check.MeasurementID = measurement.MeasurementID
			check.Message = fmt.Sprintf("范围 %s 的最终测量为 %s", scope.scopeID, measurement.MeasurementID)
		} else {
			check.Message = fmt.Sprintf("范围 %s 缺少最终测量", scope.scopeID)
		}
		add(check)
	}

	var latestSnapshot *RuleSnapshot
	if len(snapshots) > 0 {
		latestSnapshot = &snapshots[len(snapshots)-1]
	}
	snapshotCheck := ReadinessCheck{Category: "rules", Code: "FINAL_RULE_SNAPSHOT_PRESENT", Passed: latestSnapshot != nil, Blocking: true, Message: "缺少最终规则快照"}
	if latestSnapshot != nil {
		snapshotCheck.SnapshotID = latestSnapshot.SnapshotID
		snapshotCheck.Message = fmt.Sprintf("最终规则快照为 %s", latestSnapshot.SnapshotID)
	}
	add(snapshotCheck)
	if latestSnapshot != nil {
		rebuilt, err := NewRuleSnapshot(latestSnapshot.CaseID, latestSnapshot.Results, nil, nil, latestSnapshot.EvaluatedAt)
		valid := err == nil && rebuilt.ResultDigest == latestSnapshot.ResultDigest
		check := ReadinessCheck{Category: "rules", Code: "FINAL_RULE_SNAPSHOT_VALID", Passed: valid, Blocking: true, SnapshotID: latestSnapshot.SnapshotID, Message: "最终规则快照摘要可复算且一致"}
		if !valid {
			check.Message = "最终规则快照摘要无效"
		}
		add(check)
		current, currentErr := NewRuleSnapshot(c.CaseID, finalResults, nil, nil, latestSnapshot.EvaluatedAt)
		currentValid := currentErr == nil && current.ResultDigest == latestSnapshot.ResultDigest
		currentCheck := ReadinessCheck{Category: "rules", Code: "FINAL_RULE_SNAPSHOT_CURRENT", Passed: currentValid, Blocking: true, SnapshotID: latestSnapshot.SnapshotID, Message: "最终规则快照对应当前最终结论集合"}
		if !currentValid {
			currentCheck.Message = "最终规则快照与当前最终结论集合不一致"
		}
		add(currentCheck)
	}

	resultsByKey := make(map[string]RuleResult, len(finalResults))
	for _, result := range finalResults {
		resultsByKey[resultKey(result.ScopeType, result.ScopeID, result.RuleCode)] = result
	}
	ruleCodes := []string{RuleIntegrated, RuleRange, RuleTruePeak}
	for _, scope := range readinessScopes(c.CaseID, segments) {
		for _, ruleCode := range ruleCodes {
			result, ok := resultsByKey[resultKey(scope.scopeType, scope.scopeID, ruleCode)]
			check := ReadinessCheck{Category: "rules", Code: "FINAL_RULE_RESULT_PRESENT", Passed: ok, Blocking: true, ScopeType: scope.scopeType, ScopeID: scope.scopeID, SnapshotID: snapshotCheck.SnapshotID}
			if !ok {
				check.Message = fmt.Sprintf("范围 %s 缺少规则 %s 的最终结论", scope.scopeID, ruleCode)
				add(check)
				continue
			}
			check.MeasurementID = result.MeasurementID
			check.Message = fmt.Sprintf("范围 %s 的规则 %s 最终结论已定位", scope.scopeID, ruleCode)
			add(check)
			passed := ReadinessCheck{Category: "rules", Code: "FINAL_RULE_RESULT_PASSED", Passed: result.Passed, Blocking: true, ScopeType: scope.scopeType, ScopeID: scope.scopeID, MeasurementID: result.MeasurementID, SnapshotID: snapshotCheck.SnapshotID, Message: fmt.Sprintf("范围 %s 的规则 %s 最终通过", scope.scopeID, ruleCode)}
			if !result.Passed {
				passed.Message = fmt.Sprintf("范围 %s 的规则 %s 最终结论仍为失败", scope.scopeID, ruleCode)
			}
			add(passed)
		}
	}

	if len(deviations) == 0 {
		add(ReadinessCheck{Category: "remediation", Code: "REMEDIATION_NOT_REQUIRED", Passed: true, Blocking: false, Message: "案件没有需要整改的偏差，此项为提示"})
	} else {
		ordered := append([]Deviation(nil), deviations...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].DeviationID < ordered[j].DeviationID })
		for _, deviation := range ordered {
			passed := deviation.State == DeviationReviewDue && deviation.RetestMeasurementID != ""
			check := ReadinessCheck{Category: "remediation", Code: "DEVIATION_RETEST_COMPLETE", Passed: passed, Blocking: true, ScopeType: deviation.ScopeType, ScopeID: deviation.ScopeID, MeasurementID: deviation.RetestMeasurementID, DeviationID: deviation.DeviationID, SnapshotID: snapshotCheck.SnapshotID, Message: fmt.Sprintf("偏差 %s 已通过复测并待复核", deviation.DeviationID)}
			if !passed {
				check.Message = fmt.Sprintf("偏差 %s 尚未通过整改复测", deviation.DeviationID)
			}
			add(check)
		}
	}

	for _, category := range readinessCategories {
		categoryChecks := checks[category.code]
		passed := true
		for _, check := range categoryChecks {
			if check.Blocking && !check.Passed {
				passed = false
				break
			}
		}
		report.Categories = append(report.Categories, ReadinessCategory{Category: category.code, Label: category.label, Passed: passed, Checks: categoryChecks})
	}
	return report
}

type readinessScope struct {
	scopeType ScopeType
	scopeID   string
}

func readinessScopes(caseID string, segments []ProgramSegment) []readinessScope {
	scopes := []readinessScope{{scopeType: ScopeProgram, scopeID: caseID}}
	for _, segment := range segments {
		scopes = append(scopes, readinessScope{scopeType: ScopeSegment, scopeID: segment.SegmentID})
	}
	sort.Slice(scopes, func(i, j int) bool {
		return scopeKey(scopes[i].scopeType, scopes[i].scopeID) < scopeKey(scopes[j].scopeType, scopes[j].scopeID)
	})
	return scopes
}

func scopeKey(scopeType ScopeType, scopeID string) string {
	return string(scopeType) + ":" + scopeID
}

func resultKey(scopeType ScopeType, scopeID, ruleCode string) string {
	return scopeKey(scopeType, scopeID) + ":" + ruleCode
}

func eventByType(events []Event, eventType string) *Event {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eventType {
			return &events[i]
		}
	}
	return nil
}
