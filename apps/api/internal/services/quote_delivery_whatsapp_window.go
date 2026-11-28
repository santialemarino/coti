package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// whatsAppQuoteTemplateBody is the approved body of the quote template; Meta holds the same text,
// and the conversation records it with the variables filled.
const whatsAppQuoteTemplateBody = "Hola, {{1}} te envía la cotización de tu pedido.\n\n" +
	"Total: {{2}}\nVálida hasta el {{3}}\n\n" +
	"Podés verla y responderla desde este enlace:\n{{4}}\n\n" +
	"Si tenés dudas, respondé este mensaje."

// whatsAppFollowupTemplateBody is the approved body of the follow-up template.
const whatsAppFollowupTemplateBody = "Hola, te escribimos de {{1}} por la cotización que te enviamos.\n\n" +
	"{{2}}\n\n" +
	"Podés verla desde este enlace:\n{{3}}\n\n" +
	"Si tenés dudas, respondé este mensaje."

// whatsAppFollowupParameterMax keeps the seller's text inside Meta's 1,024-character template body.
const whatsAppFollowupParameterMax = 700

// errQuotePDFUnavailable refuses a quote template whose document header has no PDF to carry.
var errQuotePDFUnavailable = errors.New("quote PDF unavailable for the WhatsApp template header")

// quoteRecentSenderReader reports who wrote to a branch's channels within a window.
type quoteRecentSenderReader interface {
	ListRecentSenders(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID,
		channelIDs []uuid.UUID, senders []string, window time.Duration) (map[uuid.UUID]map[string]bool, error)
}

// whatsAppTarget is one WhatsApp destination on the channel a send goes out through.
type whatsAppTarget struct {
	ChannelID   uuid.UUID
	Destination string
}

// WithWhatsAppWindow checks WhatsApp's customer-service window before a quote or a follow-up goes
// out, and names the approved templates that replace free text outside it.
func (s *QuoteDeliveryService) WithWhatsAppWindow(reader quoteRecentSenderReader,
	templates domain.QuoteWhatsAppTemplates,
) *QuoteDeliveryService {
	s.recentSenders = reader
	s.templates = templates
	return s
}

// requireWhatsAppReach refuses a send before anything goes out when a WhatsApp destination sits
// outside the window and no template can reach it, so the seller can send by email instead.
func (s *QuoteDeliveryService) requireWhatsAppReach(ctx context.Context, q repository.Querier,
	tenant domain.Tenant, channels []domain.Channel, phones []string, followup bool,
) error {
	if s.whatsAppTemplate(followup) != "" {
		return nil
	}
	var targets []whatsAppTarget
	for _, channel := range channels {
		if channel.Type != domain.ChannelTypeWhatsApp {
			continue
		}
		for _, phone := range phones {
			targets = append(targets, whatsAppTarget{ChannelID: channel.ID, Destination: phone})
		}
	}
	closed, err := s.closedWhatsAppWindows(ctx, q, tenant, targets)
	if err != nil {
		return err
	}
	if len(closed) > 0 {
		return domain.WithCode(domain.CodeWhatsAppWindowClosed,
			fmt.Errorf("%w: no customer message within the WhatsApp window", domain.ErrInvalidInput))
	}
	return nil
}

