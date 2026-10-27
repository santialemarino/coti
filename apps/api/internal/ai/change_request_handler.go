package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.ChangeRequestHandler = (*ChangeRequestHandler)(nil)

// ChangeRequestHandler interprets a customer revision request against the lines they received.
type ChangeRequestHandler struct {
	generator domain.StructuredGenerator
}

// NewChangeRequestHandler builds a provider-agnostic customer change handler.
func NewChangeRequestHandler(generator domain.StructuredGenerator) *ChangeRequestHandler {
	return &ChangeRequestHandler{generator: generator}
}

// Propose returns typed changes without mutating a quote or calculating any monetary value. A
// NO_MATCH answer becomes one REQUIRES_SELLER operation, the domain's escape.
func (h *ChangeRequestHandler) Propose(ctx context.Context,
	in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	history := in.Context
	if history == nil {
		history = []domain.ConversationLine{}
	}
	promotions := in.Promotions
	if promotions == nil {
		promotions = []domain.ChangeRequestPromotion{}
	}
	payload, err := json.Marshal(changeRequestPayload{Context: history, Message: in.Message,
		Items: in.Items, Promotions: promotions})
	if err != nil {
		return nil, err
	}

	var answer changeRequestAnswer
	ctx = domain.WithAIOperation(ctx, domain.AIOperationChangeRequestHandling)
	usage, err := h.generator.Generate(ctx, domain.GenerationRequest{
		Instructions: changeRequestInstructions,
		Input:        []domain.Content{domain.TextContent(string(payload))},
		Schema:       changeRequestSchema(),
	}, &answer)
	if err != nil {
		return nil, err
	}
	if usage == nil {
		return nil, fmt.Errorf("%w: change request handler returned no generation usage", domain.ErrInvalidInput)
	}

	proposal := &domain.ChangeRequestProposal{Level: domain.ChangeRequestLevel(answer.Level),
		Interpretation: answer.Interpretation,
		Operations:     make([]domain.ChangeRequestOperation, 0, len(answer.Operations))}
	if answer.Intent != nil {
		intent := domain.ChangeRequestIntent(*answer.Intent)
		proposal.Intent = &intent
	}
	if answer.LowerPriceStrategy != nil {
		strategy := domain.HandlerStrategy(*answer.LowerPriceStrategy)
		proposal.LowerPriceStrategy = &strategy
	}
	for index, raw := range answer.IntentItemIDs {
		id, parseErr := uuid.Parse(strings.TrimSpace(raw))
		if parseErr != nil {
			return nil, fmt.Errorf("%w: intent_item_ids[%d] is invalid", domain.ErrInvalidInput, index)
		}
		proposal.IntentItemIDs = append(proposal.IntentItemIDs, id)
	}
	for index, operation := range answer.Operations {
		mapped, mapErr := mapChangeRequestOperation(operation, index)
		if mapErr != nil {
			return nil, mapErr
		}
		proposal.Operations = append(proposal.Operations, mapped)
	}
	if proposal.Level == domain.ChangeRequestLevelNoMatch {
		proposal.Operations = append(proposal.Operations,
			domain.ChangeRequestOperation{Type: domain.ChangeRequestOperationRequiresSeller})
	}
	return proposal, nil
}

const changeRequestInstructions = `You interpret a customer request to revise a building-materials quote for an Argentine supplier. The current quote lines are supplied with opaque IDs, and the promotions in force are supplied with opaque IDs.

customer_message holds the customer's new messages, oldest first, one per line. conversation_context holds earlier messages of the same conversation, from the customer (CLIENT), the seller (SELLER) or the system (SYSTEM); it is context only: never turn a context message into a change, and never repeat a change it shows was already requested.

First choose the level:
- ATOMIC: the customer states concrete changes. Return them as operations; intent and lower_price_strategy are null and intent_item_ids is empty.
- COMPLEX: the customer states a goal without saying how to reach it, and the goal is one of the intents below. Return no operations; the supplier's system reaches the goal itself.
- NO_MATCH: the request is ambiguous, unsupported, asks about something other than materials, or cannot be tied to the supplied lines. Return no operations, a null intent, a null lower_price_strategy and empty intent_item_ids. Acceptance, rejection, thanks, payment, delivery or stock questions are always NO_MATCH: never treat "dale", "acepto", "lo compro" or "no me interesa" as a change.

ATOMIC operations:
- ADD_ITEM: add a material that is not already a line. It needs requested_description and quantity. target_item_id and promotion_id must be null.
- REMOVE_ITEM: remove one supplied line. It needs target_item_id and every other field must be null.
- REPLACE_ITEM: replace one supplied line with a different material. It needs target_item_id, requested_description and quantity.
- UPDATE_QUANTITY: change the quantity of one supplied line. It needs target_item_id and quantity. requested_description, unit and promotion_id must be null.
- ADD_DISCOUNT: the customer asks for a promotion by name or description and it is one of the supplied promotions. It needs promotion_id copied from the promotions; every other field must be null. Never invent a promotion, a percentage or an amount: a discount the customer asks for that is not a supplied promotion is NO_MATCH.

COMPLEX intents:
- LOWER_TOTAL_PRICE: the customer finds the quote expensive or asks for a cheaper option without naming the change ("está caro", "¿no tenés algo más barato?").
- IMPROVE_QUALITY: the customer asks for better or higher-end materials without naming them ("quiero algo de mejor calidad").
intent_item_ids lists the supplied lines the customer tied the goal to; leave it empty when the goal is about the whole quote.
lower_price_strategy is set only for LOWER_TOTAL_PRICE, and only when the customer says how they would rather pay less: ADAPT_TO_REACH_DISCOUNT when they would take more to earn a discount ("prefiero llevar más y que me hagas el descuento"), FIND_CHEAPER_ALTERNATIVES when they want cheaper brands or materials ("dame marcas más baratas"). Otherwise, and for every other level or intent, it is null.

Use an item ID only when it appears in the supplied quote lines, and a promotion ID only when it appears in the supplied promotions. Do not infer a product, brand, measure, quantity, price, discount, stock, delivery promise, or material that the customer did not state. Never lower a quantity unless the customer asked for that quantity. Do not turn an unclear request into a change. One operation must not target the same item or the same promotion as another operation.

For ADD_ITEM and REPLACE_ITEM, requested_description is the material only: retain its stated size or specification, but do not repeat the expressed quantity or its unit. For example, "Agregar 5 bolsas de cal hidratada 25 kg" becomes requested_description "cal hidratada 25 kg", quantity "5", and unit "bolsa".

The interpretation is one short sentence in Argentine Spanish for the seller. The proposal is only a draft for seller review: never calculate money and never address the customer.`

