package domain

import "fmt"

// HandlerIntent is the column of the state×intention matrix a handler proposal falls in.
type HandlerIntent string

const (
	// HandlerIntentAtomic is a direct line change: add, remove, replace or requantify.
	HandlerIntentAtomic HandlerIntent = "ATOMIC"
	// HandlerIntentAddDiscount references an existing promotion; the engine computes its money.
	HandlerIntentAddDiscount HandlerIntent = "ADD_DISCOUNT"
	// HandlerIntentComplex is a goal the backend reaches through a fixed recipe.
	HandlerIntentComplex HandlerIntent = "COMPLEX"
)

// HandlerIntentOf returns the matrix column of an operation; false for REQUIRES_SELLER and any
// other type that materializes nothing.
func HandlerIntentOf(operation ChangeRequestOperationType) (HandlerIntent, bool) {
	switch operation {
	case ChangeRequestOperationAddItem, ChangeRequestOperationRemoveItem,
		ChangeRequestOperationReplaceItem, ChangeRequestOperationUpdateQuantity:
		return HandlerIntentAtomic, true
	default:
		return "", false
	}
}

// ValidateHandlerRun is the matrix's row check: the AI handler runs only on an unarchived quote
// that is SENT or CHANGE_REQUESTED. Before SENT the seller edits by hand; a closed or archived
// quote is blocked until it is reactivated or unarchived.
func ValidateHandlerRun(status QuoteStatus, archived bool) error {
	if archived {
		return WithCode(CodeQuoteArchived, fmt.Errorf("%w: the quote is archived", ErrConflict))
	}
	switch status {
	case QuoteStatusSent, QuoteStatusChangeRequested:
		return nil
	default:
		return WithCode(CodeQuoteNotSent,
			fmt.Errorf("%w: the AI handler does not act on a %s quote", ErrConflict, status))
	}
}

// ValidateHandlerIntent applies the state×intention matrix (docs/internal/domain/estados.md) to
// one proposal the AI handler would materialize. Every column of the matrix shares the row rule
// for the handler, so an unknown intent is the only further refusal.
func ValidateHandlerIntent(status QuoteStatus, archived bool, intent HandlerIntent) error {
	switch intent {
	case HandlerIntentAtomic, HandlerIntentAddDiscount, HandlerIntentComplex:
	default:
		return fmt.Errorf("%w: unknown handler intent %q", ErrInvalidInput, intent)
	}
	return ValidateHandlerRun(status, archived)
}
