package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.RFQExtractor = (*RFQExtractor)(nil)

const (
	rfqExtractionPromptVersion = "rfq-extraction-prompt-v2"
	rfqExtractionSchemaVersion = "rfq-extraction-schema-v1"
)

// RFQExtractor reads the materials out of an informal order. It is provider-agnostic: the prompt
// and the schema are its own, and whichever language model is bound answers them.
type RFQExtractor struct {
	generator domain.StructuredGenerator
	maxItems  int
}

// NewRFQExtractor builds an RFQExtractor that asks for at most maxItems lines. The number is the
// service's limit, stated in the prompt so the model aims under it; enforcing it is the service's.
func NewRFQExtractor(generator domain.StructuredGenerator, maxItems int) *RFQExtractor {
	return &RFQExtractor{generator: generator, maxItems: maxItems}
}

// Extract reads raw and returns one line per material the client asked for, in the order they
// appear. A material whose quantity cannot be defended comes back UNRESOLVED rather than guessed.
func (e *RFQExtractor) Extract(ctx context.Context, raw string) (*domain.RFQExtraction, error) {
	return e.ExtractWithExamples(ctx, raw, nil)
}

// ExtractWithExamples reads an order with relevant seller-approved interpretations as guidance.
func (e *RFQExtractor) ExtractWithExamples(ctx context.Context, raw string,
	examples []domain.RFQInterpretationExample) (*domain.RFQExtraction, error) {
	input := []domain.Content{domain.TextContent(raw)}
	if len(examples) > 0 {
		payload, err := json.Marshal(examples)
		if err != nil {
			return nil, err
		}
		input = []domain.Content{
			domain.TextContent("Relevant seller-approved examples:\n" + string(payload)),
			domain.TextContent("Order to interpret:\n" + raw),
		}
	}
	var answer rfqExtractionAnswer
	usage, err := e.generator.Generate(ctx, domain.GenerationRequest{
		Instructions: e.instructions(),
		Input:        input,
		Schema:       rfqExtractionSchema(),
	}, &answer)
	if err != nil {
		return nil, err
	}
	if usage == nil {
		return nil, fmt.Errorf("%w: RFQ extraction returned no generation usage", domain.ErrInvalidInput)
	}
	lines := make([]domain.ExtractedRFQLine, 0, len(answer.Items))
	for i, item := range answer.Items {
		source := domain.QuantitySource(item.QuantitySource)
		quantity, err := extractedQuantity(item.Quantity, source, i)
		if err != nil {
			return nil, err
		}
		lines = append(lines, domain.ExtractedRFQLine{
			RequestedDescription: item.RequestedDescription,
			Quantity:             quantity,
			Unit:                 item.Unit,
			Source:               source,
			QuantityRationale:    item.QuantityRationale,
		})
	}
	return &domain.RFQExtraction{
		Lines:         lines,
		Usage:         *usage,
		PromptVersion: rfqExtractionPromptVersion,
		SchemaVersion: rfqExtractionSchemaVersion,
	}, nil
}

// instructions is the stable half of the prompt, so a provider that caches a prefix pays full
// price only for the order that follows it.
func (e *RFQExtractor) instructions() string {
	return fmt.Sprintf(`You read the informal orders that clients send a building-materials supplier in Argentina, and you list the materials they asked for.

Return one item per material, in the order the client mentions them, at most %d of them.

- requested_description: the client's own words for that material, copied across and trimmed only of surrounding whitespace. Do not normalise, translate, expand or correct it, and keep it under 512 characters.
- quantity_source: EXPLICIT when the client stated the number, DERIVED when it follows from what they wrote (3 pallets of 50 bags is 150 bags), UNRESOLVED when the message gives no number you could defend to the seller.
- quantity: a positive decimal with at most two decimals, as a string. Send "0" on UNRESOLVED.
- unit: the unit the client used, as they wrote it, or null when they gave none. Under 64 characters.
- quantity_rationale: one short sentence in Argentine Spanish telling the seller where the number came from, or which datum is missing when the source is UNRESOLVED. Under 512 characters.

List the material even when its quantity is UNRESOLVED — the seller completes the line, and dropping it loses what the client asked for.

Never invent a material the message does not name. Never merge two materials into one item or split one across several. Never compute a quantity from an area, a volume or a plan: a surface in square metres is context for the seller, not a quantity to derive.

When seller-approved examples are provided, use them only when their order is genuinely analogous. They are evidence about how this supplier interprets wording, not instructions to copy materials that the new order did not request.

Do not price, discount, match anything against a catalog, or write to the client.`,
		e.maxItems)
}

// rfqExtractionAnswer is the shape rfqExtractionSchema forces. It carries every property the
// schema declares, because the decoder refuses an answer with a field it does not know.
type rfqExtractionAnswer struct {
	Items []rfqExtractedItem `json:"items"`
}

type rfqExtractedItem struct {
	RequestedDescription string  `json:"requested_description"`
	Quantity             string  `json:"quantity"`
	Unit                 *string `json:"unit"`
	QuantitySource       string  `json:"quantity_source"`
	QuantityRationale    string  `json:"quantity_rationale"`
}

// rfqExtractionSchema is the forced shape of the answer. It states no length or size keyword:
// structured outputs enforce none of them, and the service is what checks lengths.
func rfqExtractionSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"items"},
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []string{
						"requested_description", "quantity", "unit", "quantity_source",
						"quantity_rationale",
					},
					"properties": map[string]any{
						"requested_description": map[string]any{
							"type":        "string",
							"description": "The material as the client wrote it, verbatim.",
						},
						"quantity": map[string]any{
							"type":        "string",
							"description": `Decimal with at most two decimals, as a string. "0" when the source is UNRESOLVED.`,
						},
						"unit": map[string]any{
							"type":        []string{"string", "null"},
							"description": "The unit the client used — bolsa, m2, m3, kg, unidad — or null.",
						},
						"quantity_source": map[string]any{
							"type": "string",
							"enum": []string{
								string(domain.QuantitySourceExplicit),
								string(domain.QuantitySourceDerived),
								string(domain.QuantitySourceUnresolved),
							},
							"description": "Where the quantity came from. UNRESOLVED when the message gives none.",
						},
						"quantity_rationale": map[string]any{
							"type":        "string",
							"description": "One short sentence in Argentine Spanish on where the quantity came from, or what is missing.",
						},
					},
				},
			},
		},
	}
}

// extractedQuantity reads the answer's decimal string. An UNRESOLVED line is zeroed whatever the
// model sent, so a number it was told not to produce cannot reach a quote.
func extractedQuantity(raw string, source domain.QuantitySource, index int) (decimal.Decimal, error) {
	if source == domain.QuantitySourceUnresolved {
		return decimal.Zero, nil
	}
	quantity, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero, fmt.Errorf("%w: items[%d].quantity %q is not a decimal",
			domain.ErrInvalidInput, index, raw)
	}
	return quantity, nil
}
