package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var _ domain.CatalogMatchReviewer = (*CatalogMatchReviewer)(nil)

// reviewChunk is how many lines one review call carries.
const reviewChunk = 10

// CatalogMatchReviewer settles the lines catalog matching flagged, with trade knowledge the
// catalog's own text does not carry, such as a brand used as the generic name of a product. It
// only ever chooses among the candidates matching offered.
type CatalogMatchReviewer struct {
	generator domain.StructuredGenerator
}

// NewCatalogMatchReviewer builds a CatalogMatchReviewer on the bound language model.
func NewCatalogMatchReviewer(generator domain.StructuredGenerator) *CatalogMatchReviewer {
	return &CatalogMatchReviewer{generator: generator}
}

// Review asks for one verdict per line, reviewChunk lines to a call and the calls side by side.
// A long answer is where a model starts skipping lines, and a chunk that fails costs only its own
// lines, which keep the matcher's decision. It errs only when no chunk came back.
func (r *CatalogMatchReviewer) Review(
	ctx context.Context, lines []domain.MatchReviewLine,
) ([]domain.MatchReviewDecision, error) {
	decisions := make([]domain.MatchReviewDecision, len(lines))
	chunks := (len(lines) + reviewChunk - 1) / reviewChunk
	errs := make([]error, chunks)
	var wg sync.WaitGroup
	for c := range chunks {
		start, end := c*reviewChunk, min((c+1)*reviewChunk, len(lines))
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[c] = r.reviewChunk(ctx, lines[start:end], decisions[start:end])
		}()
	}
	wg.Wait()
	if chunks > 0 && !slices.ContainsFunc(errs, func(err error) bool { return err == nil }) {
		return nil, errors.Join(errs...)
	}
	return decisions, nil
}

// reviewChunk reviews one batch into decisions. A line's candidates travel under codes numbered
// from 1 and the schema's enum is exactly those numbers, so the model cannot name a product it
// was not shown; a code past the end of a shorter line voids that line's verdict.
func (r *CatalogMatchReviewer) reviewChunk(
	ctx context.Context, lines []domain.MatchReviewLine, decisions []domain.MatchReviewDecision,
) error {
	widest := 0
	prompt := make([]reviewPromptLine, len(lines))
	for i, line := range lines {
		prompt[i] = reviewPromptLine{Line: i + 1, Description: line.Description}
		for j, candidate := range line.Candidates {
			prompt[i].Candidates = append(prompt[i].Candidates, reviewPromptCandidate{
				Code: strconv.Itoa(j + 1), Name: candidate.Name, Description: candidate.Description,
				Unit: candidate.Unit,
			})
		}
		widest = max(widest, len(line.Candidates))
	}
	payload, err := json.Marshal(prompt)
	if err != nil {
		return err
	}
	codes := make([]string, widest)
	for i := range codes {
		codes[i] = strconv.Itoa(i + 1)
	}

	var answer reviewAnswer
	if _, err := r.generator.Generate(ctx, domain.GenerationRequest{
		Instructions: reviewInstructions,
		Input: []domain.Content{domain.TextContent(fmt.Sprintf(
			"Review all %d lines:\n%s", len(lines), payload))},
		Schema: reviewSchema(len(lines), codes),
	}, &answer); err != nil {
		return err
	}

	// A line the answer skips or repeats keeps the matcher's own decision: an empty verdict.
	seen := make([]bool, len(lines))
	for _, verdict := range answer.Lines {
		i := verdict.Line - 1
		if i < 0 || i >= len(lines) || seen[i] {
			continue
		}
		seen[i] = true
		decision := domain.MatchReviewDecision{
			Verdict: domain.MatchReviewVerdict(verdict.Verdict), Reason: verdict.Reason,
		}
		// A code past this line's candidates is a verdict about something it was not shown, so the
		// whole verdict is void rather than trimmed into one that looks well-formed.
		valid := true
		for _, code := range verdict.Candidates {
			j, err := strconv.Atoi(code)
			if err != nil || j < 1 || j > len(lines[i].Candidates) {
				valid = false
				break
			}
			decision.Chosen = append(decision.Chosen, j-1)
		}
		if valid {
			decisions[i] = decision
		}
	}
	return nil
}

// reviewInstructions is the stable half of the prompt, ahead of the lines so a provider that
// caches a prefix pays full price only for the order being reviewed.
const reviewInstructions = `You check the lines of informal orders that clients send a building-materials supplier in Argentina. Automatic matching could not settle these lines against the supplier's catalog, so for each one you are shown the client's words and the catalog products it was offered, each under a code numbered from 1 within that line.

Answer every line exactly once, in order, by its number: first a one-sentence reason in Argentine Spanish, then one verdict:

- ONE when the line identifies exactly one of the offered products: everything the line states fits it, and nothing the line leaves unsaid would change which product it is. Give that product's code.
- SEVERAL when more than one offered product fits what the line states and the line does not say which. Give the codes of the ones that fit, most likely first.
- NONE when no offered product is what the line asks for. Give no code.

Read the line the way an Argentine seller would: brand names used as the generic name of a product, trade terms and colour conventions of the trade, common abbreviations, spellings typed by ear, and measurements written in another unit.

Never infer a brand, size, colour, length or variant the line does not state: a line naming only a kind of product, against several brands or sizes of it, is SEVERAL, not ONE. A product that is only related — an accessory for what was asked, a different kind of product for the same job, another size — is not a match. Choose only among the codes shown for that line. They are a shortlist from a larger catalog, not all of it: that only one offered product fits is not evidence that it is the one the client means, unless the line itself says so.`

type reviewPromptLine struct {
	Line        int                     `json:"line"`
	Description string                  `json:"client_line"`
	Candidates  []reviewPromptCandidate `json:"offered_products"`
}

type reviewPromptCandidate struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Unit        *string `json:"unit,omitempty"`
}

// reviewAnswer is the shape reviewSchema forces. It carries every property the schema declares,
// because the decoder refuses an answer with a field it does not know.
type reviewAnswer struct {
	Lines []reviewVerdict `json:"lines"`
}

type reviewVerdict struct {
	Line       int      `json:"line"`
	Reason     string   `json:"reason"`
	Verdict    string   `json:"verdict"`
	Candidates []string `json:"candidates"`
}

// reviewSchema is the forced shape of the answer: the verdict is a closed enum, and so is every
// product code, one of the numbers the widest line's candidates were given.
func reviewSchema(lines int, codes []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"lines"},
		"properties": map[string]any{
			"lines": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"line", "reason", "verdict", "candidates"},
					"properties": map[string]any{
						"line": map[string]any{
							"type":        "integer",
							"description": fmt.Sprintf("The line's number, from 1 to %d.", lines),
						},
						"reason": map[string]any{
							"type":        "string",
							"description": "One short sentence in Argentine Spanish on why, for the seller.",
						},
						"verdict": map[string]any{
							"type": "string",
							"enum": []string{
								string(domain.MatchReviewVerdictOne),
								string(domain.MatchReviewVerdictSeveral),
								string(domain.MatchReviewVerdictNone),
							},
						},
						"candidates": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string", "enum": codes},
							"description": "Codes of the products that fit, best first. Empty on NONE.",
						},
					},
				},
			},
		},
	}
}
