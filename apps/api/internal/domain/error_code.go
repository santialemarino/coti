package domain

import "errors"

// ErrorCode is the stable identifier a client branches on, alongside the status. One route can
// answer 422 for several reasons, so the status alone stops being enough the day a second reason is
// added — and the wording stays the client's to choose.
type ErrorCode string

// What an error of each kind carries unless a call site tags something more specific.
const (
	CodeNotFound                  ErrorCode = "NOT_FOUND"
	CodeConflict                  ErrorCode = "CONFLICT"
	CodeInvalidInput              ErrorCode = "INVALID_INPUT"
	CodeUnauthenticated           ErrorCode = "UNAUTHENTICATED"
	CodeForbidden                 ErrorCode = "FORBIDDEN"
	CodeImmutable                 ErrorCode = "IMMUTABLE"
	CodeLocked                    ErrorCode = "ACCOUNT_LOCKED"
	CodeEmailNotVerified          ErrorCode = "EMAIL_NOT_VERIFIED"
	CodeRateLimited               ErrorCode = "RATE_LIMITED"
	CodeAIUnavailable             ErrorCode = "AI_UNAVAILABLE"
	CodeNotConfigured             ErrorCode = "NOT_CONFIGURED"
	CodeDeliveryUnavailable       ErrorCode = "DELIVERY_UNAVAILABLE"
	CodeRepresentationUnavailable ErrorCode = "REPRESENTATION_UNAVAILABLE"
	CodeInternal                  ErrorCode = "INTERNAL"
)

