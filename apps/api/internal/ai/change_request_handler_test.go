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
	generator := &fakeGenerator{answer: `{"interpretation":"El cliente pidió sumar cal.","operations":[
		{"type":"ADD_ITEM","target_item_id":null,"requested_description":"cal hidratada 25 kg","quantity":"5","unit":"bolsa"}
	]}`}
	handler := NewChangeRequestHandler(generator)

	proposal, err := handler.Propose(context.Background(), domain.ChangeRequestInput{
		Message: "Agregar 5 bolsas de cal hidratada 25 kg",
		Items:   []domain.ChangeRequestItem{{ID: uuid.New(), RequestedDescription: "cemento", Quantity: decimal.NewFromInt(10)}},
	})
	if err != nil {
		t.Fatalf("Propose returned %v", err)
	}
	if proposal.Interpretation != "El cliente pidió sumar cal." || len(proposal.Operations) != 1 {
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
	generator := &fakeGenerator{answer: `{"interpretation":"Quiere quitar una línea.","operations":[
		{"type":"REMOVE_ITEM","target_item_id":"not-an-id","requested_description":null,"quantity":null,"unit":null}
	]}`}

	_, err := NewChangeRequestHandler(generator).Propose(context.Background(), domain.ChangeRequestInput{
		Message: "sacá el cemento",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Propose returned %v, want ErrInvalidInput", err)
	}
}

func TestChangeRequestSchema_UsesTheClosedOperationCatalog(t *testing.T) {
	schema := changeRequestSchema()
	properties := schema["properties"].(map[string]any)
	operation := properties["operations"].(map[string]any)["items"].(map[string]any)
	typeSchema := operation["properties"].(map[string]any)["type"].(map[string]any)
	got := typeSchema["enum"].([]string)
	want := []string{
		string(domain.ChangeRequestOperationAddItem),
		string(domain.ChangeRequestOperationRemoveItem),
		string(domain.ChangeRequestOperationReplaceItem),
		string(domain.ChangeRequestOperationUpdateQuantity),
		string(domain.ChangeRequestOperationRequiresSeller),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("operation enum = %v, want %v", got, want)
	}
	if !slices.Contains(got, string(domain.ChangeRequestOperationRequiresSeller)) {
		t.Error("the schema has no safe escape for a seller-only request")
	}
}

// The instructions are the cached prefix: every request sends the same text, and the batch, its
// context and the draft's lines ride only in the input.
func TestChangeRequestHandler_Propose_KeepsTheVariablePartOutOfTheInstructions(t *testing.T) {
	answer := `{"interpretation":"Pide el visto bueno.","operations":[
		{"type":"REQUIRES_SELLER","target_item_id":null,"requested_description":null,"quantity":null,"unit":null}
	]}`
	first, second := &fakeGenerator{answer: answer}, &fakeGenerator{answer: answer}
	itemID := uuid.New()
	if _, err := NewChangeRequestHandler(first).Propose(context.Background(), domain.ChangeRequestInput{
		Message: "dale, acepto",
		Context: []domain.ConversationLine{{Author: domain.MessageAuthorSeller, Body: "¿Te sirve así?"}},
		Items:   []domain.ChangeRequestItem{{ID: itemID, RequestedDescription: "cemento", Quantity: decimal.NewFromInt(2)}},
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
	for _, variable := range []string{"dale, acepto", "¿Te sirve así?", itemID.String()} {
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
	if payload["customer_message"] != "dale, acepto" || len(history) != 1 {
		t.Errorf("payload = %v, want the batch and one context message", payload)
	}
}
