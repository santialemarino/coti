package domain

import (
	"errors"
	"testing"
)

func TestValidateHandlerIntent_FollowsTheStateIntentionMatrix(t *testing.T) {
	t.Parallel()
	intents := []HandlerIntent{HandlerIntentAtomic, HandlerIntentAddDiscount, HandlerIntentComplex}
	cases := []struct {
		status   QuoteStatus
		archived bool
		want     ErrorCode // empty when allowed.
	}{
		{QuoteStatusDraft, false, CodeQuoteNotSent},
		{QuoteStatusQuoted, false, CodeQuoteNotSent},
		{QuoteStatusSent, false, ""},
		{QuoteStatusChangeRequested, false, ""},
		{QuoteStatusAccepted, false, CodeQuoteNotSent},
		{QuoteStatusRejected, false, CodeQuoteNotSent},
		{QuoteStatusSent, true, CodeQuoteArchived},
		{QuoteStatusChangeRequested, true, CodeQuoteArchived},
	}
	for _, tc := range cases {
		for _, intent := range intents {
			err := ValidateHandlerIntent(tc.status, tc.archived, intent)
			if tc.want == "" {
				if err != nil {
					t.Errorf("%s archived=%v %s = %v, want allowed", tc.status, tc.archived, intent, err)
				}
				continue
			}
			if !errors.Is(err, ErrConflict) || CodeOf(err) != tc.want {
				t.Errorf("%s archived=%v %s = %v (%s), want %s", tc.status, tc.archived, intent, err,
					CodeOf(err), tc.want)
			}
		}
	}
}

func TestValidateHandlerIntent_RefusesAnUnknownIntent(t *testing.T) {
	t.Parallel()
	if err := ValidateHandlerIntent(QuoteStatusSent, false, "ACCEPT"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("unknown intent = %v, want ErrInvalidInput", err)
	}
}

func TestHandlerIntentOf_MapsOnlyMutations(t *testing.T) {
	t.Parallel()
	for _, operation := range []ChangeRequestOperationType{ChangeRequestOperationAddItem,
		ChangeRequestOperationRemoveItem, ChangeRequestOperationReplaceItem,
		ChangeRequestOperationUpdateQuantity} {
		if intent, ok := HandlerIntentOf(operation); !ok || intent != HandlerIntentAtomic {
			t.Errorf("HandlerIntentOf(%s) = %s, %v; want ATOMIC", operation, intent, ok)
		}
	}
	if _, ok := HandlerIntentOf(ChangeRequestOperationRequiresSeller); ok {
		t.Error("REQUIRES_SELLER has an intent, want none: it materializes nothing")
	}
}