// The refusals a screen has to tell apart from a sibling answering the same status.
const (
	CodeEmailTaken            ErrorCode = "EMAIL_TAKEN"
	CodeLastActiveBranch      ErrorCode = "LAST_ACTIVE_BRANCH"
	CodeSelfDeactivation      ErrorCode = "SELF_DEACTIVATION"
	CodeSelfRoleChange        ErrorCode = "SELF_ROLE_CHANGE"
	CodeSelfEmailChange       ErrorCode = "SELF_EMAIL_CHANGE"
	CodePasswordPolicy        ErrorCode = "PASSWORD_POLICY"
	CodeInvalidLink           ErrorCode = "INVALID_LINK"
	CodeInviteNotPending      ErrorCode = "INVITE_NOT_PENDING"
	CodeMailNotConfigured     ErrorCode = "MAIL_NOT_CONFIGURED"
	CodeBranchNotAccessible   ErrorCode = "BRANCH_NOT_ACCESSIBLE"
	CodeBranchMailboxRequired ErrorCode = "BRANCH_MAILBOX_REQUIRED"
	CodeQuoteArchived         ErrorCode = "QUOTE_ARCHIVED"
	CodeQuoteNotDraft         ErrorCode = "QUOTE_NOT_DRAFT"
	CodeQuoteNotReactivatable ErrorCode = "QUOTE_NOT_REACTIVATABLE"
	// CodeNoAIChanges refuses discarding the AI's changes on a draft that has none pending.
	CodeNoAIChanges ErrorCode = "NO_AI_CHANGES"
	// CodeBatchNotRetryable refuses retrying a batch that did not run out of attempts, or whose
	// quote the handler may no longer act on.
	CodeBatchNotRetryable ErrorCode = "BATCH_NOT_RETRYABLE"
	// CodeDiscountNotApplicable refuses restoring a promotion the quote no longer earns.
	CodeDiscountNotApplicable ErrorCode = "DISCOUNT_NOT_APPLICABLE"
	CodeLinkExpired           ErrorCode = "LINK_EXPIRED"
	// CodeUnsupportedFileType sits beside CodeFileTooLarge: both refuse an upload, and a client
	// offering a different file needs to know which rule it broke.
	CodeUnsupportedFileType ErrorCode = "UNSUPPORTED_FILE_TYPE"
	// CodeLegacyExcelFile refuses a legacy .xls workbook, so a client can ask for it as .xlsx.
	CodeLegacyExcelFile ErrorCode = "LEGACY_EXCEL_FILE"
	// The three a channel write is refused with. A form editing one channel can answer all three
	// on the same 422, and the field it has to point the administrator at differs per code.
	CodeChannelConfigShape ErrorCode = "CHANNEL_CONFIG_SHAPE"
	CodeChannelIdentifier  ErrorCode = "CHANNEL_IDENTIFIER"
	CodeManualEntryChannel ErrorCode = "MANUAL_ENTRY_CHANNEL"
	CodeQuoteNotSendable   ErrorCode = "QUOTE_NOT_SENDABLE"
	// CodeQuoteNotSent refuses a customer answering a quote that is not out for an answer.
	CodeQuoteNotSent               ErrorCode = "QUOTE_NOT_SENT"
	CodeQuoteSendExpired           ErrorCode = "QUOTE_SEND_EXPIRED"
	CodeDeliveryChannel            ErrorCode = "DELIVERY_CHANNEL"
	CodeIdempotencyMismatch        ErrorCode = "IDEMPOTENCY_MISMATCH"
	CodeQuoteRepresentationInvalid ErrorCode = "QUOTE_REPRESENTATION_INVALID"
	// CodeQuoteItemsUnresolved refuses sending a quote with a line that has no confirmed,
	// priced product.
	CodeQuoteItemsUnresolved ErrorCode = "QUOTE_ITEMS_UNRESOLVED"
	// Invoicing. INVOICE_NOT_READY carries the missing fiscal data as issues; INVOICE_REJECTED
	// carries ARCA's own reasons.
	CodeInvoiceNotReady      ErrorCode = "INVOICE_NOT_READY"
	CodeInvoiceRejected      ErrorCode = "INVOICE_REJECTED"
	CodeInvoiceInProgress    ErrorCode = "INVOICE_IN_PROGRESS"
	CodeInvoiceStale         ErrorCode = "INVOICE_STALE"
	CodePointOfSaleTaken     ErrorCode = "POINT_OF_SALE_TAKEN"
	CodeQuoteAlreadyInvoiced ErrorCode = "QUOTE_ALREADY_INVOICED"
	CodeARCACredentials      ErrorCode = "ARCA_CREDENTIALS"
	CodeInvalidTaxID         ErrorCode = "INVALID_TAX_ID"
	CodeInvoicingUnavailable ErrorCode = "INVOICING_UNAVAILABLE"
	// The account's CUIT is what ARCA is connected with: it is required to connect and locked
	// while connected.
	CodeAccountTaxIDRequired ErrorCode = "ACCOUNT_TAX_ID_REQUIRED"
	CodeARCAConnected        ErrorCode = "ARCA_CONNECTED"
	// The connection cannot go while ARCA still owes the answer for an invoice issued with it.
	CodeInvoicePending ErrorCode = "INVOICE_PENDING"
	// The connection was made for the other ARCA environment, so it can neither take a certificate nor
	// reach an invoice made in this one.
	CodeARCAEnvironmentMismatch ErrorCode = "ARCA_ENVIRONMENT_MISMATCH"
	// Credit notes: only an issued invoice is credited, once; a pending credit note holds it.
	CodeInvoiceNotCreditable   ErrorCode = "INVOICE_NOT_CREDITABLE"
	CodeInvoiceAlreadyCredited ErrorCode = "INVOICE_ALREADY_CREDITED"
	CodeCreditNoteInProgress   ErrorCode = "CREDIT_NOTE_IN_PROGRESS"
	// The account switched invoicing with Coti off, so nothing new is issued; the server key is
	// INVOICING_DISABLED.
	CodeAccountInvoicingOff ErrorCode = "ACCOUNT_INVOICING_OFF"
	// A paid sale is reopened only after its paid mark is removed.
	CodeQuoteAlreadyPaid ErrorCode = "QUOTE_ALREADY_PAID"
	// Closing a sale (paid, no invoice needed) applies only to an accepted quote.
	CodeQuoteNotAccepted ErrorCode = "QUOTE_NOT_ACCEPTED"
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
	case errors.Is(err, ErrNotConfigured):
		return CodeNotConfigured
	case errors.Is(err, ErrDeliveryUnavailable):
		return CodeDeliveryUnavailable
	case errors.Is(err, ErrRepresentationUnavailable):
		return CodeRepresentationUnavailable
	case errors.Is(err, ErrInvoicingUnavailable):
		return CodeInvoicingUnavailable
	default:
		return CodeInternal
	}
}
