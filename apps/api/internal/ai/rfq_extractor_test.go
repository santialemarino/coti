package ai

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// fakeGenerator answers with a staged payload, decoded the way the real adapter decodes: unknown
// fields are refused, so a schema that drifts from the struct behind it fails here too.
type fakeGenerator struct {
	answer string
	err    error
	calls  int
	req    domain.GenerationRequest
}

func (f *fakeGenerator) Generate(
	_ context.Context, req domain.GenerationRequest, out any,
) (*domain.GenerationUsage, error) {
	f.calls++
	f.req = req
	if f.err != nil {
		return nil, f.err
	}
	decoder := json.NewDecoder(strings.NewReader(f.answer))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return nil, err
	}
	return &domain.GenerationUsage{
		Provider: "test-provider", Model: "test-model", InputTokens: 12, OutputTokens: 7,
	}, nil
}

func TestRFQExtractor_Extract_MapsEverySource(t *testing.T) {
	generator := &fakeGenerator{answer: `{"items":[
		{"requested_description":"10 bolsas de cemento","quantity":"10","unit":"bolsa",
		 "quantity_source":"EXPLICIT","quantity_rationale":"el cliente pidió 10 bolsas"},
		{"requested_description":"3 pallets de ladrillos","quantity":"900","unit":"unidad",
		 "quantity_source":"DERIVED","quantity_rationale":"3 pallets de 300 ladrillos"},
		{"requested_description":"arena","quantity":"0","unit":null,
		 "quantity_source":"UNRESOLVED","quantity_rationale":"no indicó cuánta arena"}
	]}`}
	extractor := NewRFQExtractor(generator, 50)

	extraction, err := extractor.Extract(context.Background(), "10 bolsas de cemento, 3 pallets, arena")
	if err != nil {
		t.Fatalf("Extract returned %v", err)
	}
	lines := extraction.Lines
	if len(lines) != 3 {
		t.Fatalf("read %d lines, want 3", len(lines))
	}
	// One call for the whole order: a call per line would pay for the instructions each time.
	if generator.calls != 1 {
		t.Errorf("generator called %d times, want once for the whole order", generator.calls)
	}
	if extraction.Usage.Provider != "test-provider" || extraction.Usage.Model != "test-model" {
		t.Errorf("generation identity = %s/%s, want test-provider/test-model",
			extraction.Usage.Provider, extraction.Usage.Model)
	}
	if extraction.PromptVersion != rfqExtractionPromptVersion ||
		extraction.SchemaVersion != rfqExtractionSchemaVersion {
		t.Errorf("extraction versions = %s/%s, want %s/%s", extraction.PromptVersion,
			extraction.SchemaVersion, rfqExtractionPromptVersion, rfqExtractionSchemaVersion)
	}

	if lines[0].Source != domain.QuantitySourceExplicit {
		t.Errorf("line 0 source %q, want EXPLICIT", lines[0].Source)
	}
	if !lines[0].Quantity.Equal(decimal.RequireFromString("10")) {
		t.Errorf("line 0 quantity %s, want 10", lines[0].Quantity)
	}
	if lines[0].RequestedDescription != "10 bolsas de cemento" {
		t.Errorf("line 0 description %q, want the client's words",
			lines[0].RequestedDescription)
	}
	if lines[0].Unit == nil || *lines[0].Unit != "bolsa" {
		t.Errorf("line 0 unit %v, want %q", lines[0].Unit, "bolsa")
	}

	if lines[1].Source != domain.QuantitySourceDerived {
		t.Errorf("line 1 source %q, want DERIVED", lines[1].Source)
	}
	if !lines[1].Quantity.Equal(decimal.RequireFromString("900")) {
		t.Errorf("line 1 quantity %s, want 900", lines[1].Quantity)
	}
	if lines[1].QuantityRationale != "3 pallets de 300 ladrillos" {
		t.Errorf("line 1 rationale %q, want the derivation", lines[1].QuantityRationale)
	}

	// The escape value: a material with no defensible quantity is a valid answer, and it keeps
	// its place in the order rather than being dropped.
	if lines[2].Source != domain.QuantitySourceUnresolved {
		t.Errorf("line 2 source %q, want UNRESOLVED", lines[2].Source)
	}
	if !lines[2].Quantity.IsZero() {
		t.Errorf("line 2 quantity %s, want zero", lines[2].Quantity)
	}
	if lines[2].Unit != nil {
		t.Errorf("line 2 unit %v, want none", lines[2].Unit)
	}
}

func TestRFQExtractor_Extract_ZeroesAQuantitySentOnAnUnresolvedLine(t *testing.T) {
	// The model was told to send "0" here. When it sends a number anyway, the number is the one
	// thing this layer exists to keep out of a quote.
	generator := &fakeGenerator{answer: `{"items":[
		{"requested_description":"cemento","quantity":"25","unit":"bolsa",
		 "quantity_source":"UNRESOLVED","quantity_rationale":"no dijo cuántas bolsas"}
	]}`}
	extractor := NewRFQExtractor(generator, 50)

	extraction, err := extractor.Extract(context.Background(), "necesito cemento")
	if err != nil {
		t.Fatalf("Extract returned %v", err)
	}
	lines := extraction.Lines
	if !lines[0].Quantity.IsZero() {
		t.Errorf("quantity %s survived an unresolved line, want zero", lines[0].Quantity)
	}
}

