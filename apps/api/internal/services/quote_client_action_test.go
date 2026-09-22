package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

/*
 * REQUEST_CHANGE and COMMENT are refused here, and that is a product decision rather than an
 * oversight: a change request is simultaneously a client action and an inbound conversation
 * message, joined by an explicit reference, and the entities answering for the second half are not
 * built. Accepting one would record the action and drop the message.
 *
 * This test exists so adding either to clientTransitions cannot be a one-line change that looks
 * harmless — it has to break a test whose comment says why.
 */
func TestRecordClientAction_RefusesAnAnswerTheCustomerSurfaceDoesNotCarry(t *testing.T) {
	t.Parallel()
	service := &QuoteDeliveryService{actions: stubClientActions{}}

	for _, action := range []domain.ClientActionType{domain.ClientActionRequestChange,
		domain.ClientActionComment, domain.ClientActionType("SOMETHING_ELSE")} {
		_, err := service.RecordClientAction(context.Background(), "any-token", action)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("RecordClientAction(%q) = %v, want ErrInvalidInput", action, err)
		}
	}
}

// The two answers the surface does carry must reach the persistence, not be refused with it.
func TestRecordClientAction_RefusesWhenTheActionRecorderIsUnbound(t *testing.T) {
	t.Parallel()
	service := &QuoteDeliveryService{}

	for _, action := range []domain.ClientActionType{domain.ClientActionAccept,
		domain.ClientActionReject} {
		_, err := service.RecordClientAction(context.Background(), "any-token", action)
		if !errors.Is(err, domain.ErrNotConfigured) {
			t.Errorf("RecordClientAction(%q) with no recorder = %v, want ErrNotConfigured",
				action, err)
		}
	}
}

func TestQuoteReference_PadsToTheProductsOwnWidth(t *testing.T) {
	t.Parallel()
	cases := map[int64]string{1: "COT-000001", 42: "COT-000042", 999999: "COT-999999",
		1000000: "COT-1000000"}
	for number, want := range cases {
		if got := domain.QuoteReference(number); got != want {
			t.Errorf("QuoteReference(%d) = %q, want %q", number, got, want)
		}
	}
}

type stubClientActions struct{}

func (stubClientActions) Create(context.Context, repository.Querier, uuid.UUID,
	domain.NewClientAction) (*domain.ClientAction, error) {
	return &domain.ClientAction{}, nil
}

func (stubClientActions) GetBySend(context.Context, repository.Querier, uuid.UUID,
	uuid.UUID) (*domain.ClientAction, error) {
	return nil, domain.ErrNotFound
}
