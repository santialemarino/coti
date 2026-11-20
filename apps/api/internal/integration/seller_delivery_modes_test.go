//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

func TestSellerDelivery_ModesAndPresets(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	seed := e.seedSendableQuote(t, "Delivery modes", false)
	quoteID := seed.draft.Quote.ID
	service := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, &failingQuoteEvaluator{})
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(account_id,branch_id,type,identifier) VALUES($1,$2,'EMAIL','modes@test.local')`, seed.tenant.AccountID, seed.tenant.BranchID); err != nil {
		t.Fatal(err)
	}
	if preset, err := service.Preset(ctx, seed.tenant, quoteID, domain.QuoteDeliveryModeFirst, nil); err != nil || preset.Source != domain.QuoteDeliveryPresetNone {
		t.Fatalf("unsent preset=%+v, %v", preset, err)
	}
	phone := "+5491155550201"
	send := func(mode domain.QuoteDeliveryMode, phones, emails []string) (*domain.QuoteDeliveryResult, error) {
		return service.Send(ctx, seed.tenant, quoteID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Mode: mode, Phones: phones, Emails: emails})
	}
	first, err := send(domain.QuoteDeliveryModeFirst, []string{phone}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := send(domain.QuoteDeliveryModeFirst, []string{phone}, nil); domain.CodeOf(err) != domain.CodeQuoteNotSendable {
		t.Fatalf("a second first send=%v", err)
	}
	resendPreset, err := service.Preset(ctx, seed.tenant, quoteID, domain.QuoteDeliveryModeResend, nil)
	if err != nil || resendPreset.Source != domain.QuoteDeliveryPresetVersion || strings.Join(resendPreset.Phones, ",") != phone {
		t.Fatalf("resend preset=%+v, %v", resendPreset, err)
	}
	other, err := service.Preset(ctx, seed.tenant, quoteID, domain.QuoteDeliveryModeOtherMedium, nil)
	if err != nil || other.Source != domain.QuoteDeliveryPresetNone || strings.Join(other.UsedPhones, ",") != phone || len(other.UsedEmails) != 0 {
		t.Fatalf("other-medium preset=%+v, %v", other, err)
	}
	if _, err := send(domain.QuoteDeliveryModeOtherMedium, []string{"+5491155550202"}, nil); domain.CodeOf(err) != domain.CodeChannelAlreadyUsed {
		t.Fatalf("other medium through WhatsApp again=%v", err)
	}
	if _, err := send(domain.QuoteDeliveryModeOtherMedium, nil, []string{"buyer@example.test"}); err != nil {
		t.Fatalf("other medium through email=%v", err)
	}
	resent, err := send(domain.QuoteDeliveryModeResend, []string{phone}, nil)
	if err != nil || !resent.ExpiresAt.Equal(first.ExpiresAt.Truncate(time.Microsecond)) {
		t.Fatalf("resend=%+v, %v; first expiry %v", resent, err, first.ExpiresAt)
	}
	followup, err := service.Preset(ctx, seed.tenant, quoteID, domain.QuoteDeliveryModeFollowup, nil)
	if err != nil || strings.Join(followup.Phones, ",") != phone || strings.Join(followup.Emails, ",") != "buyer@example.test" {
		t.Fatalf("follow-up preset=%+v, %v", followup, err)
	}

	var clientID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT client_id FROM quote WHERE id=$1`, quoteID).Scan(&clientID); err != nil {
		t.Fatal(err)
	}
	sends := repository.NewQuoteSendRepository()
	if err := e.db.InTenantTx(ctx, seed.tenant, func(q repository.Querier) error {
		another, err := sends.ListClientLastQuoteSends(ctx, q, seed.tenant.AccountID, seed.tenant.BranchID, clientID, uuid.New())
		if err != nil {
			return err
		}
		reached := 0
		for _, s := range another {
			if s.ChannelType == domain.ChannelTypeWhatsApp && s.Destination == phone {
				reached++
			}
		}
		if reached != 2 {
			t.Fatalf("client last sends=%+v", another)
		}
		self, err := sends.ListClientLastQuoteSends(ctx, q, seed.tenant.AccountID, seed.tenant.BranchID, clientID, quoteID)
		if err != nil || len(self) != 0 {
			t.Fatalf("the quote itself counted as the client's last: %+v, %v", self, err)
		}
		elsewhere, err := sends.ListClientLastQuoteSends(ctx, q, seed.tenant.AccountID, uuid.New(), clientID, uuid.New())
		if err != nil || len(elsewhere) != 0 {
			t.Fatalf("another branch got this branch's destinations: %+v, %v", elsewhere, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
