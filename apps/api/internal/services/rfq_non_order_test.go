package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeNonOrderMessages struct {
	fakeInboundChannelMessages
	reserved  []domain.NewInboundChannelMessage
	detached  []domain.InboundMessageKind
	claimed   []uuid.UUID
	dismissed []uuid.UUID
	open      []domain.InboundChannelMessage
	body      string
}

func (f *fakeNonOrderMessages) Reserve(_ context.Context, _ repository.Querier, accountID, branchID,
	channelID uuid.UUID, in domain.NewInboundChannelMessage,
) (*domain.InboundChannelMessage, error) {
	f.reserved = append(f.reserved, in)
	return &domain.InboundChannelMessage{ID: in.ID, AccountID: accountID, BranchID: branchID,
		ChannelID: channelID, RFQID: in.RFQID, SenderID: in.SenderID, Body: in.Body}, nil
}

func (f *fakeNonOrderMessages) DetachNonOrder(_ context.Context, _ repository.Querier, accountID,
	branchID, _ uuid.UUID, kind domain.InboundMessageKind,
) (*domain.InboundChannelMessage, error) {
	f.detached = append(f.detached, kind)
	return &domain.InboundChannelMessage{ID: uuid.New(), AccountID: accountID, BranchID: branchID,
		ChannelID: testChannelID, Kind: &kind, SenderID: "5491100000000", Body: "Hola"}, nil
}

func (f *fakeNonOrderMessages) ListOpenNonOrders(_ context.Context, _ repository.Querier, _, _ uuid.UUID,
	_ int,
) ([]domain.InboundChannelMessage, error) {
	return f.open, nil
}

func (f *fakeNonOrderMessages) ClaimNonOrder(_ context.Context, _ repository.Querier, accountID,
	branchID, id, rfqID uuid.UUID,
) (*domain.InboundChannelMessage, error) {
	f.claimed = append(f.claimed, rfqID)
	return &domain.InboundChannelMessage{ID: id, AccountID: accountID, BranchID: branchID,
		ChannelID: testChannelID, RFQID: &rfqID, Body: f.body}, nil
}

func (f *fakeNonOrderMessages) Dismiss(_ context.Context, _ repository.Querier, _, _, id uuid.UUID) error {
	f.dismissed = append(f.dismissed, id)
	return nil
}

type fakeUnworkedRFQs struct {
	*fakeRFQs
	worked  bool
	deleted []uuid.UUID
}

func (f *fakeUnworkedRFQs) DeleteUnworked(_ context.Context, _ repository.Querier, _, _,
	rfqID uuid.UUID,
) (bool, error) {
	if f.worked {
		return false, nil
	}
	f.deleted = append(f.deleted, rfqID)
	return true, nil
}

func newNonOrderHarness(kind domain.RFQMessageKind) (*rfqHarness, *fakeNonOrderMessages, *fakeUnworkedRFQs) {
	h := newRFQHarness(nil)
	h.extractor.kind = kind
	messages := &fakeNonOrderMessages{}
	rfqs := &fakeUnworkedRFQs{fakeRFQs: h.rfqs}
	h.service.rfqs = rfqs
	h.service.WithInboundMessages(messages)
	return h, messages, rfqs
}

func whatsAppRoute() domain.InboundChannelRoute {
	return domain.InboundChannelRoute{AccountID: testAccountID, BranchID: testBranchID,
		ChannelID: testChannelID, ChannelType: domain.ChannelTypeWhatsApp}
}

func TestRFQService_CreateInboundTextDraftForMessage_FilesAGreetingInsteadOfFailingIt(t *testing.T) {
	h, messages, rfqs := newNonOrderHarness(domain.RFQMessageGreeting)
	rfqID := uuid.New()

	draft, err := h.service.CreateInboundTextDraftForMessage(context.Background(), whatsAppRoute(),
		domain.InboundTextRFQDraftInput{RawText: "Hola, necesito cotizar materiales para una obra."}, rfqID)
	if err != nil {
		t.Fatalf("CreateInboundTextDraftForMessage() = %v", err)
	}
	if draft.NonOrder == nil || *draft.NonOrder.Kind != domain.InboundMessageGreeting {
		t.Fatalf("draft = %+v, want a GREETING non-order", draft)
	}
	if len(rfqs.deleted) != 1 || rfqs.deleted[0] != rfqID {
		t.Errorf("deleted RFQs %v, want the reserved %s", rfqs.deleted, rfqID)
	}
	if len(h.rfqs.updatedStatus) != 0 || len(messages.detached) != 1 {
		t.Errorf("status updates %v / detached %v, want none / one", h.rfqs.updatedStatus, messages.detached)
	}
}

func TestRFQService_CreateInboundTextDraftForMessage_FilesAnOrderWithNoMaterialAsOther(t *testing.T) {
	h, messages, _ := newNonOrderHarness(domain.RFQMessageOrder)

	draft, err := h.service.CreateInboundTextDraftForMessage(context.Background(), whatsAppRoute(),
		domain.InboundTextRFQDraftInput{RawText: "lo de siempre"}, uuid.New())
	if err != nil || draft.NonOrder == nil {
		t.Fatalf("CreateInboundTextDraftForMessage() = %+v, %v, want a non-order", draft, err)
	}
	if messages.detached[0] != domain.InboundMessageOther {
		t.Errorf("kind = %s, want OTHER", messages.detached[0])
	}
}

