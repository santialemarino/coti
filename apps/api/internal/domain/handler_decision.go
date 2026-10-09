package domain

import (
	"github.com/google/uuid"
)

// NewHandlerDecision is one unreviewed AI proposal recorded for pilot quality metrics.
type NewHandlerDecision struct {
	QuoteVersionID   uuid.UUID
	ClientInput      string
	AIInterpretation string
	AIProposal       string
	StateAtDecision  QuoteStatus
}
