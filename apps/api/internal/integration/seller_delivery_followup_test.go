//go:build integration

package integration

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/shopspring/decimal"
	"strings"
	"testing"
	"time"
)

func TestSellerDelivery_MultipleRecipientsHistoryAndFollowup(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	seed := e.seedSendableQuote(t, "Multiple delivery", false)
	whatsapp := &captureWhatsAppSender{}
	email := &stagedQuoteEmailSender{}
	evaluator := &failingQuoteEvaluator{}
	service := e.quoteDeliveryService(t, whatsapp, email, evaluator)
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(account_id,branch_id,type,identifier) VALUES($1,$2,'EMAIL','multisend@test.local')`, seed.tenant.AccountID, seed.tenant.BranchID); err != nil {
		t.Fatal(err)
	}
	input := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phones: []string{"+5491155550101", "+5491155550102", "+5491155550101"}, Emails: []string{"one@example.test", "two@example.test"}}
	sent, err := service.Send(ctx, seed.tenant, seed.draft.Quote.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(sent.Deliveries) != 5 || whatsapp.count() != 2 || email.count() != 2 {
		t.Fatalf("deliveries=%d, WhatsApp=%d, email=%d", len(sent.Deliveries), whatsapp.count(), email.count())
	}
	if replay, err := service.Send(ctx, seed.tenant, seed.draft.Quote.ID, input); err != nil || !replay.Replay || whatsapp.count() != 2 || email.count() != 2 {
		t.Fatalf("replay=%+v, error=%v", replay, err)
	}
	changed := input
	changed.Phones = []string{"+5491155550103"}
	if _, err := service.Send(ctx, seed.tenant, seed.draft.Quote.ID, changed); domain.CodeOf(err) != domain.CodeIdempotencyMismatch {
		t.Fatalf("changed recipients=%v", err)
	}
	repeated := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phones: []string{"+5491155550103"}, OtherChannelsOnly: true}
	if _, err := service.Send(ctx, seed.tenant, seed.draft.Quote.ID, repeated); domain.CodeOf(err) != domain.CodeChannelAlreadyUsed {
		t.Fatalf("reusing a channel=%v", err)
	}
	originalExpiry := *sent.ExpiresAt
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote SET updated_at=now()-interval '6 days' WHERE account_id=$1 AND id=$2`, seed.tenant.AccountID, seed.draft.Quote.ID); err != nil {
		t.Fatal(err)
	}
	jobs, name := e.followupJobs(t)
	if _, err := jobs.Run(ctx, name); err != nil {
		t.Fatal(err)
	}
	reminder := "¿Pudiste revisar la cotización?"
	followup := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phones: input.Phones, FollowupMessage: &reminder}
	beforeEvaluation := evaluator.count()
	result, err := service.Send(ctx, seed.tenant, seed.draft.Quote.ID, followup)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deliveries) != 2 || !result.ExpiresAt.Equal(originalExpiry.Truncate(time.Microsecond)) || evaluator.count() != beforeEvaluation {
		t.Fatalf("follow-up changed publication: %+v", result)
	}
	for _, send := range result.Deliveries {
		if send.Purpose != "FOLLOWUP" || send.Message == nil || *send.Message != reminder {
			t.Fatalf("follow-up tracking=%+v", send)
		}
	}
	if state := e.followupOf(t, seed.draft.Quote.ID); state.flagged {
		t.Fatal("successful reminder did not clear follow-up flag")
	}
	if whatsapp.messages[len(whatsapp.messages)-1].Body != reminder {
		t.Fatal("provider did not receive the seller's message")
	}
	replay, err := service.Send(ctx, seed.tenant, seed.draft.Quote.ID, followup)
	if err != nil || !replay.Replay || whatsapp.count() != 4 {
		t.Fatalf("reminder replay=%+v, %v; provider calls=%d", replay, err, whatsapp.count())
	}
	other := seed.tenant
	other.BranchID = e.seedBranch(t, seed.tenant.AccountID, "Other branch")
	if _, err := service.Send(ctx, other, seed.draft.Quote.ID, followup); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-branch reminder=%v", err)
	}
}

