package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

type ManifestPayload struct {
	ManifestID        string           `json:"manifest_id"`
	CaseID            string           `json:"case_id"`
	ProgramCode       string           `json:"program_code"`
	MasterVersion     string           `json:"master_version"`
	MasterSHA256      string           `json:"master_sha256"`
	FinalMeasurements []MeasurementSet `json:"final_measurements"`
	RuleResults       []RuleResult     `json:"rule_results"`
	RuleResultDigest  string           `json:"rule_result_digest"`
	ReviewerID        string           `json:"reviewer_id"`
	ApprovedAt        string           `json:"approved_at"`
	EventChainHead    string           `json:"event_chain_head"`
}

type manifestPayload = ManifestPayload

func digestJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func CreateManifest(c DeliveryCase, measurements []MeasurementSet, results []RuleResult, reviewer, chainHead string, now time.Time) (DeliveryManifest, error) {
	if c.State != StateApproved {
		return DeliveryManifest{}, NewError(CodeState, "仅批准案件可生成交付清单")
	}
	if err := ValidateSHA256("事件链尾摘要", chainHead); err != nil {
		return DeliveryManifest{}, err
	}
	sort.Slice(measurements, func(i, j int) bool {
		if measurements[i].ScopeType == measurements[j].ScopeType {
			return measurements[i].ScopeID < measurements[j].ScopeID
		}
		return measurements[i].ScopeType < measurements[j].ScopeType
	})
	sort.Slice(results, func(i, j int) bool {
		left := string(results[i].ScopeType) + ":" + results[i].ScopeID + ":" + results[i].RuleCode
		right := string(results[j].ScopeType) + ":" + results[j].ScopeID + ":" + results[j].RuleCode
		return left < right
	})
	ruleDigest, err := digestJSON(results)
	if err != nil {
		return DeliveryManifest{}, err
	}
	manifest := DeliveryManifest{ManifestID: "manifest-" + c.CaseID, CaseID: c.CaseID, ProgramCode: c.ProgramCode, MasterVersion: c.MasterVersion, MasterSHA256: c.MasterSHA256, FinalMeasurements: measurements, RuleResults: results, RuleResultDigest: ruleDigest, ReviewerID: reviewer, ApprovedAt: now.UTC(), EventChainHead: chainHead}
	digest, err := ManifestDigest(manifest)
	if err != nil {
		return DeliveryManifest{}, err
	}
	manifest.ManifestSHA256 = digest
	return manifest, nil
}

func ManifestDigest(m DeliveryManifest) (string, error) {
	p := CanonicalManifestPayload(m)
	return digestJSON(p)
}

func CanonicalManifestPayload(m DeliveryManifest) ManifestPayload {
	return ManifestPayload{ManifestID: m.ManifestID, CaseID: m.CaseID, ProgramCode: m.ProgramCode, MasterVersion: m.MasterVersion, MasterSHA256: m.MasterSHA256, FinalMeasurements: m.FinalMeasurements, RuleResults: m.RuleResults, RuleResultDigest: m.RuleResultDigest, ReviewerID: m.ReviewerID, ApprovedAt: m.ApprovedAt.UTC().Format(time.RFC3339Nano), EventChainHead: m.EventChainHead}
}

func VerifyManifest(m DeliveryManifest) error {
	if err := VerifyRuleResultDigest(m); err != nil {
		return err
	}
	digest, err := ManifestDigest(m)
	if err != nil {
		return err
	}
	if digest != m.ManifestSHA256 {
		return NewError(CodeIntegrity, "交付清单 SHA-256 校验失败")
	}
	return nil
}

func VerifyRuleResultDigest(m DeliveryManifest) error {
	rules, err := digestJSON(m.RuleResults)
	if err != nil {
		return err
	}
	if rules != m.RuleResultDigest {
		return NewError(CodeIntegrity, "清单规则结果摘要不匹配")
	}
	return nil
}
