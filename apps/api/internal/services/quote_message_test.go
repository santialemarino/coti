package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func quoteMessagePayload(lines int) domain.QuoteRepresentationPayload {
	unit := "bolsa"
	payload := domain.QuoteRepresentationPayload{Reference: "COT-000042", Currency: "ARS",
		Supplier: domain.QuoteRepresentationSupplier{Name: "Corralón Centro"},
		Total:    "1234567.50", ValidityNote: quoteValidityNote}
	for i := range lines {
		payload.Items = append(payload.Items, domain.QuoteRepresentationItem{
			ProductName: fmt.Sprintf("Producto %d", i+1), Quantity: "2.50", Unit: &unit, Subtotal: "100.00"})
	}
	return payload
}

func TestBuildQuoteWhatsAppMessage_ListsEightLinesThenTheRest(t *testing.T) {
	t.Parallel()
	validUntil := time.Date(2026, 12, 3, 2, 0, 0, 0, time.UTC) // still Dec 2 in Argentina.
	message := buildQuoteWhatsAppMessage(quoteMessagePayload(11), &validUntil, quoteMessageOrderLink)

	if got := strings.Count(message, "\n• "); got != quoteMessageMaxItems {
		t.Errorf("listed lines = %d, want %d:\n%s", got, quoteMessageMaxItems, message)
	}
	for _, expected := range []string{"Hola.\n", "• 2,5 bolsa · Producto 8 — ARS 100,00\ny 3 más\n",
		"Total: ARS 1.234.567,50", "Válida hasta el 2/12/2026.", "aceptala acá:\n" + domain.QuotePublicURLPlaceholder} {
		if !strings.Contains(message, expected) {
			t.Errorf("message does not contain %q:\n%s", expected, message)
		}
	}
	if strings.Contains(message, "Producto 9") {
		t.Errorf("message lists a ninth line:\n%s", message)
	}
}

func TestBuildQuoteWhatsAppMessage_ListsEveryLineWhenTheyFit(t *testing.T) {
	t.Parallel()
	message := buildQuoteWhatsAppMessage(quoteMessagePayload(quoteMessageMaxItems), nil, quoteMessageOrderLink)
	if strings.Contains(message, "más\n") {
		t.Errorf("message announces more lines than it has:\n%s", message)
	}
	if !strings.Contains(message, quoteValidityNote) {
		t.Errorf("a message with no expiry yet does not fall back to the validity note:\n%s", message)
	}
}

func TestBuildQuoteWhatsAppMessage_InvitesAContactWithoutAnAccount(t *testing.T) {
	t.Parallel()
	message := buildQuoteWhatsAppMessage(quoteMessagePayload(1), nil, quoteMessageInvitation)
	if !strings.Contains(message, "creá tu cuenta en Corralón Centro:\n"+domain.QuotePublicURLPlaceholder) {
		t.Errorf("message carries no sign-up invitation:\n%s", message)
	}
	if strings.Contains(message, "aceptala acá") {
		t.Errorf("an invitation also offers the order link:\n%s", message)
	}
	if strings.Count(message, domain.QuotePublicURLPlaceholder) != 1 {
		t.Errorf("placeholder count = %d, want 1", strings.Count(message, domain.QuotePublicURLPlaceholder))
	}
}

func TestBuildQuoteEmail_NamesThePDFOnlyWhenAttached(t *testing.T) {
	t.Parallel()
	for _, withPDF := range []bool{true, false} {
		email := buildQuoteEmail(quoteMessagePayload(9), nil, quoteMessageInvitation, withPDF)
		joined := strings.Join(email.Paragraphs, "\n")
		if strings.Contains(joined, "Te adjuntamos el PDF") != withPDF {
			t.Errorf("withPDF=%v: paragraphs = %q", withPDF, joined)
		}
		if email.ActionLabel != "Crear mi cuenta" || len(email.Items) != quoteMessageMaxItems+1 ||
			email.Items[quoteMessageMaxItems] != "y 1 más" {
			t.Errorf("withPDF=%v: email = %+v", withPDF, email)
		}
	}
}

func TestQuoteMessageLink_SendsAnInvitationThroughSignUpBackToTheOrder(t *testing.T) {
	t.Parallel()
	base := "https://corralon.coti.test"
	if got := quoteMessageLink(base, "tok-1", quoteMessageOrderLink); got != base+"/quotes/tok-1" {
		t.Errorf("order link = %q", got)
	}
	if got := quoteMessageLink(base, "tok-1", quoteMessageInvitation); got != base+"/signup?next=%2Fquotes%2Ftok-1" {
		t.Errorf("invitation link = %q", got)
	}
}

func TestFormatQuantity_DropsTrailingZeros(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{"10.00": "10", "2.50": "2,5", "0.75": "0,75", "3": "3"} {
		if got := formatQuantity(raw); got != want {
			t.Errorf("formatQuantity(%q) = %q, want %q", raw, got, want)
		}
	}
}
