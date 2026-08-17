package ai

import (
	"context"
	"fmt"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// DisabledRFQExtractor fails RFQ extraction when no provider is configured.
type DisabledRFQExtractor struct{}

// NewDisabledRFQExtractor builds a DisabledRFQExtractor.
func NewDisabledRFQExtractor() *DisabledRFQExtractor {
	return &DisabledRFQExtractor{}
}

// Extract returns domain.ErrInvalidInput because RFQ AI is disabled.
func (e *DisabledRFQExtractor) Extract(_ context.Context, _ string) (domain.RFQExtraction, error) {
	return domain.RFQExtraction{}, fmt.Errorf("%w: rfq extractor is disabled", domain.ErrInvalidInput)
}
