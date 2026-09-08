package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type stubLogoLoader struct {
	logo *domain.BrandLogo
	err  error
}

func (l stubLogoLoader) Load(context.Context, string) (*domain.BrandLogo, error) {
	return l.logo, l.err
}

func TestValidateRepresentationSource_AcceptsConsistentFrozenValues(t *testing.T) {
	t.Parallel()
	source := representationSourceFixture()
	if err := validateRepresentationSource(source); err != nil {
		t.Fatalf("validateRepresentationSource() = %v, want nil", err)
	}
}

func TestValidateRepresentationSource_RejectsIncompleteAndInconsistentQuotes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*domain.QuoteRepresentationSource)
	}{
		{"no items", func(source *domain.QuoteRepresentationSource) { source.Items = nil }},
		{"unpriced item", func(source *domain.QuoteRepresentationSource) {
			source.Items[0].UnitPriceSnapshot = decimal.NullDecimal{}
		}},
		{"wrong subtotal", func(source *domain.QuoteRepresentationSource) {
			source.Items[0].Subtotal = decimal.NewNullDecimal(decimal.RequireFromString("999.00"))
		}},
		{"approved alternative without price", func(source *domain.QuoteRepresentationSource) {
			source.Alternatives[source.Items[0].ID][0].PriceSnapshot = decimal.NullDecimal{}
		}},
		{"negative discount", func(source *domain.QuoteRepresentationSource) {
			source.Discounts[0].Amount = "-1.00"
		}},
		{"wrong total", func(source *domain.QuoteRepresentationSource) {
			source.Version.Total = decimal.RequireFromString("201.00")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := representationSourceFixture()
			tc.mutate(&source)
			err := validateRepresentationSource(source)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			if domain.CodeOf(err) != domain.CodeQuoteRepresentationInvalid {
				t.Fatalf("code = %q, want %q", domain.CodeOf(err),
					domain.CodeQuoteRepresentationInvalid)
			}
		})
	}
}

func TestBuildQuoteMessage_UsesReferenceTotalAndOneURLPlaceholder(t *testing.T) {
	t.Parallel()
	payload := buildRepresentationPayload(representationSourceFixture(),
		time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC))
	message := buildQuoteMessage(payload)
	for _, expected := range []string{"Hola, Obra Norte.", "COT-000042", "ARS 200,00"} {
		if !strings.Contains(message, expected) {
			t.Errorf("message does not contain %q: %s", expected, message)
		}
	}
	if strings.Count(message, domain.QuotePublicURLPlaceholder) != 1 {
		t.Fatalf("placeholder count = %d, want 1", strings.Count(message,
			domain.QuotePublicURLPlaceholder))
	}
}

func TestQuoteRepresentationService_LoadLogoFallsBackWithoutFailingGeneration(t *testing.T) {
	t.Parallel()
	rawURL := "https://cdn.example.test/logo.png"
	service := &QuoteRepresentationService{logos: stubLogoLoader{err: errors.New("unavailable")},
		log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	logo, fallback := service.loadLogo(context.Background(), &rawURL, uuid.New())
	if logo != nil || !fallback {
		t.Fatalf("logo/fallback = %v/%v, want nil/true", logo, fallback)
	}
	service.logos = stubLogoLoader{logo: &domain.BrandLogo{Bytes: []byte("png"), ContentType: "image/png"}}
	logo, fallback = service.loadLogo(context.Background(), &rawURL, uuid.New())
	if logo == nil || fallback {
		t.Fatalf("logo/fallback = %v/%v, want logo/false", logo, fallback)
	}
}

func representationSourceFixture() domain.QuoteRepresentationSource {
	itemID := uuid.New()
	productName, productCode, unit := "Cemento", "CEM-01", "bolsa"
	customer := "Obra Norte"
	return domain.QuoteRepresentationSource{
		Account: domain.Account{Name: "Corralón Centro"},
		Branch:  domain.Branch{Name: "Casa central"},
		Quote:   domain.Quote{Number: 42},
		Version: domain.QuoteVersion{VersionNumber: 1, Currency: "ARS",
			Total: decimal.RequireFromString("200.00")},
		CustomerName: &customer,
		Items: []domain.QuoteItem{{ID: itemID, ProductID: uuidPointer(uuid.New()),
			MatchStatus:          domain.ItemMatchStatusMatched,
			RequestedDescription: "dos bolsas", Quantity: decimal.RequireFromString("2.00"),
			Unit: &unit, UnitPriceSnapshot: decimal.NewNullDecimal(decimal.RequireFromString("110.00")),
			Subtotal:    decimal.NewNullDecimal(decimal.RequireFromString("220.00")),
			ProductCode: &productCode, ProductName: &productName, ProductUnit: &unit}},
		Alternatives: map[uuid.UUID][]domain.QuoteItemAlternative{itemID: {{
			CanonicalName: &productName, Code: &productCode, Unit: &unit,
			PriceSnapshot:    decimal.NewNullDecimal(decimal.RequireFromString("105.00")),
			ApprovedBySeller: true}}},
		Discounts: []domain.QuoteRepresentationDiscount{{Description: "Bonificación",
			Amount: "20.00"}},
	}
}

func uuidPointer(id uuid.UUID) *uuid.UUID { return &id }
