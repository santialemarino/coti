package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.FollowupDrafter = (*FollowupDrafter)(nil)

// FollowupDrafter writes a follow-up message for a sent quote the client has not answered.
type FollowupDrafter struct {
	generator domain.StructuredGenerator
	now       func() time.Time
}

// NewFollowupDrafter builds a FollowupDrafter on the bound language model.
func NewFollowupDrafter(generator domain.StructuredGenerator) *FollowupDrafter {
	return &FollowupDrafter{generator: generator, now: time.Now}
}

// Draft asks for one short message and returns it trimmed; an empty answer is no draft.
func (d *FollowupDrafter) Draft(ctx context.Context, in domain.FollowupDraftInput) (string, error) {
	ctx = domain.WithAIOperation(ctx, domain.AIOperationFollowupDraft)
	payload, err := json.Marshal(followupPrompt{
		Supplier: in.SupplierName, Seller: in.SellerName, Client: in.ClientName,
		Quote: in.QuoteNumber, LastContact: in.QuietSince.Format("2006-01-02"),
		DaysQuiet: int(d.now().Sub(in.QuietSince).Hours() / 24), OfferExpired: in.Lapsed,
		Items: in.Items,
	})
	if err != nil {
		return "", err
	}
	var answer followupAnswer
	if _, err := d.generator.Generate(ctx, domain.GenerationRequest{
		Instructions: followupInstructions,
		Input:        []domain.Content{domain.TextContent(string(payload))},
		Schema:       followupSchema(),
	}, &answer); err != nil {
		return "", err
	}
	message := strings.TrimSpace(answer.Message)
	if message == "" {
		return "", fmt.Errorf("%w: the follow-up draft came back empty", domain.ErrAIUnavailable)
	}
	return message, nil
}

// followupInstructions is the stable half of the prompt, ahead of the quote it is about.
const followupInstructions = `You write follow-up messages for a seller at a building-materials supplier in Argentina. The seller sent the client a quote some days ago and the client has not answered. The seller will read your draft, edit it if needed, and send it themselves by WhatsApp or email.

Write one short message in Argentine Spanish (voseo), friendly and professional, the way a seller writes to a client they know: two to four sentences, no subject line, no signature block beyond the seller's first name when it is given. Greet the client by name when it is given. Name the quote by its number and, when it helps, the main materials in a few words. Ask whether they had a chance to look at it and offer to adjust quantities, products or delivery. When the offer expired, say the prices may have changed and offer to send it again updated.

Never state a price, a total, a discount, a percentage, a delivery date or a stock figure, and never promise one: the quote itself carries them. Never invent anything the input does not say, and describe the time since the quote went out to match days_without_answer. Do not pressure the client or add urgency.`

type followupPrompt struct {
	Supplier     string   `json:"supplier"`
	Seller       string   `json:"seller_name,omitempty"`
	Client       string   `json:"client_name,omitempty"`
	Quote        int64    `json:"quote_number"`
	LastContact  string   `json:"quote_sent_or_last_changed_on"`
	DaysQuiet    int      `json:"days_without_answer"`
	OfferExpired bool     `json:"offer_expired"`
	Items        []string `json:"items"`
}

type followupAnswer struct {
	Message string `json:"message"`
}

func followupSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"message"},
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "The follow-up message, in Argentine Spanish, ready for the seller to send.",
			},
		},
	}
}
