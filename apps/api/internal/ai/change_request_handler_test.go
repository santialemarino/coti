package ai

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestChangeRequestHandler_Propose_MapsAnAddItem(t *testing.T) {
	generator := &fakeGenerator{answer: `{"level":"ATOMIC","interpretation":"El cliente pidió sumar cal.","intent":null,"intent_item_ids":[],"operations":[
		{"type":"ADD_ITEM","target_item_id":null,"requested_description":"cal hidratada 25 kg","quantity":"5","unit":"bolsa","promotion_id":null}
	]}`}
	handler := NewChangeRequestHandler(generator)

	proposal, err := handler.Propose(context.Background(), domain.ChangeRequestInput{
		Message: "Agregar 5 bolsas de cal hidratada 25 kg",
		Items:   []domain.ChangeRequestItem{{ID: uuid.New(), RequestedDescription: "cemento", Quantity: decimal.NewFromInt(10)}},
	})
	if err != nil {
		t.Fatalf("Propose returned %v", err)
	}
	if proposal.Level != domain.ChangeRequestLevelAtomic ||
		proposal.Interpretation != "El cliente pidió sumar cal." || len(proposal.Operations) != 1 {
		t.Fatalf("proposal = %+v, want one interpreted operation", proposal)
	}
	operation := proposal.Operations[0]
	if operation.Type != domain.ChangeRequestOperationAddItem || operation.TargetItemID != nil ||
		operation.RequestedDescription == nil || *operation.RequestedDescription != "cal hidratada 25 kg" ||
		operation.Quantity == nil || !operation.Quantity.Equal(decimal.NewFromInt(5)) ||
		operation.Unit == nil || *operation.Unit != "bolsa" {
		t.Errorf("operation = %+v, want the requested material", operation)
	}
	if generator.operation != domain.AIOperationChangeRequestHandling {
		t.Errorf("operation = %q, want CHANGE_REQUEST_HANDLING", generator.operation)
	}
}

