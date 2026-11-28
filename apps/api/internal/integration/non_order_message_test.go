//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// nonOrderInbox is a WhatsApp intake whose orders run the real pipeline over a staged reading.
func (e *env) nonOrderInbox(t *testing.T, name string, kind domain.RFQMessageKind) (*nameReplyInbox, *services.RFQService) {
	t.Helper()
	box := e.nameReplyInbox(t, name, true)
	pipeline := e.pipeline(t, stagedExtractor{kind: kind}, nil).
		WithInboundMessages(repository.NewInboundChannelMessageRepository())
	box.service = services.NewWhatsAppInboundService(e.db,
		services.NewInboundRoutingService(e.db, repository.NewChannelRepository()),
		repository.NewInboundChannelMessageRepository(), nil, pipeline, nil, nil).
		WithContacts(services.NewClientContactService(e.db, repository.NewClientRepository(),
			repository.NewClientContactRepository()).WithMessaging(box.messenger))
	return box, pipeline
}

func (e *env) countRFQs(t *testing.T, account uuid.UUID) int {
	t.Helper()
	var count int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM rfq WHERE account_id=$1`, account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestNonOrderMessage_AGreetingIsKeptForTheSellerAndTheNameStillLands(t *testing.T) {
	e := newEnv(t)
	box, pipeline := e.nonOrderInbox(t, "Non-order greeting", domain.RFQMessageGreeting)
	ctx := context.Background()
	tenant := domain.Tenant{AccountID: box.account, BranchID: box.branch, UserID: uuid.New()}
	sender := "+5491155550201"

	box.send(t, sender, "Hola, necesito cotizar materiales para una obra.")
	if n := e.countRFQs(t, box.account); n != 0 {
		t.Fatalf("a greeting opened %d orders, want none", n)
	}
	open, err := pipeline.ListNonOrders(ctx, tenant)
	if err != nil || len(open) != 1 || *open[0].Kind != domain.InboundMessageGreeting || open[0].SenderID != sender {
		t.Fatalf("open non-orders = %+v, %v; want the greeting", open, err)
	}
	if len(box.messenger.messages) != 1 || box.messenger.messages[0].Purpose != domain.ContactPurposeNameRequest {
		t.Fatalf("prompts = %+v, want the name question", box.messenger.messages)
	}

	// The name answers a greeting, so it joins no order and the seller has nothing to read in it.
	box.send(t, sender, "Juan Pérez")
	if e.contactName(t, box.account, sender) != "Juan Pérez" || e.promptStatus(t, box.account, sender) != "COMPLETED" {
		t.Fatalf("name = %q, prompt %s; want the reply taken", e.contactName(t, box.account, sender),
			e.promptStatus(t, box.account, sender))
	}
	if open, _ = pipeline.ListNonOrders(ctx, tenant); len(open) != 1 || e.countRFQs(t, box.account) != 0 {
		t.Fatalf("after the name: %d listed, %d orders; want the greeting alone and no order", len(open),
			e.countRFQs(t, box.account))
	}

	if err := pipeline.DismissNonOrder(ctx, tenant, open[0].ID); err != nil {
		t.Fatalf("DismissNonOrder() = %v", err)
	}
	if open, _ = pipeline.ListNonOrders(ctx, tenant); len(open) != 0 {
		t.Fatalf("a dismissed message is still listed: %+v", open)
	}
	if err := pipeline.DismissNonOrder(ctx, tenant, open0ID(t, e, box.account)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("dismissing twice = %v, want ErrNotFound", err)
	}
	other := domain.Tenant{AccountID: box.account, BranchID: e.seedBranch(t, box.account, "Otra"), UserID: tenant.UserID}
	if listed, err := pipeline.ListNonOrders(ctx, other); err != nil || len(listed) != 0 {
		t.Fatalf("another branch lists %d messages (%v), want none", len(listed), err)
	}
}

func TestNonOrderMessage_ASellerOpensTheOrderTheReadingMissed(t *testing.T) {
	e := newEnv(t)
	box, pipeline := e.nonOrderInbox(t, "Non-order to order", domain.RFQMessageQuestion)
	ctx := context.Background()
	seller := e.seedUser(t, box.account, domain.UserRoleSeller)
	tenant := domain.Tenant{AccountID: box.account, BranchID: box.branch, UserID: seller.ID}

	box.send(t, "+5491155550202", "¿tenés stock de cemento?")
	open, err := pipeline.ListNonOrders(ctx, tenant)
	if err != nil || len(open) != 1 {
		t.Fatalf("open non-orders = %d, %v; want one", len(open), err)
	}
	draft, err := pipeline.CreateDraftFromNonOrder(ctx, tenant, open[0].ID)
	if err != nil {
		t.Fatalf("CreateDraftFromNonOrder() = %v", err)
	}
	// Still no material in it, so the seller gets the failed order to load by hand.
	if draft.RFQ.Status != domain.RFQStatusFailed {
		t.Fatalf("order status = %s, want FAILED", draft.RFQ.Status)
	}
	var rfqID *uuid.UUID
	var kind *string
	if err := e.db.CrossAccount().QueryRow(ctx,
		`SELECT rfq_id, kind::text FROM inbound_channel_message WHERE id=$1`, open[0].ID).Scan(&rfqID, &kind); err != nil {
		t.Fatal(err)
	}
	if rfqID == nil || *rfqID != draft.RFQ.ID || kind != nil {
		t.Fatalf("message points at %v with kind %v, want the new order and no kind", rfqID, kind)
	}
	if _, err := pipeline.CreateDraftFromNonOrder(ctx, tenant, open[0].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("opening an order twice = %v, want ErrNotFound", err)
	}
}

func TestNonOrderMessage_AnRFQSomethingWasBuiltOnIsNeverRemoved(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Non-order guard")
	channel := e.seedWhatsAppInbox(t, account, branch).id
	ctx := context.Background()
	rfqID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO rfq(id,account_id,branch_id,channel_id,raw_text,status) VALUES($1,$2,$3,$4,'hola','RECEIVED')`,
		rfqID, account, branch, channel); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewRFQRepository()
	err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: account, BranchID: branch}, func(q repository.Querier) error {
		if _, err := repository.NewQuoteRepository().Create(ctx, q, account, domain.NewQuote{
			BranchID: branch, RFQID: rfqID, CurrentStatus: domain.QuoteStatusDraft,
		}); err != nil {
			return err
		}
		deleted, err := repo.DeleteUnworked(ctx, q, account, branch, rfqID)
		if err == nil && deleted {
			t.Error("an RFQ with a quote was removed")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.countRFQs(t, account) != 1 {
		t.Fatal("the worked RFQ is gone")
	}
}

func open0ID(t *testing.T, e *env, account uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM inbound_channel_message WHERE account_id=$1 AND kind='GREETING'`, account).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
