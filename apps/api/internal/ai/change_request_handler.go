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

// Propose returns typed changes without mutating a quote or calculating any monetary value.
func (h *ChangeRequestHandler) Propose(ctx context.Context,
	in domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	payload, err := json.Marshal(changeRequestPayload{Message: in.Message, Items: in.Items})
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

	proposal := &domain.ChangeRequestProposal{Interpretation: answer.Interpretation,
		Operations: make([]domain.ChangeRequestOperation, 0, len(answer.Operations))}
	for index, operation := range answer.Operations {
		mapped, mapErr := mapChangeRequestOperation(operation, index)
		if mapErr != nil {
			return nil, mapErr
		}
		proposal.Operations = append(proposal.Operations, mapped)
	}
	return proposal, nil
}

const changeRequestInstructions = `You interpret a customer request to revise a building-materials quote for an Argentine supplier. The current quote lines are supplied with opaque IDs. Return only the atomic changes that are explicit in the customer's message.

Choose one of these operations:
- ADD_ITEM: add a material that is not already a line. It needs requested_description and quantity. target_item_id must be null.
- REMOVE_ITEM: remove one supplied line. It needs target_item_id and every material field must be null.
- REPLACE_ITEM: replace one supplied line with a different material. It needs target_item_id, requested_description and quantity.
- UPDATE_QUANTITY: change the quantity of one supplied line. It needs target_item_id and quantity. requested_description and unit must be null.
- REQUIRES_SELLER: the request is ambiguous, unsupported, asks about something other than materials, or cannot be tied to the supplied lines. Every other field must be null.

Use an item ID only when it appears in the supplied quote lines. Do not infer a product, brand, measure, quantity, price, discount, stock, delivery promise, or material that the customer did not state. Do not turn an unclear request into a change. One operation must not target the same item as another operation. A request that needs a seller may contain exactly one REQUIRES_SELLER operation and no other operation.

For ADD_ITEM and REPLACE_ITEM, requested_description is the material only: retain its stated size or specification, but do not repeat the expressed quantity or its unit. For example, "Agregar 5 bolsas de cal hidratada 25 kg" becomes requested_description "cal hidratada 25 kg", quantity "5", and unit "bolsa".

The interpretation is one short sentence in Argentine Spanish for the seller. The proposal is only a draft for seller review: never calculate money and never address the customer.`

type changeRequestPayload struct {
	Message string                     `json:"customer_message"`
	Items   []domain.ChangeRequestItem `json:"quote_items"`
}

type changeRequestAnswer struct {
	Interpretation string                  `json:"interpretation"`
	Operations     []changeRequestAnswerOp `json:"operations"`
}

type changeRequestAnswerOp struct {
	Type                 string  `json:"type"`
	TargetItemID         *string `json:"target_item_id"`
	RequestedDescription *string `json:"requested_description"`
	Quantity             *string `json:"quantity"`
	Unit                 *string `json:"unit"`
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
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"interpretation", "operations"},
		"properties": map[string]any{
			"interpretation": map[string]any{
				"type":        "string",
				"description": "A short Argentine Spanish explanation for the seller.",
			},
			"operations": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []string{
						"type", "target_item_id", "requested_description", "quantity", "unit",
					},
					"properties": map[string]any{
						"type": map[string]any{
							"type": "string",
							"enum": []string{
								string(domain.ChangeRequestOperationAddItem),
								string(domain.ChangeRequestOperationRemoveItem),
								string(domain.ChangeRequestOperationReplaceItem),
								string(domain.ChangeRequestOperationUpdateQuantity),
								string(domain.ChangeRequestOperationRequiresSeller),
							},
						},
						"target_item_id": map[string]any{
							"type":        []string{"string", "null"},
							"description": "An ID copied exactly from quote_items, or null.",
						},
						"requested_description": map[string]any{
							"type":        []string{"string", "null"},
							"description": "The customer's material wording, or null.",
						},
						"quantity": map[string]any{
							"type":        []string{"string", "null"},
							"description": "A positive decimal as a string, or null.",
						},
						"unit": map[string]any{
							"type":        []string{"string", "null"},
							"description": "The unit the customer wrote, or null.",
						},
					},
				},
			},
		},
	}
}
