package services

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func newProposeOnlyHandler(proposer domain.ChangeRequestHandler, matcher catalogMatcher,
) *BatchChangeHandler {
	return &BatchChangeHandler{proposer: proposer, matcher: matcher,
		cfg: BatchChangeHandlerConfig{ContextMessages: 10, MaxOperations: 2},
		log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func preparedWithLine() (preparedBatch, uuid.UUID) {
	lineID := uuid.New()
	return preparedBatch{status: domain.QuoteStatusChangeRequested,
		items: []domain.QuoteItem{{ID: lineID, RequestedDescription: "cemento",
			Quantity: decimal.NewFromInt(2)}},
		messages: []domain.QuoteMessage{{Body: " sumá cal "}, {Body: "y sacá el cemento"}},
		context: []domain.QuoteMessage{{AuthorType: domain.MessageAuthorSeller,
			Body: "¿Te sirve así?"}}}, lineID
}

func TestBatchChangeHandler_Propose_SendsTheBatchAndMatchesNewMaterials(t *testing.T) {
	prepared, lineID := preparedWithLine()
	description, quantity := "cal hidratada", decimal.NewFromInt(5)
	proposer := &fakeChangeRequestHandler{proposal: &domain.ChangeRequestProposal{
		Level: domain.ChangeRequestLevelAtomic, Interpretation: "Suma cal y quita el cemento.",
		Operations: []domain.ChangeRequestOperation{
			{Type: domain.ChangeRequestOperationAddItem, RequestedDescription: &description,
				Quantity: &quantity},
			{Type: domain.ChangeRequestOperationRemoveItem, TargetItemID: &lineID},
		}}}
	calID := uuid.New()
	matcher := &fakeChangeRequestMatcher{matches: []domain.LineMatch{{ProductID: &calID,
		MatchStatus: domain.ItemMatchStatusMatched}}}

	proposal, err := newProposeOnlyHandler(proposer, matcher).propose(context.Background(), prepared)
	if err != nil {
		t.Fatalf("propose() = %v", err)
	}
	if len(proposer.inputs) != 1 || proposer.inputs[0].Message != "sumá cal\ny sacá el cemento" ||
		len(proposer.inputs[0].Context) != 1 || proposer.inputs[0].Items[0].ID != lineID {
		t.Errorf("model input = %+v, want the batch, its context and the draft's line", proposer.inputs)
	}
	if len(matcher.texts) != 1 || len(matcher.texts[0]) != 1 || matcher.texts[0][0] != description {
		t.Errorf("matched = %v, want only the added material", matcher.texts)
	}
	if proposal.record.Outcome != domain.HandlerOutcomeApplied || proposal.resolved == nil ||
		proposal.resolved.Operations[0].Match.ProductID == nil ||
		*proposal.resolved.Operations[0].Match.ProductID != calID {
		t.Errorf("proposal = %+v, want an applied proposal carrying the match", proposal)
	}
}

func TestBatchChangeHandler_Propose_LeavesWhatItCannotApplyToTheSeller(t *testing.T) {
	prepared, _ := preparedWithLine()
	stranger := uuid.New()
	quantity := decimal.NewFromInt(3)
	cases := []struct {
		name     string
		proposer domain.ChangeRequestHandler
		want     domain.HandlerNoMatchReason
	}{
		{"no model", nil, domain.HandlerNoMatchAIDisabled},
		{"escape", &fakeChangeRequestHandler{proposal: &domain.ChangeRequestProposal{
			Level: domain.ChangeRequestLevelNoMatch, Interpretation: "Acepta la cotización.",
			Operations: []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationRequiresSeller}},
		}}, domain.HandlerNoMatchRequiresSeller},
		{"a line of another draft", &fakeChangeRequestHandler{proposal: &domain.ChangeRequestProposal{
			Level: domain.ChangeRequestLevelAtomic, Interpretation: "Cambia una cantidad.",
			Operations: []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationUpdateQuantity,
				TargetItemID: &stranger, Quantity: &quantity}},
		}}, domain.HandlerNoMatchProposalRefused},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proposal, err := newProposeOnlyHandler(tc.proposer, nil).propose(context.Background(), prepared)
			if err != nil {
				t.Fatalf("propose() = %v, want a no-match rather than an error", err)
			}
			if proposal.resolved != nil || proposal.record.Outcome != domain.HandlerOutcomeNoMatch ||
				proposal.record.NoMatchReason == nil || *proposal.record.NoMatchReason != tc.want ||
				len(proposal.record.Operations) != 0 {
				t.Errorf("proposal = %+v, want a %s no-match with no operations", proposal.record, tc.want)
			}
		})
	}
}
