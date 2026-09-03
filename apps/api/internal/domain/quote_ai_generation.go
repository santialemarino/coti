package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// QuoteAIGeneration is the immutable identity of one AI-proposed quote draft.
type QuoteAIGeneration struct {
	ID               uuid.UUID
	AccountID        uuid.UUID
	QuoteID          uuid.UUID
	QuoteVersionID   uuid.UUID
	Provider         string
	Model            string
	PromptVersion    string
	SchemaVersion    string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CreatedAt        time.Time
}

// NewQuoteAIGeneration is the metadata recorded with an AI-proposed quote draft.
type NewQuoteAIGeneration struct {
	QuoteID          uuid.UUID
	QuoteVersionID   uuid.UUID
	Provider         string
	Model            string
	PromptVersion    string
	SchemaVersion    string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

// NewQuoteAIGenerationItem is one line exactly as the pipeline proposed it before seller edits.
type NewQuoteAIGenerationItem struct {
	Position             int
	SourceQuoteItemID    uuid.UUID
	ProductID            *uuid.UUID
	RequestedDescription string
	Quantity             decimal.Decimal
	Unit                 *string
	QuantitySource       QuantitySource
	QuantityRationale    string
	MatchStatus          ItemMatchStatus
	ConfidenceScore      decimal.NullDecimal
}

// QuoteAIGenerationItem is one preserved line from the original AI proposal.
type QuoteAIGenerationItem struct {
	ID                   uuid.UUID
	AccountID            uuid.UUID
	GenerationID         uuid.UUID
	Position             int
	SourceQuoteItemID    uuid.UUID
	ProductID            *uuid.UUID
	RequestedDescription string
	Quantity             decimal.Decimal
	Unit                 *string
	QuantitySource       QuantitySource
	QuantityRationale    string
	MatchStatus          ItemMatchStatus
	ConfidenceScore      decimal.NullDecimal
	CreatedAt            time.Time
}

// QuoteQualityDifferenceKind classifies why a whole quote was not correct.
type QuoteQualityDifferenceKind string

const (
	QuoteQualityDifferenceItemRemoved     QuoteQualityDifferenceKind = "ITEM_REMOVED"
	QuoteQualityDifferenceItemAdded       QuoteQualityDifferenceKind = "ITEM_ADDED"
	QuoteQualityDifferenceFieldChanged    QuoteQualityDifferenceKind = "FIELD_CHANGED"
	QuoteQualityDifferenceUnresolvedMatch QuoteQualityDifferenceKind = "UNRESOLVED_MATCH"
	QuoteQualityDifferenceMissingPrice    QuoteQualityDifferenceKind = "MISSING_PRICE"
	QuoteQualityDifferenceInvalidSubtotal QuoteQualityDifferenceKind = "INVALID_SUBTOTAL"
	QuoteQualityDifferenceInvalidTotal    QuoteQualityDifferenceKind = "INVALID_TOTAL"
)

// QuoteQualityEvaluation records the deterministic comparison that drives correction learning.
type QuoteQualityEvaluation struct {
	ID                  uuid.UUID
	AccountID           uuid.UUID
	GenerationID        uuid.UUID
	FinalQuoteVersionID uuid.UUID
	EvaluatorVersion    string
	WholeQuoteCorrect   bool
	SameItemCount       bool
	AllItemsEquivalent  bool
	AllItemsMatched     bool
	AllItemsPriced      bool
	AllSubtotalsValid   bool
	TotalValid          bool
	CreatedAt           time.Time
}

// NewQuoteQualityEvaluation is one completed whole-quote comparison.
type NewQuoteQualityEvaluation struct {
	GenerationID        uuid.UUID
	FinalQuoteVersionID uuid.UUID
	EvaluatorVersion    string
	WholeQuoteCorrect   bool
	SameItemCount       bool
	AllItemsEquivalent  bool
	AllItemsMatched     bool
	AllItemsPriced      bool
	AllSubtotalsValid   bool
	TotalValid          bool
}

// NewQuoteQualityDifference is one explainable reason an evaluation failed.
type NewQuoteQualityDifference struct {
	Kind             QuoteQualityDifferenceKind
	GenerationItemID *uuid.UUID
	FinalQuoteItemID *uuid.UUID
	Field            *string
	ExpectedValue    *string
	ActualValue      *string
}

// QuoteQualityFinalVersion is the frozen seller-approved quote being evaluated.
type QuoteQualityFinalVersion struct {
	Version       QuoteVersion
	Items         []QuoteItem
	DiscountTotal decimal.Decimal
}
