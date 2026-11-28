package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeRecentSenders struct {
	// recent is keyed by channel, then by the sender ID as stored.
	recent map[uuid.UUID]map[string]bool
	calls  int
	window time.Duration
}

func (f *fakeRecentSenders) ListRecentSenders(_ context.Context, _ repository.Querier, _, _ uuid.UUID,
	channelIDs []uuid.UUID, senders []string, window time.Duration,
) (map[uuid.UUID]map[string]bool, error) {
	f.calls++
	f.window = window
	found := map[uuid.UUID]map[string]bool{}
	for _, channelID := range channelIDs {
		for _, sender := range senders {
			if f.recent[channelID][sender] {
				if found[channelID] == nil {
					found[channelID] = map[string]bool{}
				}
				found[channelID][sender] = true
			}
		}
	}
	return found, nil
}

type recordingQuoteWhatsApp struct {
	sent []domain.QuoteWhatsAppMessage
}

func (r *recordingQuoteWhatsApp) SendQuote(_ context.Context, message domain.QuoteWhatsAppMessage,
) (*domain.DeliveryReceipt, error) {
	r.sent = append(r.sent, message)
	return &domain.DeliveryReceipt{ProviderReference: "wamid.1"}, nil
}

var windowTemplates = domain.QuoteWhatsAppTemplates{Quote: "coti_cotizacion", Followup: "coti_seguimiento",
	Language: "es_AR"}

// windowFixture has two WhatsApp sends on one channel: the first customer wrote within the window
// (stored as Meta's bare wa_id), the second never did; and one email.
func windowFixture(t *testing.T, templates domain.QuoteWhatsAppTemplates, pdfErr error) (*QuoteDeliveryService,
	*fakeRecentSenders, []domain.QuoteSend, *domain.QuoteRepresentationResult) {
	t.Helper()
	service, _, sends, bundle := deliveryMessagesFixture(t, nil, pdfErr)
	channelID := uuid.New()
	for i := range sends {
		sends[i].ChannelID = channelID
	}
	recent := &fakeRecentSenders{recent: map[uuid.UUID]map[string]bool{channelID: {"5491111111111": true}}}
	service.WithWhatsAppWindow(recent, templates)
	return service, recent, sends, bundle
}

// Outside the window the quote goes as the approved template: PDF header, supplier, total,
// validity and link; inside it the free text stays.
func TestQuoteDeliveryService_SendMessages_UsesTheQuoteTemplateOutsideTheWindow(t *testing.T) {
	t.Parallel()
	service, recent, sends, bundle := windowFixture(t, windowTemplates, nil)
	validUntil := time.Date(2026, 12, 10, 15, 0, 0, 0, time.UTC)

	messages, err := service.sendMessages(context.Background(), domain.Tenant{AccountID: uuid.New()},
		sends, bundle, nil, "https://c.test", validUntil)
	if err != nil {
		t.Fatalf("sendMessages() = %v", err)
	}
	if recent.window != 24*time.Hour {
		t.Errorf("window = %v, want WhatsApp's 24 hours", recent.window)
	}
	if open := messages[sends[0].ID]; open.Template != nil {
		t.Errorf("a customer inside the window got a template: %+v", open.Template)
	}
	closed := messages[sends[1].ID]
	template := closed.Template
	if template == nil || template.Name != "coti_cotizacion" || template.Language != "es_AR" {
		t.Fatalf("closed window template = %+v, want the quote template", template)
	}
	want := []string{"Corralón Centro", "ARS 1.234.567,50", "10/12/2026", closed.Link}
	if strings.Join(template.Parameters, "|") != strings.Join(want, "|") {
		t.Errorf("parameters = %q, want %q", template.Parameters, want)
	}
	if template.Document == nil || template.Document.Filename != "Cotizacion-COT-000042.pdf" ||
		string(template.Document.Content) != "%PDF" || closed.Refusal != nil {
		t.Errorf("document = %+v (refusal %v), want the quote PDF", template.Document, closed.Refusal)
	}
	if !strings.Contains(closed.Body, "Hola, Corralón Centro te envía la cotización de tu pedido.") ||
		strings.Count(closed.Body, domain.QuotePublicURLPlaceholder) != 1 || strings.Contains(closed.Body, "tok-new") {
		t.Errorf("recorded body is not the filled template with the link left out:\n%s", closed.Body)
	}
	if mail := messages[sends[2].ID]; mail.Template != nil || mail.Email == nil {
		t.Errorf("the email changed: %+v", mail)
	}
}

