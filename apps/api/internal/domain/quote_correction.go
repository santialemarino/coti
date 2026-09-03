package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"github.com/shopspring/decimal"
)

// QuoteCorrectionMemoryKind separates order interpretation from catalog selection evidence.
type QuoteCorrectionMemoryKind string

const (
	QuoteCorrectionMemoryInterpretation QuoteCorrectionMemoryKind = "INTERPRETATION"
	QuoteCorrectionMemoryCatalog        QuoteCorrectionMemoryKind = "CATALOG"
)

// CorrectedQuoteLine is seller-approved extraction evidence with no pricing or catalog data.
type CorrectedQuoteLine struct {
	RequestedDescription string          `json:"requested_description"`
	Quantity             decimal.Decimal `json:"quantity"`
	Unit                 *string         `json:"unit"`
}

// NewQuoteCorrectionMemory is one relevant seller correction awaiting vectorization.
type NewQuoteCorrectionMemory struct {
	Kind           QuoteCorrectionMemoryKind
	SourceText     string
	CorrectedItems []CorrectedQuoteLine
	ProductID      *uuid.UUID
	SourceKey      string
}

// QuoteCorrectionMemory is one account-local correction pattern.
type QuoteCorrectionMemory struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	Kind           QuoteCorrectionMemoryKind
	SourceText     string
	CorrectedItems []CorrectedQuoteLine
	ProductID      *uuid.UUID
	Embedding      *pgvector.Vector
	CreatedAt      time.Time
}

// RFQInterpretationExample is a previous order and its seller-approved interpretation.
type RFQInterpretationExample struct {
	SourceText     string
	CorrectedItems []CorrectedQuoteLine
}
