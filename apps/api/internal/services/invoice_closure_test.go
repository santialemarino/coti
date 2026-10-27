package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// An invoice ARCA authorizes ends «No requiere factura»; one it refuses leaves the seller's decision.
func TestInvoiceService_IssueClearsTheWaiverOnlyWhenAuthorized(t *testing.T) {
	t.Parallel()
	for _, refused := range []bool{false, true} {
		f := newInvoiceFixture(t)
		now, waiver := time.Now(), uuid.New()
		f.quotes.quote.InvoiceWaivedAt, f.quotes.quote.InvoiceWaivedByUserID = &now, &waiver
		if refused {
			f.issuer.auth, f.issuer.err = nil, &domain.InvoiceRejectedError{Issues: []string{"10013: DocTipo"}}
		}

		_, err := f.issue(context.Background())

		if refused {
			var rejection *domain.InvoiceRejectedError
			if !errors.As(err, &rejection) || f.quotes.waiverCleared != 0 {
				t.Fatalf("refused: err = %v, cleared = %d, want the refusal and the waiver kept", err, f.quotes.waiverCleared)
			}
			continue
		}
		if err != nil || f.quotes.waiverCleared != 1 || f.quotes.quote.InvoiceWaivedAt != nil {
			t.Fatalf("authorized: err = %v, cleared = %d, want the waiver cleared once", err, f.quotes.waiverCleared)
		}
	}
}

// The account default for «Facturar también» round-trips through the settings, and a body that
// leaves it out keeps it.
func TestInvoiceService_SettingsCarryTheInvoiceOnPaymentDefault(t *testing.T) {
	t.Parallel()
	f := newInvoiceFixture(t)
	on := true

	settings, err := f.service.UpdateSettings(context.Background(), f.tenant, domain.InvoicingSettingsUpdate{InvoiceOnPayment: &on})
	if err != nil || !settings.InvoiceOnPayment {
		t.Fatalf("UpdateSettings(on) = %+v, %v, want the default on", settings, err)
	}
	settings, err = f.service.UpdateSettings(context.Background(), f.tenant, domain.InvoicingSettingsUpdate{})
	if err != nil || !settings.InvoiceOnPayment {
		t.Fatalf("UpdateSettings(omitted) = %+v, %v, want the default kept", settings, err)
	}
}