// A template whose document header has no PDF is refused before Meta is called.
func TestQuoteDeliveryService_SendMessages_RefusesTheQuoteTemplateWithoutItsPDF(t *testing.T) {
	t.Parallel()
	service, _, sends, bundle := windowFixture(t, windowTemplates, errors.New("storage down"))

	messages, err := service.sendMessages(context.Background(), domain.Tenant{AccountID: uuid.New()},
		sends, bundle, nil, "https://c.test", time.Now())
	if err != nil {
		t.Fatalf("sendMessages() = %v", err)
	}
	if closed := messages[sends[1].ID]; !errors.Is(closed.Refusal, errQuotePDFUnavailable) {
		t.Errorf("refusal = %v, want the missing PDF", closed.Refusal)
	}
	if open := messages[sends[0].ID]; open.Refusal != nil {
		t.Errorf("the free-text message was refused: %v", open.Refusal)
	}
}

// A follow-up outside the window carries the seller's text flattened into the follow-up template.
func TestQuoteDeliveryService_SendMessages_UsesTheFollowupTemplateOutsideTheWindow(t *testing.T) {
	t.Parallel()
	service, _, sends, bundle := windowFixture(t, windowTemplates, nil)
	followup := "¿Pudiste ver la cotización?\n\nQuedamos atentos.   Saludos"

	messages, err := service.sendMessages(context.Background(), domain.Tenant{AccountID: uuid.New()},
		sends, bundle, &followup, "https://c.test", time.Now())
	if err != nil {
		t.Fatalf("sendMessages() = %v", err)
	}
	if open := messages[sends[0].ID]; open.Template != nil || open.Body != followup {
		t.Errorf("inside the window the follow-up changed: %+v", open)
	}
	closed := messages[sends[1].ID]
	if closed.Template == nil || closed.Template.Name != "coti_seguimiento" || closed.Template.Document != nil {
		t.Fatalf("closed window template = %+v, want the follow-up template", closed.Template)
	}
	want := []string{"Corralón Centro", "¿Pudiste ver la cotización? Quedamos atentos. Saludos", sends[1].PublicURL}
	if strings.Join(closed.Template.Parameters, "|") != strings.Join(want, "|") {
		t.Errorf("parameters = %q, want %q", closed.Template.Parameters, want)
	}
	if strings.Contains(closed.Body, sends[1].PublicToken) ||
		!strings.Contains(closed.Body, "Hola, te escribimos de Corralón Centro") {
		t.Errorf("recorded follow-up = %q", closed.Body)
	}
}

