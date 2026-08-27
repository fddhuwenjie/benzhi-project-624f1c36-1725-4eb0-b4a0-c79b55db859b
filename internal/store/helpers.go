package store

import (
	"strings"

	"mastergate/internal/domain"
)

func DraftIdentity(c domain.DeliveryCase) string {
	return strings.ToLower(c.ProgramCode) + "\x00" + strings.ToLower(c.MasterVersion) + "\x00" + c.MasterSHA256
}

func FindMeasurement(data *CaseData, id string) (domain.MeasurementSet, bool) {
	for _, m := range data.Measurements {
		if m.MeasurementID == id {
			return m, true
		}
	}
	return domain.MeasurementSet{}, false
}

func LatestMeasurements(data *CaseData) []domain.MeasurementSet {
	superseded := make(map[string]bool)
	for _, m := range data.Measurements {
		if m.SupersedesID != "" {
			superseded[m.SupersedesID] = true
		}
	}
	result := make([]domain.MeasurementSet, 0)
	for _, m := range data.Measurements {
		if !superseded[m.MeasurementID] {
			result = append(result, m)
		}
	}
	return result
}

func LatestResults(data *CaseData) []domain.RuleResult {
	latest := LatestMeasurements(data)
	ids := make(map[string]bool)
	for _, m := range latest {
		ids[m.MeasurementID] = true
	}
	result := make([]domain.RuleResult, 0)
	for _, r := range data.RuleResults {
		if ids[r.MeasurementID] {
			result = append(result, r)
		}
	}
	return result
}

func LatestMeasurementsForScope(data *CaseData, scope domain.ScopeType, scopeID string) string {
	for _, m := range LatestMeasurements(data) {
		if m.ScopeType == scope && m.ScopeID == scopeID {
			return m.MeasurementID
		}
	}
	return ""
}
