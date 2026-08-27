package domain

import "time"

type CaseState string

const (
	StateDraft       CaseState = "draft"
	StateBaseline    CaseState = "baseline_frozen"
	StateMeasuring   CaseState = "measuring"
	StateRemediation CaseState = "remediation"
	StateReview      CaseState = "pending_review"
	StateApproved    CaseState = "approved"
	StateRejected    CaseState = "rejected"
)

func (s CaseState) Terminal() bool { return s == StateApproved || s == StateRejected }

type StandardProfile struct {
	Name                   string  `json:"name"`
	TargetIntegratedLUFS   float64 `json:"target_integrated_lufs"`
	IntegratedToleranceLU  float64 `json:"integrated_tolerance_lu"`
	MaxLoudnessRangeLU     float64 `json:"max_loudness_range_lu"`
	MaxTruePeakDBTP        float64 `json:"max_true_peak_dbtp"`
	ExpectedDurationMillis int64   `json:"expected_duration_millis"`
}

type DeliveryCase struct {
	CaseID            string             `json:"case_id"`
	ProgramCode       string             `json:"program_code"`
	MasterVersion     string             `json:"master_version"`
	EngineerID        string             `json:"engineer_id"`
	MasterSHA256      string             `json:"master_sha256"`
	StandardProfile   StandardProfile    `json:"standard_profile"`
	State             CaseState          `json:"state"`
	Revision          int64              `json:"revision"`
	CreatedAt         time.Time          `json:"created_at"`
	BaselineFrozenAt  *time.Time         `json:"baseline_frozen_at,omitempty"`
	ReviewedBy        string             `json:"reviewed_by,omitempty"`
	ReviewChecks      []string           `json:"review_checks,omitempty"`
	ReviewAnnotations []ReviewAnnotation `json:"review_annotations,omitempty"`
	RejectionCode     string             `json:"rejection_code,omitempty"`
	RejectionDetail   string             `json:"rejection_detail,omitempty"`
}

type ReviewAnnotation struct {
	CheckType   string `json:"check_type"`
	Comment     string `json:"comment"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
}

type ProgramSegment struct {
	SegmentID      string `json:"segment_id"`
	CaseID         string `json:"case_id"`
	Title          string `json:"title"`
	StartMillis    int64  `json:"start_millis"`
	EndMillis      int64  `json:"end_millis"`
	ChannelLayout  string `json:"channel_layout"`
	AudioSHA256    string `json:"audio_sha256"`
	CalibrationRef string `json:"calibration_ref"`
}

type ScopeType string

const (
	ScopeProgram ScopeType = "program"
	ScopeSegment ScopeType = "segment"
)

type MeasurementSet struct {
	MeasurementID   string    `json:"measurement_id"`
	CaseID          string    `json:"case_id"`
	SupersedesID    string    `json:"supersedes_id,omitempty"`
	ScopeType       ScopeType `json:"scope_type"`
	ScopeID         string    `json:"scope_id"`
	IntegratedLUFS  float64   `json:"integrated_lufs"`
	LoudnessRangeLU float64   `json:"loudness_range_lu"`
	TruePeakDBTP    float64   `json:"true_peak_dbtp"`
	GateThresholdLU float64   `json:"gate_threshold_lu"`
	IntegratedUnit  string    `json:"integrated_unit"`
	RangeUnit       string    `json:"range_unit"`
	PeakUnit        string    `json:"peak_unit"`
	GateUnit        string    `json:"gate_unit"`
	EvidenceSHA256  string    `json:"evidence_sha256"`
	SubmittedBy     string    `json:"submitted_by"`
	SubmittedAt     time.Time `json:"submitted_at"`
}

type RuleResult struct {
	ResultID      string    `json:"result_id"`
	CaseID        string    `json:"case_id"`
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
	EvaluatedAt   time.Time `json:"evaluated_at"`
}

type RuleDifference struct {
	ScopeType      ScopeType `json:"scope_type"`
	ScopeID        string    `json:"scope_id"`
	RuleCode       string    `json:"rule_code"`
	PreviousPassed *bool     `json:"previous_passed,omitempty"`
	CurrentPassed  bool      `json:"current_passed"`
	Affected       bool      `json:"affected"`
}

type RuleSnapshot struct {
	SnapshotID     string           `json:"snapshot_id"`
	CaseID         string           `json:"case_id"`
	ResultDigest   string           `json:"result_digest"`
	PreviousDigest string           `json:"previous_digest,omitempty"`
	Results        []RuleResult     `json:"results"`
	Differences    []RuleDifference `json:"differences"`
	EvaluatedAt    time.Time        `json:"evaluated_at"`
}

type DeviationState string

const (
	DeviationOpen      DeviationState = "open"
	DeviationRetestDue DeviationState = "retest_due"
	DeviationReviewDue DeviationState = "pending_review"
)

type Deviation struct {
	DeviationID            string          `json:"deviation_id"`
	CaseID                 string          `json:"case_id"`
	RuleCode               string          `json:"rule_code"`
	ScopeType              ScopeType       `json:"scope_type"`
	ScopeID                string          `json:"scope_id"`
	FailedMeasurementID    string          `json:"failed_measurement_id"`
	RootCause              string          `json:"root_cause,omitempty"`
	CorrectionSummary      string          `json:"correction_summary,omitempty"`
	ReplacementAudioSHA256 string          `json:"replacement_audio_sha256,omitempty"`
	RetestMeasurementID    string          `json:"retest_measurement_id,omitempty"`
	State                  DeviationState  `json:"state"`
	CreatedAt              time.Time       `json:"created_at"`
	CorrectionRecordedAt   *time.Time      `json:"correction_recorded_at,omitempty"`
	LastRetestAt           *time.Time      `json:"last_retest_at,omitempty"`
	FailedRetestIDs        []string        `json:"failed_retest_ids,omitempty"`
	RetestHistory          []RetestAttempt `json:"retest_history,omitempty"`
	ResolvedAt             *time.Time      `json:"resolved_at,omitempty"`
}

type RetestAttempt struct {
	MeasurementID string    `json:"measurement_id"`
	Passed        bool      `json:"passed"`
	SubmittedAt   time.Time `json:"submitted_at"`
}

type SegmentIssue struct {
	SegmentID string   `json:"segment_id"`
	Errors    []string `json:"errors"`
}

type CoverageInterval struct {
	StartMillis int64 `json:"start_millis"`
	EndMillis   int64 `json:"end_millis"`
}

type SegmentCoverageReport struct {
	Valid                  bool               `json:"valid"`
	ExpectedDurationMillis int64              `json:"expected_duration_millis"`
	FirstStartMillis       int64              `json:"first_start_millis"`
	LastEndMillis          int64              `json:"last_end_millis"`
	SortedSegments         []ProgramSegment   `json:"sorted_segments"`
	Gaps                   []CoverageInterval `json:"gaps"`
	Overlaps               []CoverageInterval `json:"overlaps"`
	SegmentIssues          []SegmentIssue     `json:"segment_issues"`
	Message                string             `json:"message"`
}

type Event struct {
	Sequence       int64     `json:"sequence"`
	CaseID         string    `json:"case_id"`
	Type           string    `json:"type"`
	ActorID        string    `json:"actor_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	Revision       int64     `json:"revision"`
	Data           []byte    `json:"data"`
	PreviousDigest string    `json:"previous_digest"`
	Digest         string    `json:"digest"`
}

