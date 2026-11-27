package services

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// quoteMessageMaxItems is how many lines a quote message lists before «y N más».
const quoteMessageMaxItems = 8

// quoteMessageStage is what a quote message offers its destination contact (decision 92).
type quoteMessageStage int

const (
	// quoteMessageOrderLink goes to a contact proved in a portal account before the send.
	quoteMessageOrderLink quoteMessageStage = iota
	// quoteMessageInvitation goes to anyone else: the summary plus an invitation to sign up.
	quoteMessageInvitation
)

// quoteMessageContent is the deterministic summary every channel renders: no AI writes it.
type quoteMessageContent struct {
	Greeting string
	Intro    string
	Items    []string
	More     string
	Total    string
	Validity string
}

func newQuoteMessageContent(payload domain.QuoteRepresentationPayload, validUntil *time.Time) quoteMessageContent {
	content := quoteMessageContent{Greeting: "Hola.",
		Intro: payload.Supplier.Name + " te envía la cotización de tu pedido:",
		Total: "Total: " + formatCommercialMoney(payload.Currency, payload.Total),
	}
	if payload.Customer.Name != nil && strings.TrimSpace(*payload.Customer.Name) != "" {
		content.Greeting = "Hola, " + strings.TrimSpace(*payload.Customer.Name) + "."
	}
	for i, item := range payload.Items {
		if i == quoteMessageMaxItems {
			content.More = fmt.Sprintf("y %d más", len(payload.Items)-quoteMessageMaxItems)
			break
		}
		quantity := formatQuantity(item.Quantity)
		if item.Unit != nil && strings.TrimSpace(*item.Unit) != "" {
			quantity += " " + strings.TrimSpace(*item.Unit)
		}
		content.Items = append(content.Items, quantity+" · "+item.ProductName+" — "+
			formatCommercialMoney(payload.Currency, item.Subtotal))
	}
	content.Validity = payload.ValidityNote
	if validUntil != nil {
		content.Validity = "Válida hasta el " + validUntil.In(domain.Argentina).Format("2/1/2006") + "."
	}
	return content
}

// buildQuoteWhatsAppMessage renders the WhatsApp text with the link left as the placeholder, so the
// conversation stores it without the token.
func buildQuoteWhatsAppMessage(payload domain.QuoteRepresentationPayload, validUntil *time.Time,
	stage quoteMessageStage,
) string {
	content := newQuoteMessageContent(payload, validUntil)
	var b strings.Builder
	b.WriteString(content.Greeting + "\n\n" + content.Intro + "\n")
	for _, item := range content.Items {
		b.WriteString("• " + item + "\n")
	}
	if content.More != "" {
		b.WriteString(content.More + "\n")
	}
	b.WriteString("\n" + content.Total + "\n" + content.Validity + "\n\n")
	if stage == quoteMessageInvitation {
		b.WriteString("Para aceptarla y seguir tus pedidos online, creá tu cuenta en " +
			payload.Supplier.Name + ":\n" + domain.QuotePublicURLPlaceholder +
			"\n\nTambién podés responder este mensaje para confirmarla o pedir cambios.")
		return b.String()
	}
	b.WriteString("Mirá el detalle, elegí alternativas y aceptala acá:\n" + domain.QuotePublicURLPlaceholder)
	return b.String()
}

// quoteEmailContent is the copy of a quote email; the layout and the brand come from renderEmail.
type quoteEmailContent struct {
	Subject     string
	Heading     string
	Paragraphs  []string
	Items       []string
	ActionLabel string
}

func buildQuoteEmail(payload domain.QuoteRepresentationPayload, validUntil *time.Time,
	stage quoteMessageStage, withPDF bool,
) quoteEmailContent {
	content := newQuoteMessageContent(payload, validUntil)
	email := quoteEmailContent{
		Subject:     "Cotización " + payload.Reference + " de " + payload.Supplier.Name,
		Heading:     "Tu cotización está lista",
		Paragraphs:  []string{content.Greeting + " " + content.Intro},
		Items:       content.Items,
		ActionLabel: "Ver mi pedido",
	}
	if content.More != "" {
		email.Items = append(email.Items, content.More)
	}
	summary := content.Total + ". " + content.Validity
	if withPDF {
		summary += " Te adjuntamos el PDF con el detalle completo."
	}
	if stage == quoteMessageInvitation {
		email.Paragraphs = append(email.Paragraphs, summary,
			"Para aceptarla y seguir tus pedidos online, creá tu cuenta en "+payload.Supplier.Name+
				". También podés responder este correo para confirmarla o pedir cambios.")
		email.ActionLabel = "Crear mi cuenta"
		return email
	}
	email.Paragraphs = append(email.Paragraphs, summary)
	return email
}

// quoteMessageLink is where a destination is sent: its order, or sign-up that returns to it. The
// token is the delivery's own; the URL names no contact.
func quoteMessageLink(base, token string, stage quoteMessageStage) string {
	if stage == quoteMessageInvitation {
		return customerSignupURL(base) + "?next=" + url.QueryEscape("/quotes/"+url.PathEscape(token))
	}
	return customerQuoteURL(base, token)
}

// formatQuantity renders a NUMERIC(14,2) quantity the Argentine way, without trailing zeros.
func formatQuantity(raw string) string {
	integer, fraction, found := strings.Cut(raw, ".")
	fraction = strings.TrimRight(fraction, "0")
	if !found || fraction == "" {
		return integer
	}
	return integer + "," + fraction
}