func TestRFQExtractor_Extract_RejectsAQuantityThatIsNotADecimal(t *testing.T) {
	generator := &fakeGenerator{answer: `{"items":[
		{"requested_description":"cemento","quantity":"unas cuantas","unit":"bolsa",
		 "quantity_source":"EXPLICIT","quantity_rationale":"pidió unas cuantas"}
	]}`}
	extractor := NewRFQExtractor(generator, 50)

	_, err := extractor.Extract(context.Background(), "cemento")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Extract returned %v, want ErrInvalidInput", err)
	}
	if errors.Is(err, domain.ErrAIUnavailable) {
		t.Error("an unreadable number was reported as a provider outage")
	}
}

func TestRFQExtractor_Extract_PassesTheProviderFailureThrough(t *testing.T) {
	generator := &fakeGenerator{err: Unavailable(errors.New("no language model provider"))}
	extractor := NewRFQExtractor(generator, 50)

	_, err := extractor.Extract(context.Background(), "cemento")
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Extract returned %v, want ErrAIUnavailable", err)
	}
}

func TestRFQExtractor_Extract_SendsTheOrderAsTheVariableHalf(t *testing.T) {
	generator := &fakeGenerator{answer: `{"items":[]}`}
	extractor := NewRFQExtractor(generator, 42)

	if _, err := extractor.Extract(context.Background(), "10 bolsas de cemento"); err != nil {
		t.Fatalf("Extract returned %v", err)
	}
	req := generator.req
	if err := req.Validate(); err != nil {
		t.Fatalf("the request the extractor built is invalid: %v", err)
	}
	if len(req.Input) != 1 || req.Input[0].Kind != domain.ContentKindText ||
		req.Input[0].Text != "10 bolsas de cemento" {
		t.Errorf("request input %+v, want the order as one text block", req.Input)
	}
	// The order rides Input, never Instructions: a prompt that changes per request caches nothing.
	if strings.Contains(req.Instructions, "10 bolsas de cemento") {
		t.Error("the order was built into the instructions, which breaks the cached prefix")
	}
	// The cap the service enforces is the number the model is asked to aim under.
	if !strings.Contains(req.Instructions, "at most 42") {
		t.Errorf("instructions do not state the 42-item cap:\n%s", req.Instructions)
	}
}

func TestRFQExtractionSchema_ForcesTheClosedSourceEnum(t *testing.T) {
	schema := rfqExtractionSchema()
	item := itemSchema(t, schema)

	properties, ok := item["properties"].(map[string]any)
	if !ok {
		t.Fatal("the item schema declares no properties")
	}
	source, ok := properties["quantity_source"].(map[string]any)
	if !ok {
		t.Fatal("the item schema declares no quantity_source")
	}
	got, ok := source["enum"].([]string)
	if !ok {
		t.Fatalf("quantity_source enum is %T, want a closed list", source["enum"])
	}
	want := []string{"EXPLICIT", "DERIVED", "UNRESOLVED"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("quantity_source enum %v, want %v", got, want)
	}
	// Without the escape value in the enum the model has no valid way to say it cannot tell, so
	// it invents a number instead.
	if !slices.Contains(got, string(domain.QuantitySourceUnresolved)) {
		t.Error("the enum has no escape value; an unresolvable quantity would have to be invented")
	}
}

func TestRFQExtractionSchema_MatchesTheStructItDecodesInto(t *testing.T) {
	item := itemSchema(t, rfqExtractionSchema())
	properties, ok := item["properties"].(map[string]any)
	if !ok {
		t.Fatal("the item schema declares no properties")
	}

	declared := make([]string, 0, len(properties))
	for name := range properties {
		declared = append(declared, name)
	}
	fields := make([]string, 0, len(declared))
	itemType := reflect.TypeOf(rfqExtractedItem{})
	for i := range itemType.NumField() {
		fields = append(fields, itemType.Field(i).Tag.Get("json"))
	}
	sort.Strings(declared)
	sort.Strings(fields)
	// The decoder refuses unknown fields, so a property with no field behind it turns every
	// answer into a retry that can never succeed.
	if !reflect.DeepEqual(declared, fields) {
		t.Errorf("schema declares %v, the struct carries %v", declared, fields)
	}

	required, ok := item["required"].([]string)
	if !ok {
		t.Fatalf("the item schema's required list is %T", item["required"])
	}
	sort.Strings(required)
	if !reflect.DeepEqual(required, declared) {
		t.Errorf("required %v, want every property: an omitted one decodes as a silent zero value",
			required)
	}
}

func TestRFQExtractionSchema_CarriesNoKeywordStructuredOutputsIgnores(t *testing.T) {
	// minLength, maxLength, minItems and maxItems are not enforced by structured outputs. Stating
	// one here would read as a guarantee, and the service is what actually checks the lengths.
	ignored := []string{"minLength", "maxLength", "minItems", "maxItems", "minimum", "maximum"}
	encoded, err := json.Marshal(rfqExtractionSchema())
	if err != nil {
		t.Fatalf("the schema does not encode: %v", err)
	}
	for _, keyword := range ignored {
		if strings.Contains(string(encoded), `"`+keyword+`"`) {
			t.Errorf("the schema states %q, which the provider does not enforce", keyword)
		}
	}
	if !strings.Contains(string(encoded), `"additionalProperties":false`) {
		t.Error("the schema allows extra properties, which strict decoding then refuses")
	}
}

// itemSchema digs out the per-item object, so a test asserts on one level rather than on a chain
// of type assertions.
func itemSchema(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("the schema declares no properties")
	}
	items, ok := properties["items"].(map[string]any)
	if !ok {
		t.Fatal("the schema declares no items array")
	}
	item, ok := items["items"].(map[string]any)
	if !ok {
		t.Fatal("the items array declares no element schema")
	}
	return item
}