// Without a configured template the send stops before anything is created or sent, for a quote
// and for a follow-up; an open window, a template or an email-only send pass.
func TestQuoteDeliveryService_RequireWhatsAppReach(t *testing.T) {
	t.Parallel()
	whatsApp := domain.Channel{ID: uuid.New(), Type: domain.ChannelTypeWhatsApp}
	email := domain.Channel{ID: uuid.New(), Type: domain.ChannelTypeEmail}
	for _, test := range []struct {
		name      string
		templates domain.QuoteWhatsAppTemplates
		channels  []domain.Channel
		phones    []string
		followup  bool
		refused   bool
		queried   bool
	}{
		{name: "quote outside the window", channels: []domain.Channel{whatsApp, email},
			phones: []string{"+5491122222222"}, refused: true, queried: true},
		{name: "follow-up outside the window", channels: []domain.Channel{whatsApp},
			phones: []string{"+5491122222222"}, followup: true, refused: true, queried: true},
		{name: "one of two numbers outside", channels: []domain.Channel{whatsApp},
			phones: []string{"+5491111111111", "+5491122222222"}, refused: true, queried: true},
		{name: "inside the window by the number without its 9", channels: []domain.Channel{whatsApp},
			phones: []string{"+541133333333"}, queried: true},
		{name: "quote template configured", templates: windowTemplates, channels: []domain.Channel{whatsApp},
			phones: []string{"+5491122222222"}},
		{name: "follow-up template only covers follow-ups", channels: []domain.Channel{whatsApp},
			templates: domain.QuoteWhatsAppTemplates{Followup: "coti_seguimiento"},
			phones:    []string{"+5491122222222"}, refused: true, queried: true},
		{name: "email only", channels: []domain.Channel{email}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			recent := &fakeRecentSenders{recent: map[uuid.UUID]map[string]bool{
				whatsApp.ID: {"5491111111111": true, "+5491133333333": true}}}
			service := (&QuoteDeliveryService{}).WithWhatsAppWindow(recent, test.templates)
			err := service.requireWhatsAppReach(context.Background(), nil, domain.Tenant{AccountID: uuid.New()},
				test.channels, test.phones, test.followup)
			if refused := domain.CodeOf(err) == domain.CodeWhatsAppWindowClosed; refused != test.refused ||
				(!test.refused && err != nil) {
				t.Errorf("requireWhatsAppReach() = %v, want refused=%v", err, test.refused)
			}
			if test.refused && !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("refusal %v is not invalid input", err)
			}
			if (recent.calls > 0) != test.queried {
				t.Errorf("queried the window %d times, want queried=%v", recent.calls, test.queried)
			}
		})
	}
}

// Dispatch hands the template to the provider and fails a refused send without calling it.
func TestQuoteDeliveryService_Dispatch_SendsTheTemplateAndHonoursARefusal(t *testing.T) {
	t.Parallel()
	sender := &recordingQuoteWhatsApp{}
	service := &QuoteDeliveryService{db: &fakePublicQuoteDB{}, whatsapp: sender, log: slog.Default()}
	pending := domain.SendTrackingStatusPending
	sends := []domain.QuoteSend{
		{ID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp, Destination: "+5491122222222",
			TrackingStatus: pending},
		{ID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp, Destination: "+5491144444444",
			TrackingStatus: pending},
	}
	template := &domain.WhatsAppTemplate{Name: "coti_cotizacion", Parameters: []string{"x"}}
	messages := map[uuid.UUID]quoteSendMessage{
		sends[0].ID: {Body: "recorded", Link: "https://c.test/q", Template: template},
		sends[1].ID: {Body: "recorded", Link: "https://c.test/q", Template: template, Refusal: errQuotePDFUnavailable},
	}

	outcomes := service.dispatch(context.Background(), domain.Tenant{AccountID: uuid.New()},
		domain.Quote{ID: uuid.New()}, sends, nil, messages)
	status := map[uuid.UUID]domain.SendTrackingStatus{}
	for _, outcome := range outcomes {
		status[outcome.ID] = outcome.Status
	}
	if status[sends[0].ID] != domain.SendTrackingStatusSent || status[sends[1].ID] != domain.SendTrackingStatusFailed {
		t.Errorf("outcomes = %v, want the template sent and the refused one failed", status)
	}
	if len(sender.sent) != 1 || sender.sent[0].Template != template || sender.sent[0].To != "+5491122222222" {
		t.Errorf("provider calls = %+v, want only the unrefused template", sender.sent)
	}
}

func TestTemplateParameter(t *testing.T) {
	t.Parallel()
	if got := templateParameter("  Hola\n\n\tqué tal    todo  ", 0); got != "Hola qué tal todo" {
		t.Errorf("flattened = %q", got)
	}
	if got := templateParameter("abcdefgh", 5); got != "abcd…" {
		t.Errorf("shortened = %q, want five runes ending in an ellipsis", got)
	}
}
