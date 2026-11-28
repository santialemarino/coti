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
	return preparedBatch{stateAtClaim: domain.QuoteStatusChangeRequested,
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

func TestBatchChangeHandler_Propose_ActsOnTheDecisionAndLetsAChangeWin(t *testing.T) {
	prepared, lineID := preparedWithLine()
	quantity := decimal.NewFromInt(4)
	requantified := []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationUpdateQuantity,
		TargetItemID: &lineID, Quantity: &quantity}}
	escape := []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationRequiresSeller}}
	cases := []struct {
		name        string
		level       domain.ChangeRequestLevel
		decision    domain.CustomerDecision
		conditional bool
		operations  []domain.ChangeRequestOperation
		wantChange  bool
		wantReason  domain.HandlerNoMatchReason
		wantActedOn domain.CustomerDecision
	}{
		{"acceptance", domain.ChangeRequestLevelNoMatch, domain.CustomerDecisionAccept, false, escape,
			false, domain.HandlerNoMatchCustomerAccepts, domain.CustomerDecisionAccept},
		{"rejection", domain.ChangeRequestLevelNoMatch, domain.CustomerDecisionReject, false, escape,
			false, domain.HandlerNoMatchCustomerRejects, domain.CustomerDecisionReject},
		{"question", domain.ChangeRequestLevelNoMatch, domain.CustomerDecisionQuestion, false, escape,
			false, domain.HandlerNoMatchCustomerQuestion, domain.CustomerDecisionQuestion},
		{"nothing", domain.ChangeRequestLevelNoMatch, domain.CustomerDecisionNone, false, escape,
			false, domain.HandlerNoMatchNoRequest, domain.CustomerDecisionNone},
		{"new order", domain.ChangeRequestLevelNoMatch, domain.CustomerDecisionNewOrder, false, escape,
			false, domain.HandlerNoMatchNewOrder, domain.CustomerDecisionNewOrder},
		{"conditional acceptance", domain.ChangeRequestLevelNoMatch, domain.CustomerDecisionAccept,
			true, escape, true, domain.HandlerNoMatchRequiresSeller, domain.CustomerDecisionChange},
		{"unmapped change", domain.ChangeRequestLevelNoMatch, domain.CustomerDecisionChange, false,
			escape, true, domain.HandlerNoMatchRequiresSeller, domain.CustomerDecisionChange},
		{"acceptance with a change", domain.ChangeRequestLevelAtomic, domain.CustomerDecisionAccept,
			false, requantified, true, "", domain.CustomerDecisionChange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proposer := &fakeChangeRequestHandler{proposal: &domain.ChangeRequestProposal{
				Level: tc.level, Interpretation: "Responde a la cotización.", CustomerDecision: tc.decision,
				DecisionConfidencePercent: 120, Conditional: tc.conditional, Operations: tc.operations}}
			proposal, err := newProposeOnlyHandler(proposer, nil).propose(context.Background(), prepared)
			if err != nil {
				t.Fatalf("propose() = %v", err)
			}
			if proposal.change != tc.wantChange {
				t.Errorf("change = %v, want %v", proposal.change, tc.wantChange)
			}
			record := proposal.record
			if tc.wantReason == "" {
				if record.Outcome != domain.HandlerOutcomeApplied || proposal.resolved == nil {
					t.Errorf("record = %+v, want the change applied", record)
				}
			} else if record.NoMatchReason == nil || *record.NoMatchReason != tc.wantReason {
				t.Errorf("reason = %v, want %s", record.NoMatchReason, tc.wantReason)
			}
			if record.CustomerDecision == nil || *record.CustomerDecision != tc.wantActedOn ||
				record.DecisionConfidencePercent == nil || *record.DecisionConfidencePercent != 100 {
				t.Errorf("decision = %v at %v, want %s clamped to 100", record.CustomerDecision,
					record.DecisionConfidencePercent, tc.wantActedOn)
			}
		})
	}
}