// closedWhatsAppWindows returns the targets whose customer has not written to that channel within
// the window. Without a reader every window counts as open.
func (s *QuoteDeliveryService) closedWhatsAppWindows(ctx context.Context, q repository.Querier,
	tenant domain.Tenant, targets []whatsAppTarget,
) (map[whatsAppTarget]bool, error) {
	if s.recentSenders == nil || len(targets) == 0 {
		return nil, nil
	}
	forms := make(map[whatsAppTarget][]string, len(targets))
	var channelIDs []uuid.UUID
	var senders []string
	for _, target := range targets {
		destinations, err := domain.InboundSenderDestinations(domain.ChannelTypeWhatsApp, target.Destination)
		if err != nil {
			return nil, err
		}
		// Meta reports the sender as its bare wa_id; older rows may carry the plus.
		for _, destination := range destinations {
			forms[target] = append(forms[target], destination, strings.TrimPrefix(destination, "+"))
		}
		senders = append(senders, forms[target]...)
		channelIDs = append(channelIDs, target.ChannelID)
	}
	recent, err := s.recentSenders.ListRecentSenders(ctx, q, tenant.AccountID, tenant.BranchID,
		channelIDs, senders, whatsAppServiceWindow)
	if err != nil {
		return nil, err
	}
	closed := map[whatsAppTarget]bool{}
	for _, target := range targets {
		open := false
		for _, form := range forms[target] {
			open = open || recent[target.ChannelID][form]
		}
		if !open {
			closed[target] = true
		}
	}
	return closed, nil
}

// whatsAppTemplate is the configured template name for a quote or a follow-up.
func (s *QuoteDeliveryService) whatsAppTemplate(followup bool) string {
	if followup {
		return s.templates.Followup
	}
	return s.templates.Quote
}

// quoteTemplateMessage turns a quote message into the approved template: the PDF as the document
// header, then supplier, total, validity and link.
func (s *QuoteDeliveryService) quoteTemplateMessage(message quoteSendMessage,
	payload domain.QuoteRepresentationPayload, validUntil time.Time, pdf []domain.EmailAttachment,
) quoteSendMessage {
	parameters := []string{
		templateParameter(payload.Supplier.Name, 0),
		formatCommercialMoney(payload.Currency, payload.Total),
		validUntil.In(domain.Argentina).Format("2/1/2006"),
	}
	template := &domain.WhatsAppTemplate{Name: s.templates.Quote, Language: s.templates.Language,
		Parameters: append(parameters, message.Link)}
	if len(pdf) == 0 {
		message.Refusal = errQuotePDFUnavailable
	} else {
		template.Document = &domain.WhatsAppDocument{Filename: pdf[0].Filename,
			ContentType: pdf[0].ContentType, Content: pdf[0].Content}
	}
	message.Template = template
	message.Body = renderTemplateBody(whatsAppQuoteTemplateBody, append(parameters, domain.QuotePublicURLPlaceholder))
	return message
}

// followupTemplateMessage turns a follow-up into the approved template carrying the seller's text.
func (s *QuoteDeliveryService) followupTemplateMessage(message quoteSendMessage, supplier string,
	text string,
) quoteSendMessage {
	parameters := []string{templateParameter(supplier, 0),
		templateParameter(strings.ReplaceAll(text, domain.QuotePublicURLPlaceholder, message.Link),
			whatsAppFollowupParameterMax)}
	message.Template = &domain.WhatsAppTemplate{Name: s.templates.Followup,
		Language: s.templates.Language, Parameters: append(parameters, message.Link)}
	recorded := parameters[:1:1]
	recorded = append(recorded, templateParameter(text, whatsAppFollowupParameterMax),
		domain.QuotePublicURLPlaceholder)
	message.Body = renderTemplateBody(whatsAppFollowupTemplateBody, recorded)
	return message
}

// templateParameter flattens a value the way Meta requires (no newline, tab or run of spaces) and
// shortens it to max runes when max is positive.
func templateParameter(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if max > 0 && utf8.RuneCountInString(value) > max {
		value = strings.TrimSpace(string([]rune(value)[:max-1])) + "…"
	}
	return value
}

// renderTemplateBody fills {{1}}…{{n}} the way Meta renders the approved body.
func renderTemplateBody(body string, parameters []string) string {
	pairs := make([]string, 0, 2*len(parameters))
	for i, value := range parameters {
		pairs = append(pairs, fmt.Sprintf("{{%d}}", i+1), value)
	}
	return strings.NewReplacer(pairs...).Replace(body)
}
