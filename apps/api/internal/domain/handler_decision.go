package domain

import (
	"time"

	"github.com/google/uuid"
)

// HandlerOutcome says whether a handler run changed the draft or left the batch to the seller.
type HandlerOutcome string

const (
	HandlerOutcomeApplied HandlerOutcome = "APPLIED"
	HandlerOutcomeNoMatch HandlerOutcome = "NO_MATCH"
)

// HandlerNoMatchOrigin separates a request never classified from one whose recipes all failed.
type HandlerNoMatchOrigin string

const (
	HandlerNoMatchDirectInput       HandlerNoMatchOrigin = "DIRECT_INPUT"
	HandlerNoMatchPipelineExhausted HandlerNoMatchOrigin = "PIPELINE_EXHAUSTED"
)

// HandlerNoMatchReason is why a batch was left to the seller; the backoffice labels it.
type HandlerNoMatchReason string

const (
	// HandlerNoMatchRequiresSeller is the model's own escape: ambiguous, off-catalog, or an
	// acceptance or rejection, which the AI never infers.
	HandlerNoMatchRequiresSeller HandlerNoMatchReason = "REQUIRES_SELLER"
	// HandlerNoMatchAIDisabled means no language model is configured.
	HandlerNoMatchAIDisabled HandlerNoMatchReason = "AI_DISABLED"
	// HandlerNoMatchProposalRefused means the backend refused the model's proposal.
	HandlerNoMatchProposalRefused HandlerNoMatchReason = "PROPOSAL_REFUSED"
)

// HandlerProposalRecord is what handler_decision.ai_proposal stores for one batch.
type HandlerProposalRecord struct {
	Outcome        HandlerOutcome           `json:"outcome"`
	NoMatchOrigin  *HandlerNoMatchOrigin    `json:"no_match_origin,omitempty"`
	NoMatchReason  *HandlerNoMatchReason    `json:"no_match_reason,omitempty"`
	Interpretation string                   `json:"interpretation"`
	Operations     []ChangeRequestOperation `json:"operations"`
}

// NewHandlerDecision is one unreviewed AI proposal recorded for pilot quality metrics.
type NewHandlerDecision struct {
	QuoteVersionID   uuid.UUID
	MessageBatchID   *uuid.UUID
	ClientInput      string
	AIInterpretation string
	AIProposal       string
	// StateAtDecision is the quote status the handler acted on, read before any flip it caused.
	StateAtDecision QuoteStatus
}

// HandlerDecisionNote is the latest handler decision on a draft, as the seller reads it.
type HandlerDecisionNote struct {
	ClientInput    string
	Interpretation string
	Outcome        HandlerOutcome
	NoMatchReason  *HandlerNoMatchReason
	CreatedAt      time.Time
}
