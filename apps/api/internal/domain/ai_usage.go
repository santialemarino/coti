package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AIOperation is what an AI provider call was paid for.
type AIOperation string

// The operations that spend on a provider, one per call site.
const (
	AIOperationRFQExtraction        AIOperation = "RFQ_EXTRACTION"
	AIOperationAudioTranscription   AIOperation = "AUDIO_TRANSCRIPTION"
	AIOperationInterpretationLookup AIOperation = "INTERPRETATION_LOOKUP"
	AIOperationCatalogSearch        AIOperation = "CATALOG_SEARCH"
	AIOperationCatalogMatchReview   AIOperation = "CATALOG_MATCH_REVIEW"
	AIOperationCatalogEmbedding     AIOperation = "CATALOG_EMBEDDING"
	AIOperationCorrectionLearning   AIOperation = "CORRECTION_LEARNING"
)

// AIUsageScope is whom an AI call is spent for and on what. It travels in the context, from the
// service that knows the account down to the adapter that knows the cost.
type AIUsageScope struct {
	AccountID uuid.UUID
	BranchID  *uuid.UUID
	RFQID     *uuid.UUID
	Operation AIOperation
}

type aiUsageScopeKey struct{}

// AIUsageScopeFrom returns the scope the context carries, zero when it carries none.
func AIUsageScopeFrom(ctx context.Context) AIUsageScope {
	scope, _ := ctx.Value(aiUsageScopeKey{}).(AIUsageScope)
	return scope
}

// WithAIAccount attributes the AI calls made under ctx to the tenant's account and branch.
func WithAIAccount(ctx context.Context, tenant Tenant) context.Context {
	scope := AIUsageScopeFrom(ctx)
	scope.AccountID = tenant.AccountID
	scope.BranchID = nil
	if tenant.BranchID != uuid.Nil {
		branchID := tenant.BranchID
		scope.BranchID = &branchID
	}
	return context.WithValue(ctx, aiUsageScopeKey{}, scope)
}

// WithAIRFQ attributes the AI calls made under ctx to the order they serve.
func WithAIRFQ(ctx context.Context, rfqID uuid.UUID) context.Context {
	scope := AIUsageScopeFrom(ctx)
	scope.RFQID = &rfqID
	return context.WithValue(ctx, aiUsageScopeKey{}, scope)
}

// WithAIOperation names what the AI calls made under ctx are paid for.
func WithAIOperation(ctx context.Context, operation AIOperation) context.Context {
	scope := AIUsageScopeFrom(ctx)
	scope.Operation = operation
	return context.WithValue(ctx, aiUsageScopeKey{}, scope)
}

// AIUsage is what one provider call consumed, summed over its attempts, and whom it served.
type AIUsage struct {
	AIUsageScope
	Provider  string
	Model     string
	Succeeded bool
	Attempts  int
	Elapsed   time.Duration
	// The cache figures are priced apart and are not part of InputTokens.
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	// AudioSeconds is what a provider that bills transcription by duration reported.
	AudioSeconds float64
}

// AIUsageRecorder keeps the ledger of AI spend. Recording never fails the call it describes: the
// spend already happened, so a recorder that cannot write reports it and moves on.
type AIUsageRecorder interface {
	Record(ctx context.Context, usage AIUsage)
}
