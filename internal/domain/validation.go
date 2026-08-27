package domain

import (
	"encoding/hex"
	"math"
	"regexp"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{1,63}$`)

func ValidateIdentifier(label, value string) error {
	if !identifierPattern.MatchString(value) {
		return NewError(CodeInvalid, "%s 必须为 2 至 64 位技术标识", label)
	}
	return nil
}

func ValidateText(label, value string, max int) error {
	v := strings.TrimSpace(value)
	if v == "" || len([]rune(v)) > max {
		return NewError(CodeInvalid, "%s 不能为空且不得超过 %d 个字符", label, max)
	}
	return nil
}

func ValidateSHA256(label, value string) error {
	if len(value) != 64 {
		return NewError(CodeInvalid, "%s 必须是 64 位 SHA-256 十六进制摘要", label)
	}
	b, err := hex.DecodeString(value)
	if err != nil || len(b) != 32 || strings.ToLower(value) != value {
		return NewError(CodeInvalid, "%s 必须使用小写 SHA-256 十六进制摘要", label)
	}
	return nil
}

func ValidateFinite(label string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return NewError(CodeInvalid, "%s 必须是有限数值", label)
	}
	return nil
}

func ValidateStandard(p StandardProfile) error {
	if err := ValidateText("标准名称", p.Name, 100); err != nil {
		return err
	}
	values := []struct {
		name  string
		value float64
	}{
		{"目标集成响度", p.TargetIntegratedLUFS}, {"集成响度容差", p.IntegratedToleranceLU},
		{"最大响度范围", p.MaxLoudnessRangeLU}, {"最大真峰值", p.MaxTruePeakDBTP},
	}
	for _, item := range values {
		if err := ValidateFinite(item.name, item.value); err != nil {
			return err
		}
	}
	if p.TargetIntegratedLUFS < -70 || p.TargetIntegratedLUFS > 0 {
		return NewError(CodeInvalid, "目标集成响度超出 -70 至 0 LUFS")
	}
	if p.IntegratedToleranceLU <= 0 || p.IntegratedToleranceLU > 10 {
		return NewError(CodeInvalid, "集成响度容差必须大于 0 且不超过 10 LU")
	}
	if p.MaxLoudnessRangeLU <= 0 || p.MaxLoudnessRangeLU > 40 {
		return NewError(CodeInvalid, "最大响度范围必须大于 0 且不超过 40 LU")
	}
	if p.MaxTruePeakDBTP < -20 || p.MaxTruePeakDBTP > 0 {
		return NewError(CodeInvalid, "最大真峰值必须位于 -20 至 0 dBTP")
	}
	if p.ExpectedDurationMillis <= 0 || p.ExpectedDurationMillis > 24*60*60*1000 {
		return NewError(CodeInvalid, "节目总时长必须大于 0 且不超过 24 小时")
	}
	return nil
}

func ValidateSegment(segment ProgramSegment) error {
	if err := ValidateIdentifier("分段标识", segment.SegmentID); err != nil {
		return err
	}
	if err := ValidateIdentifier("案件标识", segment.CaseID); err != nil {
		return err
	}
	if err := ValidateText("分段标题", segment.Title, 120); err != nil {
		return err
	}
	if segment.StartMillis < 0 || segment.EndMillis <= segment.StartMillis {
		return NewError(CodeInvalid, "分段时间范围无效")
	}
	allowed := map[string]bool{"mono": true, "stereo": true, "5.1": true, "7.1": true}
	if !allowed[segment.ChannelLayout] {
		return NewError(CodeInvalid, "声道布局仅支持 mono、stereo、5.1 或 7.1")
	}
	if err := ValidateSHA256("音频摘要", segment.AudioSHA256); err != nil {
		return err
	}
	return ValidateText("校准引用", segment.CalibrationRef, 160)
}

func ValidateMeasurement(m MeasurementSet) error {
	if err := ValidateIdentifier("测量标识", m.MeasurementID); err != nil {
		return err
	}
	if err := ValidateIdentifier("案件标识", m.CaseID); err != nil {
		return err
	}
	if m.ScopeType != ScopeProgram && m.ScopeType != ScopeSegment {
		return NewError(CodeInvalid, "测量范围类型无效")
	}
	if err := ValidateIdentifier("范围标识", m.ScopeID); err != nil {
		return err
	}
	if m.IntegratedUnit != "LUFS" || m.RangeUnit != "LU" || m.PeakUnit != "dBTP" || m.GateUnit != "LU" {
		return NewError(CodeInvalid, "测量单位必须分别为 LUFS、LU、dBTP、LU")
	}
	values := []struct {
		name  string
		value float64
	}{{"集成响度", m.IntegratedLUFS}, {"响度范围", m.LoudnessRangeLU}, {"真峰值", m.TruePeakDBTP}, {"测量门限", m.GateThresholdLU}}
	for _, item := range values {
		if err := ValidateFinite(item.name, item.value); err != nil {
			return err
		}
	}
	if m.IntegratedLUFS < -100 || m.IntegratedLUFS > 10 {
		return NewError(CodeInvalid, "集成响度超出有效范围")
	}
	if m.LoudnessRangeLU < 0 || m.LoudnessRangeLU > 100 {
		return NewError(CodeInvalid, "响度范围超出有效范围")
	}
	if m.TruePeakDBTP < -100 || m.TruePeakDBTP > 10 {
		return NewError(CodeInvalid, "真峰值超出有效范围")
	}
	if m.GateThresholdLU < -100 || m.GateThresholdLU > 0 {
		return NewError(CodeInvalid, "测量门限超出有效范围")
	}
	if err := ValidateSHA256("证据摘要", m.EvidenceSHA256); err != nil {
		return err
	}
	return ValidateIdentifier("提交人", m.SubmittedBy)
}
