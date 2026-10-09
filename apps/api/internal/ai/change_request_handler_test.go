package ai

import (
	"context"
	"errors"
	"reflect"
	"slices"
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
