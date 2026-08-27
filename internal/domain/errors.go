package domain

import "fmt"

type ErrorCode string

const (
	CodeInvalid    ErrorCode = "invalid_input"
	CodeState      ErrorCode = "invalid_state"
	CodeNotFound   ErrorCode = "not_found"
	CodeConflict   ErrorCode = "revision_conflict"
	CodeIdempotent ErrorCode = "idempotency_conflict"
	CodeForbidden  ErrorCode = "forbidden"
	CodeIntegrity  ErrorCode = "integrity_failure"
)

type Error struct {
	Code            ErrorCode        `json:"code"`
	Message         string           `json:"message"`
	CurrentRevision int64            `json:"current_revision,omitempty"`
	LatestBlockers  []ReadinessCheck `json:"latest_blockers,omitempty"`
}

func ConflictWithReadiness(current int64, blockers []ReadinessCheck) error {
	return &Error{Code: CodeConflict, Message: "案件修订号已变化，请刷新就绪度后重试", CurrentRevision: current, LatestBlockers: append([]ReadinessCheck(nil), blockers...)}
}

func (e *Error) Error() string { return e.Message }

func NewError(code ErrorCode, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func Conflict(current int64) error {
	return &Error{Code: CodeConflict, Message: "案件修订号已变化，请刷新后重试", CurrentRevision: current}
}

func ErrorCodeOf(err error) ErrorCode {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return "internal"
}
