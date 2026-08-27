package store

import "mastergate/internal/domain"

type CaseData struct {
	Case          domain.DeliveryCase      `json:"case"`
	Segments      []domain.ProgramSegment  `json:"segments"`
	Measurements  []domain.MeasurementSet  `json:"measurements"`
	RuleResults   []domain.RuleResult      `json:"rule_results"`
	RuleSnapshots []domain.RuleSnapshot    `json:"rule_snapshots"`
	Deviations    []domain.Deviation       `json:"deviations"`
	Events        []domain.Event           `json:"events"`
	Manifest      *domain.DeliveryManifest `json:"manifest,omitempty"`
}

type snapshot struct {
	Cases       map[string]*CaseData                `json:"cases"`
	Idempotency map[string]domain.IdempotencyRecord `json:"idempotency"`
}

func emptySnapshot() snapshot {
	return snapshot{Cases: make(map[string]*CaseData), Idempotency: make(map[string]domain.IdempotencyRecord)}
}

type Mutation struct {
	RequestID        string
	Fingerprint      string
	CaseID           string
	ExpectedRevision int64
	ActorID          string
	EventType        string
	StatusCode       int
	Create           bool
	DraftIdentity    string
}

type Result struct {
	Response   []byte
	StatusCode int
	Replayed   bool
}

type Change struct {
	Response  any
	EventData any
	Finalize  func(*CaseData, domain.Event) (any, error)
}
