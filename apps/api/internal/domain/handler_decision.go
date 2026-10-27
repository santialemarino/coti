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
	// HandlerNoMatchRecipesExhausted means every recipe for the customer's goal came up empty.
	HandlerNoMatchRecipesExhausted HandlerNoMatchReason = "PIPELINE_EXHAUSTED"
	// HandlerNoMatchPromotionNotEarned means the promotion the customer asked for does not apply.
	HandlerNoMatchPromotionNotEarned HandlerNoMatchReason = "PROMOTION_NOT_EARNED"
)

// HandlerStrategy is one recipe the backend ran for a complex intent.
type HandlerStrategy string

const (
	// HandlerStrategyFindCheaperAlternatives swaps lines for their cheapest cheaper ECONOMY alternative.
	HandlerStrategyFindCheaperAlternatives HandlerStrategy = "FIND_CHEAPER_ALTERNATIVES"
	// HandlerStrategyImproveQuality swaps lines for their cheapest PREMIUM alternative.
	HandlerStrategyImproveQuality HandlerStrategy = "IMPROVE_QUALITY"
	// HandlerStrategyAdaptToReachDiscount raises quoted quantities to a promotion's threshold.
	HandlerStrategyAdaptToReachDiscount HandlerStrategy = "ADAPT_TO_REACH_DISCOUNT"
)

// IsLowerPriceStrategy reports whether a strategy is one a lower-price goal can run first.
func (s HandlerStrategy) IsLowerPriceStrategy() bool {
	return s == HandlerStrategyFindCheaperAlternatives || s == HandlerStrategyAdaptToReachDiscount
}

// HandlerAdaptation is what ADAPT_TO_REACH_DISCOUNT did to the draft, with the engine's totals.
type HandlerAdaptation struct {
	Lines       []HandlerAdaptedLine `json:"lines"`
	TotalBefore string               `json:"total_before"`
	TotalAfter  string               `json:"total_after"`
	// TotalRaised is true when the customer asked for the promotion and reaching it costs more.
	TotalRaised bool `json:"total_raised"`
}

// HandlerAdaptedLine is one line raised to reach a promotion. Quantities are decimal strings.
type HandlerAdaptedLine struct {
	ItemID        uuid.UUID `json:"item_id"`
	Description   string    `json:"description"`
	FromQuantity  string    `json:"from_quantity"`
	ToQuantity    string    `json:"to_quantity"`
	PromotionID   uuid.UUID `json:"promotion_id"`
	PromotionName string    `json:"promotion_name"`
}

// HandlerProposalRecord is what handler_decision.ai_proposal stores for one batch.
type HandlerProposalRecord struct {
	Outcome        HandlerOutcome        `json:"outcome"`
	NoMatchOrigin  *HandlerNoMatchOrigin `json:"no_match_origin,omitempty"`
	NoMatchReason  *HandlerNoMatchReason `json:"no_match_reason,omitempty"`
	Interpretation string                `json:"interpretation"`
	Intent         *ChangeRequestIntent  `json:"intent,omitempty"`
	// IntentItemIDs are the draft lines a complex intent was tied to; empty for the whole quote.
	IntentItemIDs []uuid.UUID `json:"intent_item_ids,omitempty"`
	// StrategiesTried lists the recipes run for a complex intent; empty for a direct request.
	StrategiesTried []HandlerStrategy `json:"strategies_tried,omitempty"`
	// PreferredStrategy is the lower-price strategy the customer's own wording asked for.
	PreferredStrategy *HandlerStrategy         `json:"preferred_strategy,omitempty"`
	Adaptation        *HandlerAdaptation       `json:"adaptation,omitempty"`
	Operations        []ChangeRequestOperation `json:"operations"`
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
	ClientInput     string
	Interpretation  string
	Outcome         HandlerOutcome
	NoMatchReason   *HandlerNoMatchReason
	StrategiesTried []HandlerStrategy
	Adaptation      *HandlerAdaptation
	SellerDecision  *HandlerSellerDecision
	CreatedAt       time.Time
}

// HandlerDecisionReview is a handler run as the conversation panel shows it.
type HandlerDecisionReview struct {
	VersionID       uuid.UUID
	Interpretation  string
	Outcome         HandlerOutcome
	NoMatchReason   *HandlerNoMatchReason
	StrategiesTried []HandlerStrategy
	Adaptation      *HandlerAdaptation
	SellerDecision  *HandlerSellerDecision
	DecidedAt       *time.Time
	CreatedAt       time.Time
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