func TestChangeRequestHandler_Propose_RefusesAnInvalidTargetID(t *testing.T) {
	generator := &fakeGenerator{answer: `{"level":"ATOMIC","interpretation":"Quiere quitar una línea.","intent":null,"intent_item_ids":[],"operations":[
		{"type":"REMOVE_ITEM","target_item_id":"not-an-id","requested_description":null,"quantity":null,"unit":null,"promotion_id":null}
	]}`}

	_, err := NewChangeRequestHandler(generator).Propose(context.Background(), domain.ChangeRequestInput{
		Message: "sacá el cemento",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Propose returned %v, want ErrInvalidInput", err)
	}
}

func TestChangeRequestHandler_Propose_MapsLevelsIntentsAndDiscounts(t *testing.T) {
	lineID, promotionID := uuid.New(), uuid.New()
	cases := []struct {
		name   string
		answer string
		check  func(t *testing.T, proposal *domain.ChangeRequestProposal)
	}{
		{"no match becomes the seller escape",
			`{"level":"NO_MATCH","interpretation":"Acepta.","intent":null,"intent_item_ids":[],"operations":[]}`,
			func(t *testing.T, proposal *domain.ChangeRequestProposal) {
				if proposal.Level != domain.ChangeRequestLevelNoMatch || len(proposal.Operations) != 1 ||
					proposal.Operations[0].Type != domain.ChangeRequestOperationRequiresSeller {
					t.Errorf("proposal = %+v, want one REQUIRES_SELLER under NO_MATCH", proposal)
				}
			}},
		{"complex carries its intent and lines",
			`{"level":"COMPLEX","interpretation":"Lo ve caro.","intent":"LOWER_TOTAL_PRICE","intent_item_ids":["` +
				lineID.String() + `"],"operations":[]}`,
			func(t *testing.T, proposal *domain.ChangeRequestProposal) {
				if proposal.Intent == nil || *proposal.Intent != domain.ChangeRequestIntentLowerTotalPrice ||
					len(proposal.IntentItemIDs) != 1 || proposal.IntentItemIDs[0] != lineID ||
					len(proposal.Operations) != 0 {
					t.Errorf("proposal = %+v, want the intent on the named line and no operations", proposal)
				}
				if proposal.LowerPriceStrategy != nil {
					t.Errorf("strategy = %v, want none when the answer omits it", *proposal.LowerPriceStrategy)
				}
			}},
		{"a lower price carries the customer's preferred strategy",
			`{"level":"COMPLEX","interpretation":"Prefiere llevar más.","intent":"LOWER_TOTAL_PRICE","intent_item_ids":[],"lower_price_strategy":"ADAPT_TO_REACH_DISCOUNT","operations":[]}`,
			func(t *testing.T, proposal *domain.ChangeRequestProposal) {
				if proposal.LowerPriceStrategy == nil ||
					*proposal.LowerPriceStrategy != domain.HandlerStrategyAdaptToReachDiscount {
					t.Errorf("strategy = %v, want ADAPT_TO_REACH_DISCOUNT", proposal.LowerPriceStrategy)
				}
			}},
		{"a discount names its promotion",
			`{"level":"ATOMIC","interpretation":"Pide la promo.","intent":null,"intent_item_ids":[],"operations":[
				{"type":"ADD_DISCOUNT","target_item_id":null,"requested_description":null,"quantity":null,"unit":null,"promotion_id":"` +
				promotionID.String() + `"}]}`,
			func(t *testing.T, proposal *domain.ChangeRequestProposal) {
				if len(proposal.Operations) != 1 || proposal.Operations[0].PromotionID == nil ||
					*proposal.Operations[0].PromotionID != promotionID {
					t.Errorf("proposal = %+v, want the promotion id", proposal)
				}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proposal, err := NewChangeRequestHandler(&fakeGenerator{answer: tc.answer}).Propose(
				context.Background(), domain.ChangeRequestInput{Message: "x"})
			if err != nil {
				t.Fatalf("Propose returned %v", err)
			}
			tc.check(t, proposal)
		})
	}
}

func TestChangeRequestHandler_Propose_RefusesMalformedIDs(t *testing.T) {
	for name, answer := range map[string]string{
		"promotion": `{"level":"ATOMIC","interpretation":"x","intent":null,"intent_item_ids":[],"operations":[
			{"type":"ADD_DISCOUNT","target_item_id":null,"requested_description":null,"quantity":null,"unit":null,"promotion_id":"10%"}]}`,
		"intent line": `{"level":"COMPLEX","interpretation":"x","intent":"IMPROVE_QUALITY","intent_item_ids":["cemento"],"operations":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewChangeRequestHandler(&fakeGenerator{answer: answer}).Propose(context.Background(),
				domain.ChangeRequestInput{Message: "x"})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("Propose returned %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestChangeRequestSchema_UsesTheClosedCatalogs(t *testing.T) {
	properties := changeRequestSchema()["properties"].(map[string]any)
	levels := properties["level"].(map[string]any)["enum"].([]string)
	if want := []string{"ATOMIC", "COMPLEX", "NO_MATCH"}; !reflect.DeepEqual(levels, want) {
		t.Errorf("level enum = %v, want %v with the safe escape", levels, want)
	}
	intent := properties["intent"].(map[string]any)["anyOf"].([]any)
	intents := intent[0].(map[string]any)["enum"].([]string)
	if want := []string{"LOWER_TOTAL_PRICE", "IMPROVE_QUALITY"}; !reflect.DeepEqual(intents, want) ||
		intent[1].(map[string]any)["type"] != "null" {
		t.Errorf("intent = %v, want %v or null", intent, want)
	}
	strategy := properties["lower_price_strategy"].(map[string]any)["anyOf"].([]any)
	strategies := strategy[0].(map[string]any)["enum"].([]string)
	if want := []string{"FIND_CHEAPER_ALTERNATIVES", "ADAPT_TO_REACH_DISCOUNT"}; !reflect.DeepEqual(strategies, want) ||
		strategy[1].(map[string]any)["type"] != "null" {
		t.Errorf("lower_price_strategy = %v, want %v or null", strategy, want)
	}
	if required := changeRequestSchema()["required"].([]string); !slices.Contains(required, "lower_price_strategy") {
		t.Errorf("required = %v, want lower_price_strategy: a strict schema lists every key", required)
	}
	operation := properties["operations"].(map[string]any)["items"].(map[string]any)
	got := operation["properties"].(map[string]any)["type"].(map[string]any)["enum"].([]string)
	want := []string{
		string(domain.ChangeRequestOperationAddItem),
		string(domain.ChangeRequestOperationRemoveItem),
		string(domain.ChangeRequestOperationReplaceItem),
		string(domain.ChangeRequestOperationUpdateQuantity),
		string(domain.ChangeRequestOperationAddDiscount),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("operation enum = %v, want %v", got, want)
	}
	if slices.Contains(got, string(domain.ChangeRequestOperationRequiresSeller)) {
		t.Error("the escape is the NO_MATCH level, not an operation the model can mix with changes")
	}
}

// The instructions are the cached prefix: every request sends the same text, and the batch, its
// context and the draft's lines ride only in the input.
func TestChangeRequestHandler_Propose_KeepsTheVariablePartOutOfTheInstructions(t *testing.T) {
	answer := `{"level":"NO_MATCH","interpretation":"Pide el visto bueno.","intent":null,"intent_item_ids":[],"operations":[]}`
	first, second := &fakeGenerator{answer: answer}, &fakeGenerator{answer: answer}
	itemID, promotionID := uuid.New(), uuid.New()
	if _, err := NewChangeRequestHandler(first).Propose(context.Background(), domain.ChangeRequestInput{
		Message:    "dale, acepto",
		Context:    []domain.ConversationLine{{Author: domain.MessageAuthorSeller, Body: "¿Te sirve así?"}},
		Items:      []domain.ChangeRequestItem{{ID: itemID, RequestedDescription: "cemento", Quantity: decimal.NewFromInt(2)}},
		Promotions: []domain.ChangeRequestPromotion{{ID: promotionID, Name: "Cemento 10%"}},
	}); err != nil {
		t.Fatalf("Propose returned %v", err)
	}
	if _, err := NewChangeRequestHandler(second).Propose(context.Background(), domain.ChangeRequestInput{
		Message: "sacá la arena"}); err != nil {
		t.Fatalf("Propose returned %v", err)
	}
	if first.req.Instructions != second.req.Instructions {
		t.Error("the instructions differ between requests, so their prefix cannot cache")
	}
	for _, variable := range []string{"dale, acepto", "¿Te sirve así?", itemID.String(), promotionID.String()} {
		if strings.Contains(first.req.Instructions, variable) {
			t.Errorf("instructions carry %q, want it only in the input", variable)
		}
	}
	if len(first.req.Input) != 1 {
		t.Fatalf("input = %d parts, want one", len(first.req.Input))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(first.req.Input[0].Text), &payload); err != nil {
		t.Fatalf("input is not JSON: %v", err)
	}
	history, _ := payload["conversation_context"].([]any)
	promotions, _ := payload["promotions"].([]any)
	if payload["customer_message"] != "dale, acepto" || len(history) != 1 || len(promotions) != 1 {
		t.Errorf("payload = %v, want the batch, one context message and the promotion in force", payload)
	}
}
