package domain

import "sort"

func InspectSegmentCoverage(segments []ProgramSegment, duration int64) SegmentCoverageReport {
	report := SegmentCoverageReport{ExpectedDurationMillis: duration, SortedSegments: append([]ProgramSegment(nil), segments...)}
	sort.Slice(report.SortedSegments, func(i, j int) bool {
		if report.SortedSegments[i].StartMillis == report.SortedSegments[j].StartMillis {
			return report.SortedSegments[i].SegmentID < report.SortedSegments[j].SegmentID
		}
		return report.SortedSegments[i].StartMillis < report.SortedSegments[j].StartMillis
	})
	for _, segment := range report.SortedSegments {
		if err := ValidateSegment(segment); err != nil {
			report.SegmentIssues = append(report.SegmentIssues, SegmentIssue{SegmentID: segment.SegmentID, Errors: []string{err.Error()}})
		}
	}
	if len(report.SortedSegments) == 0 {
		report.Message = "尚未登记节目分段"
		return report
	}
	report.FirstStartMillis = report.SortedSegments[0].StartMillis
	report.LastEndMillis = report.SortedSegments[len(report.SortedSegments)-1].EndMillis
	if report.FirstStartMillis > 0 {
		report.Gaps = append(report.Gaps, CoverageInterval{StartMillis: 0, EndMillis: report.FirstStartMillis})
	}
	frontier := report.SortedSegments[0].EndMillis
	for _, segment := range report.SortedSegments[1:] {
		if segment.StartMillis > frontier {
			report.Gaps = append(report.Gaps, CoverageInterval{StartMillis: frontier, EndMillis: segment.StartMillis})
		} else if segment.StartMillis < frontier {
			end := segment.EndMillis
			if end > frontier {
				end = frontier
			}
			report.Overlaps = append(report.Overlaps, CoverageInterval{StartMillis: segment.StartMillis, EndMillis: end})
		}
		if segment.EndMillis > frontier {
			frontier = segment.EndMillis
		}
	}
	if frontier < duration {
		report.Gaps = append(report.Gaps, CoverageInterval{StartMillis: frontier, EndMillis: duration})
	}
	if frontier > duration {
		report.Overlaps = append(report.Overlaps, CoverageInterval{StartMillis: duration, EndMillis: frontier})
	}
	report.LastEndMillis = frontier
	report.Valid = report.FirstStartMillis == 0 && frontier == duration && len(report.Gaps) == 0 && len(report.Overlaps) == 0 && len(report.SegmentIssues) == 0
	if report.Valid {
		report.Message = "分段覆盖连续且证据字段有效"
	} else {
		report.Message = "分段覆盖或证据字段未通过预检"
	}
	return report
}

func ValidateSegmentsComplete(segments []ProgramSegment, duration int64) error {
	report := InspectSegmentCoverage(segments, duration)
	if report.Valid {
		return nil
	}
	if len(report.SegmentIssues) > 0 {
		return NewError(CodeInvalid, "分段 %s 字段无效：%s", report.SegmentIssues[0].SegmentID, report.SegmentIssues[0].Errors[0])
	}
	if len(report.Gaps) > 0 {
		gap := report.Gaps[0]
		return NewError(CodeInvalid, "分段覆盖存在 %d-%d 毫秒缺口", gap.StartMillis, gap.EndMillis)
	}
	overlap := report.Overlaps[0]
	return NewError(CodeInvalid, "分段覆盖存在 %d-%d 毫秒重叠或越界", overlap.StartMillis, overlap.EndMillis)
}

// ValidateSegmentCollection validates a mutable segmentation plan without
// requiring it to be complete yet. Coverage completeness remains a preflight
// requirement before rule evaluation.
func ValidateSegmentCollection(segments []ProgramSegment, caseID string, duration int64) error {
	seen := make(map[string]bool, len(segments))
	ordered := append([]ProgramSegment(nil), segments...)
	for _, segment := range ordered {
		if segment.CaseID != caseID {
			return NewError(CodeInvalid, "分段 %s 的 case_id 与案件不一致", segment.SegmentID)
		}
		if seen[segment.SegmentID] {
			return NewError(CodeConflict, "分段标识 %s 重复", segment.SegmentID)
		}
		seen[segment.SegmentID] = true
		if err := ValidateSegment(segment); err != nil {
			return err
		}
		if segment.EndMillis > duration {
			return NewError(CodeInvalid, "分段 %s 的结束时间超过节目总时长", segment.SegmentID)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].StartMillis == ordered[j].StartMillis {
			return ordered[i].SegmentID < ordered[j].SegmentID
		}
		return ordered[i].StartMillis < ordered[j].StartMillis
	})
	for i := 1; i < len(ordered); i++ {
		if ordered[i].StartMillis < ordered[i-1].EndMillis {
			return NewError(CodeInvalid, "分段 %s 与 %s 的时间范围重叠", ordered[i-1].SegmentID, ordered[i].SegmentID)
		}
	}
	return nil
}
