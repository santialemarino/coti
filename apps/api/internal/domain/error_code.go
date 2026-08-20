package domain

import "errors"

// ErrorCode is the stable identifier a client branches on, alongside the status. One route can
// answer 422 for several reasons, so the status alone stops being enough the day a second reason is
// added — and the wording stays the client's to choose.
type ErrorCode string

// What an error of each kind carries unless a call site tags something more specific.
const (
	CodeNotFound         ErrorCode = "NOT_FOUND"
	CodeConflict         ErrorCode = "CONFLICT"
	CodeInvalidInput     ErrorCode = "INVALID_INPUT"
	CodeUnauthenticated  ErrorCode = "UNAUTHENTICATED"
	CodeForbidden        ErrorCode = "FORBIDDEN"
	CodeImmutable        ErrorCode = "IMMUTABLE"
	CodeLocked           ErrorCode = "ACCOUNT_LOCKED"
	CodeEmailNotVerified ErrorCode = "EMAIL_NOT_VERIFIED"
	CodeRateLimited      ErrorCode = "RATE_LIMITED"
	CodeAIUnavailable    ErrorCode = "AI_UNAVAILABLE"
	CodeInternal         ErrorCode = "INTERNAL"
)

// The refusals a screen has to tell apart from a sibling answering the same status.
const (
	CodeEmailTaken       ErrorCode = "EMAIL_TAKEN"
	CodeLastActiveBranch ErrorCode = "LAST_ACTIVE_BRANCH"
	CodeSelfDeactivation ErrorCode = "SELF_DEACTIVATION"
	CodeSelfRoleChange   ErrorCode = "SELF_ROLE_CHANGE"
	CodePasswordPolicy   ErrorCode = "PASSWORD_POLICY"
	CodeInvalidLink      ErrorCode = "INVALID_LINK"
	CodeQuoteArchived    ErrorCode = "QUOTE_ARCHIVED"
	CodeQuoteNotDraft    ErrorCode = "QUOTE_NOT_DRAFT"
	CodeLinkExpired      ErrorCode = "LINK_EXPIRED"
	// CodeUnsupportedFileType sits beside CodeFileTooLarge: both refuse an upload, and a client
	// offering a different file needs to know which rule it broke.
	CodeUnsupportedFileType ErrorCode = "UNSUPPORTED_FILE_TYPE"
)

// The two an upload is refused with. The delivery layer raises both on its own, before any
// service is reached; CodeFileTooLarge also answers ErrTooLarge, so the same refusal reads the
// same whether the transport or a service caught it.
const (
	CodeInvalidBody  ErrorCode = "INVALID_BODY"
	CodeFileTooLarge ErrorCode = "FILE_TOO_LARGE"
)

// codedError carries the tag without hiding the sentinel underneath it.
type codedError struct {
	code ErrorCode
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }

func (e *codedError) Unwrap() error { return e.err }

// WithCode tags an error with the code a client branches on. The sentinel it wraps still decides
// the status, so errors.Is keeps working on everything above it.
func WithCode(code ErrorCode, err error) error {
	return &codedError{code: code, err: err}
}

// CodeOf returns the tagged code, or the one the sentinel implies when nothing tagged it.
func CodeOf(err error) ErrorCode {
	var coded *codedError
	if errors.As(err, &coded) {
		return coded.code
	}

	switch {
	case errors.Is(err, ErrTooLarge):
		return CodeFileTooLarge
	case errors.Is(err, ErrNotFound):
		return CodeNotFound
	case errors.Is(err, ErrImmutable):
		return CodeImmutable
	case errors.Is(err, ErrConflict):
		return CodeConflict
	case errors.Is(err, ErrInvalidInput):
		return CodeInvalidInput
	case errors.Is(err, ErrUnauthenticated):
		return CodeUnauthenticated
	case errors.Is(err, ErrEmailNotVerified):
		return CodeEmailNotVerified
	case errors.Is(err, ErrLocked):
		return CodeLocked
	case errors.Is(err, ErrForbidden):
		return CodeForbidden
	case errors.Is(err, ErrRateLimited):
		return CodeRateLimited
	case errors.Is(err, ErrAIUnavailable):
		return CodeAIUnavailable
	default:
		return CodeInternal
	}
}
