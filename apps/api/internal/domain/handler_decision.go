package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// HandlerOutcome says whether a handler run changed the draft or left the batch to the seller.
type HandlerOutcome string

const (
	HandlerOutcomeApplied HandlerOutcome = "APPLIED"
	HandlerOutcomeNoMatch HandlerOutcome = "NO_MATCH"
)

// HandlerSellerDecision is what the seller did with a handler run, written when it is settled.
type HandlerSellerDecision string

const (
	HandlerDecisionApprovedAsIs   HandlerSellerDecision = "APPROVED_AS_IS"
	HandlerDecisionEdited         HandlerSellerDecision = "EDITED"
	HandlerDecisionRejected       HandlerSellerDecision = "REJECTED"
	HandlerDecisionManualOverride HandlerSellerDecision = "MANUAL_OVERRIDE"
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
	DraftBefore     DraftSnapshot
	DraftAfter      DraftSnapshot
}

// HandlerDecisionNote is the latest handler decision on a draft, as the seller reads it.
type HandlerDecisionNote struct {
	ClientInput    string
	Interpretation string
	Outcome        HandlerOutcome
	NoMatchReason  *HandlerNoMatchReason
	SellerDecision *HandlerSellerDecision
	CreatedAt      time.Time
}

// HandlerDecisionReview is a handler run as the conversation panel shows it.
type HandlerDecisionReview struct {
	VersionID      uuid.UUID
	Interpretation string
	Outcome        HandlerOutcome
	NoMatchReason  *HandlerNoMatchReason
	SellerDecision *HandlerSellerDecision
	DecidedAt      *time.Time
	CreatedAt      time.Time
}

// HandlerDecisionTrace is a recorded run on one draft, in the order it happened, for settling.
type HandlerDecisionTrace struct {
	ID             uuid.UUID
	Outcome        HandlerOutcome
	SellerDecision *HandlerSellerDecision
	DraftBefore    json.RawMessage
	DraftAfter     json.RawMessage
}

// DraftSnapshot is what a seller controls on a draft: its lines and the discounts they typed or
// suppressed. Prices are left out, so repricing a draft never reads as an edit.
type DraftSnapshot struct {
	Items     []DraftSnapshotItem     `json:"items"`
	Discounts []DraftSnapshotDiscount `json:"discounts"`
}

// DraftSnapshotItem is one line as the seller shaped it.
type DraftSnapshotItem struct {
	ProductID            *uuid.UUID  `json:"product_id"`
	RequestedDescription string      `json:"requested_description"`
	Quantity             string      `json:"quantity"`
	Unit                 *string     `json:"unit"`
	ApprovedAlternatives []uuid.UUID `json:"approved_alternatives"`
}

// DraftSnapshotDiscount is a seller-typed discount, or an automatic one the seller suppressed.
type DraftSnapshotDiscount struct {
	Origin      DiscountOrigin      `json:"origin"`
	PromotionID *uuid.UUID          `json:"promotion_id"`
	ActionType  PromotionActionType `json:"action_type"`
	Value       *string             `json:"value"`
	Scope       DiscountScope       `json:"scope"`
	Suppressed  bool                `json:"suppressed"`
	// Items are the covered lines' positions in DraftSnapshot.Items.
	Items []int `json:"items"`
}

// SettleHandlerDecisions decides every pending run on a draft being frozen for sending, given the
// draft as it is frozen. An applied run is APPROVED_AS_IS when the seller changed nothing after it
// and EDITED otherwise; a no-match run was resolved by hand. A snapshot that cannot be read counts
// as an edit.
func SettleHandlerDecisions(runs []HandlerDecisionTrace, final DraftSnapshot,
) map[uuid.UUID]HandlerSellerDecision {
	decisions := make(map[uuid.UUID]HandlerSellerDecision)
	finalJSON, err := json.Marshal(final)
	edited := err != nil
	for i := len(runs) - 1; i >= 0; i-- {
		run := runs[i]
		if i == len(runs)-1 {
			edited = edited || !sameSnapshot(run.DraftAfter, finalJSON)
		} else {
			edited = edited || !sameSnapshot(runs[i+1].DraftBefore, run.DraftAfter)
		}
		if run.SellerDecision != nil {
			continue
		}
		switch {
		case run.Outcome == HandlerOutcomeNoMatch:
			decisions[run.ID] = HandlerDecisionManualOverride
		case edited:
			decisions[run.ID] = HandlerDecisionEdited
		default:
			decisions[run.ID] = HandlerDecisionApprovedAsIs
		}
	}
	return decisions
}

// sameSnapshot compares two stored snapshots by content, since jsonb does not keep the bytes.
func sameSnapshot(a, b json.RawMessage) bool {
	var left, right DraftSnapshot
	if len(a) == 0 || len(b) == 0 || json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	leftJSON, errLeft := json.Marshal(left)
	rightJSON, errRight := json.Marshal(right)
	return errLeft == nil && errRight == nil && string(leftJSON) == string(rightJSON)
}