type DeliveryManifest struct {
	ManifestID        string           `json:"manifest_id"`
	CaseID            string           `json:"case_id"`
	ProgramCode       string           `json:"program_code"`
	MasterVersion     string           `json:"master_version"`
	MasterSHA256      string           `json:"master_sha256"`
	FinalMeasurements []MeasurementSet `json:"final_measurements"`
	RuleResults       []RuleResult     `json:"rule_results"`
	RuleResultDigest  string           `json:"rule_result_digest"`
	ReviewerID        string           `json:"reviewer_id"`
	ApprovedAt        time.Time        `json:"approved_at"`
	EventChainHead    string           `json:"event_chain_head"`
	ManifestSHA256    string           `json:"manifest_sha256"`
}

type IdempotencyRecord struct {
	RequestID   string `json:"request_id"`
	Fingerprint string `json:"fingerprint"`
	Response    []byte `json:"response"`
	StatusCode  int    `json:"status_code"`
}

type ReadinessCheck struct {
	Category      string    `json:"category"`
	Code          string    `json:"code"`
	Passed        bool      `json:"passed"`
	Blocking      bool      `json:"blocking"`
	Message       string    `json:"message"`
	ScopeType     ScopeType `json:"scope_type,omitempty"`
	ScopeID       string    `json:"scope_id,omitempty"`
	MeasurementID string    `json:"measurement_id,omitempty"`
	SnapshotID    string    `json:"snapshot_id,omitempty"`
	DeviationID   string    `json:"deviation_id,omitempty"`
	EventDigest   string    `json:"event_digest,omitempty"`
	EventSummary  string    `json:"event_summary,omitempty"`
}

type ReadinessCategory struct {
	Category string           `json:"category"`
	Label    string           `json:"label"`
	Passed   bool             `json:"passed"`
	Checks   []ReadinessCheck `json:"checks"`
}

type ReadinessReport struct {
	CaseID     string              `json:"case_id"`
	Revision   int64               `json:"revision"`
	Ready      bool                `json:"ready"`
	Categories []ReadinessCategory `json:"categories"`
	Blockers   []ReadinessCheck    `json:"blockers"`
	Hints      []ReadinessCheck    `json:"hints"`
}