func TestRemapChangeRequest_PointsEveryNamedLineAtItsClone(t *testing.T) {
	sent, cloned, other := uuid.New(), uuid.New(), uuid.New()
	quantity := decimal.NewFromInt(4)
	operation := domain.ChangeRequestOperation{Type: domain.ChangeRequestOperationUpdateQuantity,
		TargetItemID: &sent, Quantity: &quantity}
	resolved := resolvedChangeRequest{
		Proposal: domain.ChangeRequestProposal{IntentItemIDs: []uuid.UUID{sent},
			Operations: []domain.ChangeRequestOperation{operation}},
		Operations: []resolvedChangeOperation{{Operation: operation}}}

	remapped, err := remapChangeRequest(resolved, map[uuid.UUID]uuid.UUID{sent: cloned})
	if err != nil {
		t.Fatalf("remapChangeRequest() = %v", err)
	}
	if remapped.Proposal.IntentItemIDs[0] != cloned || *remapped.Proposal.Operations[0].TargetItemID != cloned ||
		*remapped.Operations[0].Operation.TargetItemID != cloned {
		t.Errorf("remapped = %+v, want every reference on the clone's line", remapped)
	}
	if *resolved.Operations[0].Operation.TargetItemID != sent {
		t.Error("the original proposal changed, want it kept for a retried run")
	}
	if _, err := remapChangeRequest(resolved, map[uuid.UUID]uuid.UUID{other: cloned}); err != errDraftMoved {
		t.Errorf("remap with the line missing = %v, want errDraftMoved", err)
	}
}

type recordingNewOrders struct {
	routes []domain.InboundChannelRoute
	inputs []domain.InboundTextRFQDraftInput
	ids    []uuid.UUID
}

func (r *recordingNewOrders) CreateInboundTextDraftForMessage(_ context.Context,
	route domain.InboundChannelRoute, in domain.InboundTextRFQDraftInput, rfqID uuid.UUID,
) (*domain.TextRFQDraft, error) {
	r.routes, r.inputs, r.ids = append(r.routes, route), append(r.inputs, in), append(r.ids, rfqID)
	return &domain.TextRFQDraft{}, nil
}

func TestBatchChangeHandler_TakeNewOrder_SendsOnlyAWhatsAppNewOrderToIntake(t *testing.T) {
	channelID, clientID, actionID := uuid.New(), uuid.New(), uuid.New()
	tenant := domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()}
	batch := domain.MessageBatch{ID: uuid.New()}
	decided := func(decision domain.CustomerDecision) domain.HandlerProposalRecord {
		return domain.HandlerProposalRecord{CustomerDecision: &decision}
	}
	whatsApp := []domain.QuoteMessage{{ChannelID: &channelID, Body: " necesito cotizar "},
		{ChannelID: &channelID, Body: "5 bolsas"}}
	cases := []struct {
		name     string
		decision domain.CustomerDecision
		messages []domain.QuoteMessage
		want     bool
	}{
		{"new order", domain.CustomerDecisionNewOrder, whatsApp, true},
		{"question", domain.CustomerDecisionQuestion, whatsApp, false},
		{"portal request", domain.CustomerDecisionNewOrder,
			[]domain.QuoteMessage{{ChannelID: &channelID, ClientActionID: &actionID, Body: "otro"}}, false},
		{"no channel", domain.CustomerDecisionNewOrder, []domain.QuoteMessage{{Body: "otro"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			intake := &recordingNewOrders{}
			handler := newProposeOnlyHandler(nil, nil).WithNewOrders(intake)
			record := decided(tc.decision)
			prepared := preparedBatch{tenant: tenant, clientID: &clientID, messages: tc.messages}
			if err := handler.takeNewOrder(context.Background(), batch, prepared, &record); err != nil {
				t.Fatalf("takeNewOrder() = %v", err)
			}
			if !tc.want {
				if len(intake.ids) != 0 || record.NewOrderRFQID != nil {
					t.Errorf("intake = %v, record = %v, want nothing taken", intake.ids, record.NewOrderRFQID)
				}
				return
			}
			if len(intake.ids) != 1 || record.NewOrderRFQID == nil || *record.NewOrderRFQID != intake.ids[0] {
				t.Fatalf("intake = %v, record = %v, want one order named on the record", intake.ids,
					record.NewOrderRFQID)
			}
			route, in := intake.routes[0], intake.inputs[0]
			if route.AccountID != tenant.AccountID || route.BranchID != tenant.BranchID ||
				route.ChannelID != channelID || route.ChannelType != domain.ChannelTypeWhatsApp ||
				in.ClientID == nil || *in.ClientID != clientID || in.RawText != "necesito cotizar\n5 bolsas" {
				t.Errorf("intake got %+v and %+v, want the batch's tenant, channel, client and text", route, in)
			}
			again := decided(tc.decision)
			_ = handler.takeNewOrder(context.Background(), batch, prepared, &again)
			if intake.ids[1] != intake.ids[0] {
				t.Errorf("retry order id = %s, want the first one (%s)", intake.ids[1], intake.ids[0])
			}
		})
	}
}