func TestRFQService_CreateInboundTextDraftForMessage_FailsAnRFQSomethingWasBuiltOn(t *testing.T) {
	h, _, rfqs := newNonOrderHarness(domain.RFQMessageQuestion)
	rfqs.worked = true

	draft, err := h.service.CreateInboundTextDraftForMessage(context.Background(), whatsAppRoute(),
		domain.InboundTextRFQDraftInput{RawText: "¿tenés stock de cemento?"}, uuid.New())
	if err != nil {
		t.Fatalf("CreateInboundTextDraftForMessage() = %v", err)
	}
	if draft.NonOrder != nil || draft.RFQ.Status != domain.RFQStatusFailed {
		t.Errorf("draft = %+v, want the order failed as before", draft)
	}
}

func TestRFQService_CreateTextDraft_ASellerTypedOrderWithNoMaterialStillFails(t *testing.T) {
	h, messages, rfqs := newNonOrderHarness(domain.RFQMessageGreeting)

	draft, err := h.service.CreateTextDraft(context.Background(), rfqTenant(),
		domain.TextRFQDraftInput{ChannelID: testChannelID, RawText: "hola"})
	if err != nil {
		t.Fatalf("CreateTextDraft() = %v", err)
	}
	if draft.RFQ.Status != domain.RFQStatusFailed || len(messages.detached) != 0 || len(rfqs.deleted) != 0 {
		t.Errorf("status %s / detached %v / deleted %v, want FAILED and nothing filed",
			draft.RFQ.Status, messages.detached, rfqs.deleted)
	}
}

func TestRFQService_CreateWhatsAppMockDraft_RetainsTheMessageLikeADelivery(t *testing.T) {
	h, messages, _ := newNonOrderHarness(domain.RFQMessageQuestion)

	draft, err := h.service.CreateWhatsAppMockDraft(context.Background(), rfqTenant(),
		domain.WhatsAppMockRFQInput{From: "5491155550000", Text: "¿A qué hora cierran?"})
	if err != nil {
		t.Fatalf("CreateWhatsAppMockDraft() = %v", err)
	}
	if len(messages.reserved) != 1 || messages.reserved[0].RFQID == nil ||
		messages.reserved[0].SenderID != "5491155550000" {
		t.Fatalf("reserved %+v, want one message waiting on an RFQ", messages.reserved)
	}
	if draft.NonOrder == nil || *draft.NonOrder.Kind != domain.InboundMessageQuestion {
		t.Errorf("draft = %+v, want a QUESTION non-order", draft)
	}
}

func TestRFQService_CreateDraftFromNonOrder_OpensTheOrderTheSellerAskedFor(t *testing.T) {
	h, messages, rfqs := newNonOrderHarness(domain.RFQMessageGreeting)
	messages.body = "Hola, necesito cotizar materiales para una obra."
	h.rfqs.rfqRow = &domain.RFQ{ID: uuid.New(), AccountID: testAccountID, BranchID: testBranchID,
		ChannelID: testChannelID, Status: domain.RFQStatusReceived}
	tenant := rfqTenant()

	draft, err := h.service.CreateDraftFromNonOrder(context.Background(), tenant, uuid.New())
	if err != nil {
		t.Fatalf("CreateDraftFromNonOrder() = %v", err)
	}
	if len(messages.claimed) != 1 || len(h.rfqs.created) != 1 || *h.rfqs.created[0].ID != messages.claimed[0] {
		t.Fatalf("claimed %v / created %+v, want the message pointed at the order created with it",
			messages.claimed, h.rfqs.created)
	}
	if draft.RFQ.Status != domain.RFQStatusFailed || len(rfqs.deleted) != 0 {
		t.Errorf("status %s / deleted %v, want a failed order kept", draft.RFQ.Status, rfqs.deleted)
	}
}

func TestRFQService_NonOrders_NeedAnActiveBranch(t *testing.T) {
	h, _, _ := newNonOrderHarness(domain.RFQMessageGreeting)
	tenant := domain.Tenant{AccountID: testAccountID, UserID: testUserID}

	if _, err := h.service.ListNonOrders(context.Background(), tenant); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("ListNonOrders() = %v, want ErrInvalidInput", err)
	}
	if err := h.service.DismissNonOrder(context.Background(), tenant, uuid.New()); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("DismissNonOrder() = %v, want ErrInvalidInput", err)
	}
}

func TestRFQService_CreateDraftFromNonOrder_RefusesAMessageWithNoTextBeforeOpeningAnything(t *testing.T) {
	h, messages, _ := newNonOrderHarness(domain.RFQMessageGreeting)

	_, err := h.service.CreateDraftFromNonOrder(context.Background(), rfqTenant(), uuid.New())
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("CreateDraftFromNonOrder() = %v, want ErrInvalidInput", err)
	}
	if len(h.rfqs.created) != 0 || h.extractor.calls != 0 || len(messages.claimed) != 1 {
		t.Errorf("created %d RFQs / read %d times, want the claim refused in its transaction",
			len(h.rfqs.created), h.extractor.calls)
	}
}

func TestRFQService_CreateInboundTextDraftForMessage_AWebLinkRequestStaysAnOrder(t *testing.T) {
	h, messages, rfqs := newNonOrderHarness(domain.RFQMessageQuestion)
	route := domain.InboundChannelRoute{AccountID: testAccountID, BranchID: testBranchID,
		ChannelID: testChannelID, ChannelType: domain.ChannelTypeWebApp}

	draft, err := h.service.CreateInboundTextDraftForMessage(context.Background(), route,
		domain.InboundTextRFQDraftInput{RawText: "Hola, ¿hacen envíos a Pilar? Llamame."}, uuid.New())
	if err != nil {
		t.Fatalf("CreateInboundTextDraftForMessage() = %v", err)
	}
	if draft.NonOrder != nil || draft.RFQ.Status != domain.RFQStatusFailed ||
		len(messages.detached) != 0 || len(rfqs.deleted) != 0 {
		t.Errorf("draft = %+v, want the customer's request kept as a failed order", draft)
	}
}
