package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func followupInput() domain.FollowupDraftInput {
	return domain.FollowupDraftInput{SupplierName: "Corralón San Martín", SellerName: "Ana",
		ClientName: "Juan Pérez", QuoteNumber: 42,
		QuietSince: time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC), Lapsed: true,
		Items: []string{"10 bol. Cemento 50 kg", "2 m³ Arena fina"}}
}

func TestFollowupDrafter_Draft_SendsTheQuoteUnderAForcedSchema(t *testing.T) {
	generator := &fakeGenerator{answer: `{"message":"  Hola Juan, ¿pudiste ver la cotización #42?  "}`}

	drafter := NewFollowupDrafter(generator)
	drafter.now = func() time.Time { return time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC) }
	message, err := drafter.Draft(context.Background(), followupInput())
	if err != nil {
		t.Fatalf("Draft() = %v, want no error", err)
	}
	if message != "Hola Juan, ¿pudiste ver la cotización #42?" {
		t.Errorf("message = %q, want it trimmed", message)
	}
	if generator.operation != domain.AIOperationFollowupDraft {
		t.Errorf("operation = %q, want FOLLOWUP_DRAFT", generator.operation)
	}
	if generator.req.Schema["required"].([]string)[0] != "message" {
		t.Errorf("schema = %v, want a required message", generator.req.Schema)
	}
	input := generator.req.Input[0].Text
	for _, want := range []string{`"quote_number":42`, `"client_name":"Juan Pérez"`,
		`"offer_expired":true`, `"quote_sent_or_last_changed_on":"2026-10-28"`, `"days_without_answer":5`, "Arena fina"} {
		if !strings.Contains(input, want) {
			t.Errorf("input %s does not carry %s", input, want)
		}
	}
	if !strings.Contains(generator.req.Instructions, "Never state a price") {
		t.Error("instructions do not forbid amounts")
	}
}

// A blank answer is no draft at all, not an empty message for the seller to send.
func TestFollowupDrafter_Draft_RefusesABlankAnswer(t *testing.T) {
	generator := &fakeGenerator{answer: `{"message":"   "}`}

	_, err := NewFollowupDrafter(generator).Draft(context.Background(), followupInput())
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Draft() = %v, want ErrAIUnavailable", err)
	}
}

func TestFollowupDrafter_Draft_PassesAProviderFailureOn(t *testing.T) {
	boom := errors.New("provider down")
	generator := &fakeGenerator{err: boom}

	if _, err := NewFollowupDrafter(generator).Draft(context.Background(),
		followupInput()); !errors.Is(err, boom) {
		t.Fatalf("Draft() = %v, want %v", err, boom)
	}
}
