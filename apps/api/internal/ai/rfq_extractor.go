package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var (
	_ domain.RFQExtractor        = (*RFQExtractor)(nil)
	_ domain.RFQContentExtractor = (*RFQExtractor)(nil)
)

const (
	rfqExtractionPromptVersion = "rfq-extraction-prompt-v7"
	rfqExtractionSchemaVersion = "rfq-extraction-schema-v3"
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
	return e.extract(ctx, input)
}

// ExtractFromContent reads an order the client sent as a file. The blocks are the order itself,
// so the caller assembles them and the prompt is the same one plain text answers.
func (e *RFQExtractor) ExtractFromContent(ctx context.Context, blocks []domain.Content,
	examples []domain.RFQInterpretationExample) (*domain.RFQExtraction, error) {
	input := blocks
	if len(examples) > 0 {
		payload, err := json.Marshal(examples)
		if err != nil {
			return nil, err
		}
		input = append([]domain.Content{
			domain.TextContent("Relevant seller-approved examples:\n" + string(payload)),
		}, blocks...)
	}
	return e.extract(ctx, input)
}

func (e *RFQExtractor) extract(ctx context.Context,
	input []domain.Content) (*domain.RFQExtraction, error) {
	var answer rfqExtractionAnswer
	ctx = domain.WithAIOperation(ctx, domain.AIOperationRFQExtraction)
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
			SearchDescription:    item.SearchDescription,
			SelectionPreference:  domain.RFQSelectionPreference(item.SelectionPreference),
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

- requested_description: the client's own words for that material, copied across and trimmed only of surrounding whitespace. Do not normalise, translate, expand or correct it, and keep it under 512 characters. The one addition: when the client names a material once and lists more of it by size alone ("20 caños de 110 y 10 de 63"), carry the material's name into each of those items ("caños de 63"), because an item has to say what it is.
- search_description: the material to search for, under 512 characters, without the ordered quantity, conversational filler or selection preferences. Preserve every explicit brand, material type, size, presentation, colour and other product constraint. Correct obvious spelling slips such as "semento" to "cemento" without changing identity. Never add an unspecified attribute or replace a requested brand with a substitute.
- selection_preference: LOWEST_PRICE only when the client explicitly requests the lowest price for this item (such as "más barato" or "de la más económica"); HIGHEST_PRICE only for the highest price (such as "del más caro"); otherwise NONE. A budget, a discount request, "barato" without a lowest-price comparison, and a request for better quality are NONE. Highest price does not imply higher quality. Apply a preference only to the items its wording qualifies. A shared instruction such as "ambos de lo más económico" applies LOWEST_PRICE to both preceding materials. "Al precio más bajo" means LOWEST_PRICE; "más costoso" and "al precio más alto" mean HIGHEST_PRICE. Separate it from product identity: "5 de cemento más barato que tengas" means search_description "cemento", quantity "5", selection_preference LOWEST_PRICE; "cemento Loma Negra de 50 kg más barato" retains "cemento Loma Negra de 50 kg". "10 mt de arena de la más económica y 10 bolsas de cemento del más caro" assigns LOWEST_PRICE to arena and HIGHEST_PRICE to cemento; keep "mt" as the arena unit, without converting it to m3. The backend compares current prices; never infer which brand or product is cheaper or more expensive.
- quantity_source: EXPLICIT when the client stated the number, DERIVED when it follows from what they wrote (3 pallets of 50 bags is 150 bags), UNRESOLVED when the message gives no number you could defend to the seller.
- quantity: a positive decimal with at most two decimals, as a string. Send "0" on UNRESOLVED.
- unit: the unit the client used, as they wrote it, or null when they gave none. Under 64 characters.
- quantity_rationale: one short sentence in Argentine Spanish telling the seller where the number came from, or which datum is missing when the source is UNRESOLVED. Under 512 characters.

Clients write the way they talk at the counter. A container or its diminutive is the unit, never the material: "una bolsita de cal" is quantity "1", unit "bolsita", search_description "cal". "Medio" and "media" are half: "medio metro de arena" is quantity "0.5", unit "metro", search_description "arena". "Un par de" is two, DERIVED. A number standing for an item names the material around it: "una de cal" is quantity "1" of cal with no unit, and "hierro del 8" is an 8 mm iron, search_description "hierro 8 mm". A purpose is context, not identity: "arena para revocar" searches "arena". A brand clients use as the everyday name of a material ("durlock", "klaukol", "tacurú") is kept as written in search_description; never swap it for the generic name or another brand.

List the material even when its quantity is UNRESOLVED — the seller completes the line, and dropping it loses what the client asked for.

Never invent a material the message does not name. Never merge two materials into one item or split one across several: an accessory the client asks for alongside another material ("una bacha con su grifería") is an item of its own. Never compute a quantity from an area, a volume or a plan: a surface in square metres is context for the seller, not a quantity to derive.

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
	SearchDescription    string  `json:"search_description"`
	SelectionPreference  string  `json:"selection_preference"`
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
						"search_description", "selection_preference",
					},
					"properties": map[string]any{
						"search_description": map[string]any{
							"type":        "string",
							"description": "The requested material without quantity or selection preferences, retaining every explicit product constraint.",
						},
						"selection_preference": map[string]any{
							"type": "string",
							"enum": []string{string(domain.RFQSelectionNone), string(domain.RFQSelectionLowestPrice), string(domain.RFQSelectionHighestPrice)},
						},
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
