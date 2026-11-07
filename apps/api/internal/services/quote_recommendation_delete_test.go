package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestDeleteRecommendationAlternative_OnlyMutableQuotedVersion(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    domain.QuoteStatus
		immutable bool
		want      error
	}{
		{"quoted", domain.QuoteStatusQuoted, false, nil},
		{"sent", domain.QuoteStatusSent, false, domain.ErrConflict},
		{"immutable", domain.QuoteStatusQuoted, true, domain.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := pricedLine(uuid.New(), "1")
			f := newQuoteFixture([]domain.QuoteItem{item}, nil)
			f.quotes.quote.CurrentStatus = tc.status
			f.quotes.version.IsImmutable = tc.immutable
			kind := domain.ProductAlternativeEconomy
			id := uuid.New()
			f.quotes.alternatives = map[uuid.UUID][]domain.QuoteItemAlternative{
				item.ID: {{ID: id, RecommendationType: &kind}},
			}
			err := f.service.DeleteRecommendationAlternative(context.Background(), f.tenant, f.quoteID, item.ID, id)
			if !errors.Is(err, tc.want) {
				t.Fatalf("delete error = %v, want %v", err, tc.want)
			}
			wantLength := 1
			if tc.want == nil {
				wantLength = 0
			}
			if got := len(f.quotes.alternatives[item.ID]); got != wantLength {
				t.Fatalf("alternatives = %d, want %d", got, wantLength)
			}
		})
	}
}
