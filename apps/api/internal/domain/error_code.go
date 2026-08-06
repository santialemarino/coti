package domain

import "errors"

/*
 * The status says how a request failed; the code says which rule refused it. One route can answer
 * 422 for several reasons, and a client that has to guess from the status alone guesses wrong the
 * day a second reason is added — so every refusal a caller is expected to act on carries a stable
 * identifier, and the wording stays the client's to choose.
 */
type ErrorCode string

// The code every error of that kind carries unless a call site tags a more specific one.
const (
	CodeNotFound         ErrorCode = "NOT_FOUND"
	CodeConflict         ErrorCode = "CONFLICT"
	CodeInvalidInput     ErrorCode = "INVALID_INPUT"
	CodeInvalidBody      ErrorCode = "INVALID_BODY"
	CodeUnauthenticated  ErrorCode = "UNAUTHENTICATED"
	CodeForbidden        ErrorCode = "FORBIDDEN"
	CodeImmutable        ErrorCode = "IMMUTABLE"
	CodeLocked           ErrorCode = "ACCOUNT_LOCKED"
	CodeEmailNotVerified ErrorCode = "EMAIL_NOT_VERIFIED"
	CodeRateLimited      ErrorCode = "RATE_LIMITED"
	CodeInternal         ErrorCode = "INTERNAL"
)

// The refusals a screen has to tell apart from their siblings on the same status.
const (
	CodeEmailTaken       ErrorCode = "EMAIL_TAKEN"
	CodeLastActiveBranch ErrorCode = "LAST_ACTIVE_BRANCH"
	CodeSelfDeactivation ErrorCode = "SELF_DEACTIVATION"
	CodeSelfRoleChange   ErrorCode = "SELF_ROLE_CHANGE"
	CodePasswordPolicy   ErrorCode = "PASSWORD_POLICY"
	CodeInvalidLink      ErrorCode = "INVALID_LINK"
	CodeFileTooLarge     ErrorCode = "FILE_TOO_LARGE"
)

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
	case errors.Is(err, ErrNotFound):
		return CodeNotFound
	case errors.Is(err, ErrConflict), errors.Is(err, ErrImmutable):
		return codeForConflict(err)
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
	default:
		return CodeInternal
	}
}

func codeForConflict(err error) ErrorCode {
	if errors.Is(err, ErrImmutable) {
		return CodeImmutable
	}
	return CodeConflict
}
