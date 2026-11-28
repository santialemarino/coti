//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// seedInboundWhatsApp records a customer message on a channel as Meta reports it: the bare wa_id.
func (e *env) seedInboundWhatsApp(t *testing.T, tenant domain.Tenant, channelID uuid.UUID, sender string,
	receivedAt time.Time) {
	t.Helper()
	id := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO inbound_channel_message
 (id, account_id, branch_id, channel_id, external_message_id, sender_id, body, payload, provider_received_at, kind)
 VALUES ($1,$2,$3,$4,$5,$6,'Hola','{}',$7,'GREETING')`, id, tenant.AccountID, tenant.BranchID, channelID,
		"wamid."+id.String(), sender, receivedAt); err != nil {
		t.Fatalf("seed inbound message: %v", err)
	}
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM inbound_channel_message WHERE id = $1`, id) })
}

func (e *env) windowDeliveryService(t *testing.T, whatsapp domain.QuoteWhatsAppSender,
	templates domain.QuoteWhatsAppTemplates) *services.QuoteDeliveryService {
	t.Helper()
	return e.quoteDeliveryService(t, whatsapp, &stagedQuoteEmailSender{}, nil).
		WithWhatsAppWindow(repository.NewInboundChannelMessageRepository(), templates)
}

func countQuoteSends(t *testing.T, e *env, quoteID uuid.UUID) int {
	t.Helper()
	var sends int
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM quote_send s
 JOIN quote_version v ON v.id = s.version_id WHERE v.quote_id = $1`, quoteID).Scan(&sends); err != nil {
		t.Fatal(err)
	}
	return sends
}

// Without a template, a WhatsApp send to a customer who has not written to that number within 24
// hours stops before any send exists; an old message, another number's message or another
// customer's message does not open the window.
func TestQuoteDelivery_RefusesWhatsAppOutsideTheWindowWithoutATemplate(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Window closed", false)
	now := time.Now()
	e.seedInboundWhatsApp(t, seed.tenant, seed.draft.RFQ.ChannelID, "5491155550101", now.Add(-25*time.Hour))
	e.seedInboundWhatsApp(t, seed.tenant, seed.draft.RFQ.ChannelID, "5491155550999", now.Add(-time.Hour))
	other := e.seedIntakeChannel(t, seed.tenant.AccountID, seed.tenant.BranchID)
	e.seedInboundWhatsApp(t, seed.tenant, other, "5491155550101", now.Add(-time.Hour))
	whatsapp := &captureWhatsAppSender{}
	service := e.windowDeliveryService(t, whatsapp, domain.QuoteWhatsAppTemplates{})

	_, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phones: []string{"+5491155550101"},
			Mode: domain.QuoteDeliveryModeFirst})
	if domain.CodeOf(err) != domain.CodeWhatsAppWindowClosed || !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Send() = %v, want WHATSAPP_WINDOW_CLOSED", err)
	}
	if whatsapp.count() != 0 || countQuoteSends(t, e, seed.draft.Quote.ID) != 0 {
		t.Errorf("calls/sends = %d/%d, want nothing created or sent", whatsapp.count(),
			countQuoteSends(t, e, seed.draft.Quote.ID))
	}
}

// Inside the window the quote and a later follow-up go as free text; once it closes, the follow-up
// is refused too.
func TestQuoteDelivery_SendsFreeTextInsideTheWindowAndChecksTheFollowup(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Window open", false)
	e.seedInboundWhatsApp(t, seed.tenant, seed.draft.RFQ.ChannelID, "5491155550101", time.Now().Add(-23*time.Hour))
	whatsapp := &captureWhatsAppSender{}
	service := e.windowDeliveryService(t, whatsapp, domain.QuoteWhatsAppTemplates{})

	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phones: []string{"+5491155550101"},
			Mode: domain.QuoteDeliveryModeFirst}); err != nil {
		t.Fatalf("Send() = %v", err)
	}
	if whatsapp.count() != 1 || whatsapp.messages[0].Template != nil {
		t.Fatalf("messages = %+v, want one free-text message", whatsapp.messages)
	}
	followup := "¿Pudiste revisarla?"
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phones: []string{"+5491155550101"},
			FollowupMessage: &followup}); err != nil {
		t.Fatalf("follow-up inside the window = %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `UPDATE inbound_channel_message
 SET provider_received_at = now() - interval '25 hours' WHERE channel_id = $1`, seed.draft.RFQ.ChannelID); err != nil {
		t.Fatal(err)
	}
	_, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phones: []string{"+5491155550101"},
			FollowupMessage: &followup})
	if domain.CodeOf(err) != domain.CodeWhatsAppWindowClosed || whatsapp.count() != 2 {
		t.Errorf("follow-up outside the window = %v after %d calls, want WHATSAPP_WINDOW_CLOSED and no call",
			err, whatsapp.count())
	}
}

// With the quote template configured, a closed window sends the template with the quote's PDF.
func TestQuoteDelivery_SendsTheQuoteTemplateOutsideTheWindow(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Window template", false)
	whatsapp := &captureWhatsAppSender{}
	service := e.windowDeliveryService(t, whatsapp, domain.QuoteWhatsAppTemplates{Quote: "coti_cotizacion",
		Language: "es_AR"})

	result, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phones: []string{"+5491155550101"},
			Mode: domain.QuoteDeliveryModeFirst})
	if err != nil || result.CurrentStatus != domain.QuoteStatusSent {
		t.Fatalf("Send() = %+v, %v; want the quote sent", result, err)
	}
	if whatsapp.count() != 1 {
		t.Fatalf("calls = %d, want 1", whatsapp.count())
	}
	template := whatsapp.messages[0].Template
	if template == nil || template.Name != "coti_cotizacion" || len(template.Parameters) != 4 ||
		template.Document == nil || !bytes.HasPrefix(template.Document.Content, []byte("%PDF")) {
		t.Fatalf("template = %+v, want the quote template with its PDF", template)
	}
}