type changeRequestPayload struct {
	Context    []domain.ConversationLine       `json:"conversation_context"`
	Message    string                          `json:"customer_message"`
	Items      []domain.ChangeRequestItem      `json:"quote_items"`
	Promotions []domain.ChangeRequestPromotion `json:"promotions"`
}

type changeRequestAnswer struct {
	Level              string                  `json:"level"`
	Interpretation     string                  `json:"interpretation"`
	Intent             *string                 `json:"intent"`
	IntentItemIDs      []string                `json:"intent_item_ids"`
	LowerPriceStrategy *string                 `json:"lower_price_strategy"`
	Operations         []changeRequestAnswerOp `json:"operations"`
}

type changeRequestAnswerOp struct {
	Type                 string  `json:"type"`
	TargetItemID         *string `json:"target_item_id"`
	RequestedDescription *string `json:"requested_description"`
	Quantity             *string `json:"quantity"`
	Unit                 *string `json:"unit"`
	PromotionID          *string `json:"promotion_id"`
}

func mapChangeRequestOperation(answer changeRequestAnswerOp, index int) (domain.ChangeRequestOperation, error) {
	operation := domain.ChangeRequestOperation{
		Type:                 domain.ChangeRequestOperationType(answer.Type),
		RequestedDescription: trimmedText(answer.RequestedDescription),
		Unit:                 trimmedText(answer.Unit),
	}
	if answer.TargetItemID != nil {
		id, err := uuid.Parse(strings.TrimSpace(*answer.TargetItemID))
		if err != nil {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d].target_item_id is invalid",
				domain.ErrInvalidInput, index)
		}
		operation.TargetItemID = &id
	}
	if answer.PromotionID != nil {
		id, err := uuid.Parse(strings.TrimSpace(*answer.PromotionID))
		if err != nil {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d].promotion_id is invalid",
				domain.ErrInvalidInput, index)
		}
		operation.PromotionID = &id
	}
	if answer.Quantity != nil {
		quantity, err := decimal.NewFromString(strings.TrimSpace(*answer.Quantity))
		if err != nil {
			return domain.ChangeRequestOperation{}, fmt.Errorf("%w: operations[%d].quantity is invalid",
				domain.ErrInvalidInput, index)
		}
		operation.Quantity = &quantity
	}
	return operation, nil
}

func trimmedText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func changeRequestSchema() map[string]any {
	nullableText := func(description string) map[string]any {
		return map[string]any{"type": []string{"string", "null"}, "description": description}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{"level", "interpretation", "intent", "intent_item_ids",
			"lower_price_strategy", "operations"},
		"properties": map[string]any{
			"level": map[string]any{
				"type": "string",
				"enum": []string{
					string(domain.ChangeRequestLevelAtomic),
					string(domain.ChangeRequestLevelComplex),
					string(domain.ChangeRequestLevelNoMatch),
				},
			},
			"interpretation": map[string]any{
				"type":        "string",
				"description": "A short Argentine Spanish explanation for the seller.",
			},
			"intent": map[string]any{
				"anyOf": []any{
					map[string]any{"type": "string", "enum": []string{
						string(domain.ChangeRequestIntentLowerTotalPrice),
						string(domain.ChangeRequestIntentImproveQuality),
					}},
					map[string]any{"type": "null"},
				},
				"description": "The goal of a COMPLEX request, or null.",
			},
			"intent_item_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "IDs copied exactly from quote_items that a COMPLEX goal is about.",
			},
			"lower_price_strategy": map[string]any{
				"anyOf": []any{
					map[string]any{"type": "string", "enum": []string{
						string(domain.HandlerStrategyFindCheaperAlternatives),
						string(domain.HandlerStrategyAdaptToReachDiscount),
					}},
					map[string]any{"type": "null"},
				},
				"description": "How a LOWER_TOTAL_PRICE customer said they prefer to pay less, or null.",
			},
			"operations": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []string{
						"type", "target_item_id", "requested_description", "quantity", "unit",
						"promotion_id",
					},
					"properties": map[string]any{
						"type": map[string]any{
							"type": "string",
							"enum": []string{
								string(domain.ChangeRequestOperationAddItem),
								string(domain.ChangeRequestOperationRemoveItem),
								string(domain.ChangeRequestOperationReplaceItem),
								string(domain.ChangeRequestOperationUpdateQuantity),
								string(domain.ChangeRequestOperationAddDiscount),
							},
						},
						"target_item_id":        nullableText("An ID copied exactly from quote_items, or null."),
						"requested_description": nullableText("The customer's material wording, or null."),
						"quantity":              nullableText("A positive decimal as a string, or null."),
						"unit":                  nullableText("The unit the customer wrote, or null."),
						"promotion_id":          nullableText("An ID copied exactly from promotions, or null."),
					},
				},
			},
		},
	}
}
