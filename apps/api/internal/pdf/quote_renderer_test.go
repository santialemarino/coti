package pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestQuoteRenderer_Render_ProducesAUnicodePDF(t *testing.T) {
	t.Parallel()
	payload := domain.QuoteRepresentationPayload{
		Reference: "COT-000123", VersionNumber: 1,
		ApprovedAt: time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC), Currency: "ARS",
		Supplier: domain.QuoteRepresentationSupplier{Name: "Corralón Ñandú",
			BrandColor: "#C2410C"},
		Branch: domain.QuoteRepresentationBranch{Name: "Casa central"},
		Items: []domain.QuoteRepresentationItem{{RequestedDescription: "cemento portland",
			ProductName: "Cemento común", Quantity: "2.00", UnitPrice: "1000.50",
			Subtotal: "2001.00", Alternatives: []domain.QuoteRepresentationAlternative{}}},
		Discounts: []domain.QuoteRepresentationDiscount{}, Total: "2001.00",
		ValidityNote: "Consultá la vigencia actual en el enlace de la cotización.",
	}
	content, err := NewQuoteRenderer().Render(payload, nil)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		t.Fatalf("PDF prefix = %q", content[:min(len(content), 8)])
	}
	if len(content) < 1000 {
		t.Fatalf("PDF size = %d, want embedded-font document", len(content))
	}
}

func TestDisplayQuantity_RemovesOnlyVisualZeroes(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{"10": "10", "100": "100", "10.00": "10", "2.50": "2.5", "0.25": "0.25"} {
		if got := displayQuantity(raw); got != want {
			t.Errorf("displayQuantity(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestQuoteRenderer_LongUnicodeRowsPaginate(t *testing.T) {
	t.Parallel()
	unit := "bolsa"
	payload := domain.QuoteRepresentationPayload{
		Reference: "COT-000042", VersionNumber: 2, Currency: "ARS",
		ApprovedAt: time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC),
		Supplier:   domain.QuoteRepresentationSupplier{Name: "Corralón Ñandú - Materiales para la construcción", BrandColor: "#FFFF00"},
		Branch:     domain.QuoteRepresentationBranch{Name: "Casa central"},
		Total:      "123456789012.34", ValidityNote: "Consultá la vigencia actual en el enlace de la cotización.",
	}
	for i := 0; i < 32; i++ {
		name := "Cemento de albañilería, calidad superior"
		if i == 3 {
			name = strings.Repeat("Descripción extensa con acentos y eñes. ", 100)
		}
		payload.Items = append(payload.Items, domain.QuoteRepresentationItem{
			ProductName: name, Quantity: "10.00", Unit: &unit, UnitPrice: "123456789012.34", Subtotal: "123456789012.34",
			Alternatives: []domain.QuoteRepresentationAlternative{{Name: "Alternativa de calidad equivalente aprobada por el vendedor", UnitPrice: "123456789012.34"}},
		})
	}
	content, err := NewQuoteRenderer().Render(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(content, []byte("/Type /Page\n")) < 3 {
		t.Fatal("long rows did not produce multiple pages")
	}
	if directory := os.Getenv("TEST_QUOTE_PDF_OUTPUT_DIR"); directory != "" {
		if err := os.WriteFile(filepath.Join(directory, "quote-pagination.pdf"), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