func TestSellerFollowup_UsesConfiguredTemplate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	seed := e.seedSendableQuote(t, "Configured reminder", false)
	service := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil)
	if _, err := service.Send(ctx, seed.tenant, seed.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550101"}); err != nil {
		t.Fatal(err)
	}
	renderer := e.followupService()
	initial, err := renderer.Template(ctx, seed.tenant, seed.draft.Quote.ID)
	if err != nil || initial.TemplateConfigured {
		t.Fatalf("default=%+v,%v", initial, err)
	}
	template := "Seguimiento: {cotizacion}."
	err = e.db.InTenantTx(ctx, seed.tenant, func(q repository.Querier) error {
		repo := repository.NewAccountRepository()
		account, err := repo.GetByID(ctx, q, seed.tenant.AccountID)
		if err != nil {
			return err
		}
		_, err = repo.Update(ctx, q, account.ID, domain.AccountUpdate{Name: account.Name, FollowupTemplate: &template})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	configured, err := renderer.Template(ctx, seed.tenant, seed.draft.Quote.ID)
	if err != nil || !configured.TemplateConfigured || configured.Message == initial.Message {
		t.Fatalf("configured=%+v,%v", configured, err)
	}
}

func TestSellerArchive_FailedSourceWithoutQuote(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	account, branch := e.seedAccount(t, "Failed source archive")
	channel := e.seedIntakeChannel(t, account, branch)
	user := e.seedUser(t, account, domain.UserRoleAdmin)
	tenant := domain.Tenant{AccountID: account, BranchID: branch, UserID: user.ID, Role: domain.UserRoleAdmin}
	repo := repository.NewRFQRepository()
	var source *domain.RFQ
	if err := e.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		source, err = repo.Create(ctx, q, account, domain.NewRFQ{BranchID: branch, ChannelID: channel, Status: domain.RFQStatusFailed})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.dropDraft(t, source.ID)
	service := e.pipeline(t, stagedExtractor{}, map[string]int{})
	if err := service.SetFailedArchived(ctx, tenant, source.ID, true); err != nil {
		t.Fatal(err)
	}
	visible, err := service.List(ctx, tenant, false)
	if err != nil || len(visible) != 0 {
		t.Fatalf("open queue=%+v,%v", visible, err)
	}
	all, err := service.List(ctx, tenant, true)
	if err != nil || len(all) != 1 || all[0].ArchivedAt == nil {
		t.Fatalf("archive=%+v,%v", all, err)
	}
	other := tenant
	other.BranchID = e.seedBranch(t, account, "Not accessible here")
	if err := service.SetFailedArchived(ctx, other, source.ID, false); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-branch restore=%v", err)
	}
	if err := service.SetFailedArchived(ctx, tenant, source.ID, false); err != nil {
		t.Fatal(err)
	}
	restored, err := service.List(ctx, tenant, false)
	if err != nil || len(restored) != 1 || restored[0].ArchivedAt != nil {
		t.Fatalf("restored=%+v,%v", restored, err)
	}
}

func TestSellerSource_ManualOrderRetainsOriginalLines(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	account, branch := e.seedAccount(t, "Manual source")
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(account_id,branch_id,type) VALUES($1,$2,'MANUAL_ENTRY')`, account, branch); err != nil {
		t.Fatal(err)
	}
	user := e.seedUser(t, account, domain.UserRoleAdmin)
	tenant := domain.Tenant{AccountID: account, BranchID: branch, UserID: user.ID, Role: domain.UserRoleAdmin}
	service := e.pipeline(t, stagedExtractor{}, map[string]int{})
	creation, err := service.CreateManual(ctx, tenant, domain.NewRfq{Items: []domain.NewRfqItem{{RequestedDescription: "Original material", Quantity: decimal.NewFromInt(2)}}})
	if err != nil {
		t.Fatal(err)
	}
	e.dropDraft(t, creation.Rfq.ID)
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE quote_item SET requested_description='Seller replacement',quantity=5 WHERE account_id=$1 AND version_id=$2`, account, creation.Version.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetDetail(ctx, tenant, creation.Rfq.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.OriginalText == nil || !strings.Contains(*detail.OriginalText, "2 — Original material") || strings.Contains(*detail.OriginalText, "replacement") {
		t.Fatalf("original text=%v", detail.OriginalText)
	}
}
