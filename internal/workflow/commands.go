package workflow

import (
	"time"

	"mastergate/internal/domain"
)

type Metadata struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}

type CreateCaseCommand struct {
	Metadata
	CaseID        string                 `json:"case_id"`
	ProgramCode   string                 `json:"program_code"`
	MasterVersion string                 `json:"master_version"`
	EngineerID    string                 `json:"engineer_id"`
	MasterSHA256  string                 `json:"master_sha256"`
	Standard      domain.StandardProfile `json:"standard_profile"`
}

type FreezeBaselineCommand struct {
	Metadata
	CaseID  string `json:"case_id"`
	ActorID string `json:"actor_id"`
}
type ReviseCaseCommand struct {
	Metadata
	CaseID        string                 `json:"case_id"`
	ActorID       string                 `json:"actor_id"`
	ProgramCode   string                 `json:"program_code"`
	MasterVersion string                 `json:"master_version"`
	EngineerID    string                 `json:"engineer_id"`
	MasterSHA256  string                 `json:"master_sha256"`
	Standard      domain.StandardProfile `json:"standard_profile"`
}
type AddSegmentCommand struct {
	Metadata
	CaseID  string                `json:"case_id"`
	ActorID string                `json:"actor_id"`
	Segment domain.ProgramSegment `json:"segment"`
}
type ReviseSegmentCommand struct {
	Metadata
	CaseID    string                `json:"case_id"`
	ActorID   string                `json:"actor_id"`
	SegmentID string                `json:"segment_id"`
	Segment   domain.ProgramSegment `json:"segment"`
}
type WithdrawSegmentCommand struct {
	Metadata
	CaseID    string `json:"case_id"`
	ActorID   string `json:"actor_id"`
	SegmentID string `json:"segment_id"`
}
type SubmitMeasurementCommand struct {
	Metadata
	CaseID      string                `json:"case_id"`
	ActorID     string                `json:"actor_id"`
	Measurement domain.MeasurementSet `json:"measurement"`
}
type SubmitMeasurementBatchCommand struct {
	Metadata
	CaseID       string                  `json:"case_id"`
	ActorID      string                  `json:"actor_id"`
	Measurements []domain.MeasurementSet `json:"measurements"`
}
type EvaluateCommand struct {
	Metadata
	CaseID  string `json:"case_id"`
	ActorID string `json:"actor_id"`
}
type CorrectDeviationCommand struct {
	Metadata
	CaseID                 string `json:"case_id"`
	ActorID                string `json:"actor_id"`
	DeviationID            string `json:"deviation_id"`
	RootCause              string `json:"root_cause"`
	CorrectionSummary      string `json:"correction_summary"`
	ReplacementAudioSHA256 string `json:"replacement_audio_sha256"`
}
type CorrectDeviationsCommand struct {
	Metadata
	CaseID                 string   `json:"case_id"`
	ActorID                string   `json:"actor_id"`
	DeviationIDs           []string `json:"deviation_ids"`
	RootCause              string   `json:"root_cause"`
	CorrectionSummary      string   `json:"correction_summary"`
	ReplacementAudioSHA256 string   `json:"replacement_audio_sha256"`
}
type RetestCommand struct {
	Metadata
	CaseID              string                `json:"case_id"`
	ActorID             string                `json:"actor_id"`
	DeviationID         string                `json:"deviation_id"`
	FailedMeasurementID string                `json:"failed_measurement_id"`
	RuleCode            string                `json:"rule_code"`
	ScopeType           domain.ScopeType      `json:"scope_type"`
	ScopeID             string                `json:"scope_id"`
	Measurement         domain.MeasurementSet `json:"measurement"`
}
type JointRetestCommand struct {
	Metadata
	CaseID       string                `json:"case_id"`
	ActorID      string                `json:"actor_id"`
	DeviationIDs []string              `json:"deviation_ids"`
	Measurement  domain.MeasurementSet `json:"measurement"`
}
type ReviewCommand struct {
	Metadata
	CaseID          string                    `json:"case_id"`
	ActorID         string                    `json:"actor_id,omitempty"`
	ReviewerID      string                    `json:"reviewer_id"`
	Decision        string                    `json:"decision"`
	Checks          []string                  `json:"checks,omitempty"`
	Annotations     []domain.ReviewAnnotation `json:"annotations"`
	RejectionCode   string                    `json:"rejection_code,omitempty"`
	RejectionDetail string                    `json:"rejection_detail,omitempty"`
}

type CommandResult struct {
	Case     domain.DeliveryCase      `json:"case"`
	Manifest *domain.DeliveryManifest `json:"manifest,omitempty"`
}

type CaseView struct {
	Case             domain.DeliveryCase          `json:"case"`
	Segments         []domain.ProgramSegment      `json:"segments"`
	Measurements     []domain.MeasurementSet      `json:"measurements"`
	RuleResults      []domain.RuleResult          `json:"rule_results"`
	RuleSnapshots    []domain.RuleSnapshot        `json:"rule_snapshots"`
	Deviations       []domain.Deviation           `json:"deviations"`
	Coverage         domain.SegmentCoverageReport `json:"coverage"`
	DeviationQueue   []DeviationQueueItem         `json:"deviation_queue"`
	DeviationSummary DeviationSummary             `json:"deviation_summary"`
	Events           []domain.Event               `json:"events"`
	Manifest         *domain.DeliveryManifest     `json:"manifest,omitempty"`
	Integrity        string                       `json:"integrity"`
}

type DeviationQueueItem struct {
	domain.Deviation
	Priority       int    `json:"priority"`
	PriorityLabel  string `json:"priority_label"`
	BlocksApproval bool   `json:"blocks_approval"`
	Overdue        bool   `json:"overdue"`
	OverdueReason  string `json:"overdue_reason,omitempty"`
}

type DeviationSummary struct {
	OpenCount      int `json:"open_count"`
	RetestDueCount int `json:"retest_due_count"`
	OverdueCount   int `json:"overdue_count"`
}

type CaseListFilter struct {
	ProgramCode  string
	State        domain.CaseState
	EngineerID   string
	ApprovedFrom *time.Time
	ApprovedTo   *time.Time
}

type CaseListItem struct {
	domain.DeliveryCase
	HasValidManifest bool `json:"has_valid_manifest"`
}

type ManifestVerification struct {
	CaseID           string              `json:"case_id"`
	Valid            bool                `json:"valid"`
	ManifestSHA256   string              `json:"manifest_sha256,omitempty"`
	EventChainHead   string              `json:"event_chain_head,omitempty"`
	Message          string              `json:"message"`
	CanonicalPayload any                 `json:"canonical_payload,omitempty"`
	Checks           []VerificationCheck `json:"checks"`
	FailureLocation  string              `json:"failure_location,omitempty"`
}

type VerificationCheck struct {
	Field   string `json:"field"`
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}
