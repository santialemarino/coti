package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func reviewLines(n int) []domain.MatchReviewLine {
	lines := make([]domain.MatchReviewLine, n)
	for i := range lines {
		lines[i] = domain.MatchReviewLine{
			Description: fmt.Sprintf("line %d", i+1),
			Candidates: []domain.MatchReviewCandidate{
				{Name: "Durlock placa 12.5"}, {Name: "Durlock placa 9"},
			},
		}
	}
	return lines
}

func TestCatalogMatchReviewer_MapsEachVerdictOntoItsOwnLinesCandidates(t *testing.T) {
	lines := []domain.MatchReviewLine{
		{Description: "placas de yeso", Candidates: []domain.MatchReviewCandidate{
			{Name: "Durlock placa 12.5"}, {Name: "Durlock placa 9"}, {Name: "Ladrillo"},
		}},
		{Description: "cemento", Candidates: []domain.MatchReviewCandidate{
			{Name: "Cemento Loma Negra 25kg"},
		}},
		{Description: "pintura latex", Candidates: []domain.MatchReviewCandidate{
			{Name: "Pintura asfáltica 4L"},
		}},
	}
	generator := &fakeGenerator{answer: `{"lines":[
		{"line":1,"reason":"Son placas de yeso de dos espesores.","verdict":"SEVERAL",
		 "candidates":["2","1"]},
		{"line":2,"reason":"Es el único cemento.","verdict":"ONE","candidates":["1","3"]},
		{"line":3,"reason":"No hay látex.","verdict":"NONE","candidates":[]}
	]}`}

	decisions, err := NewCatalogMatchReviewer(generator).Review(context.Background(), lines)
	if err != nil {
		t.Fatalf("Review() = %v", err)
	}

	want := []domain.MatchReviewDecision{
		{Verdict: domain.MatchReviewVerdictSeveral, Chosen: []int{1, 0},
			Reason: "Son placas de yeso de dos espesores."},
		// "3" is a code the widest line handed out, but this line has one candidate: a verdict
		// about something it was not shown is void, not trimmed into a clean ONE.
		{},
		{Verdict: domain.MatchReviewVerdictNone, Reason: "No hay látex."},
	}
	if !reflect.DeepEqual(decisions, want) {
		t.Errorf("decisions = %+v, want %+v", decisions, want)
	}
}

// The schema is what makes the model unable to name a product it was not shown: its verdict and
// every code are closed enums, and the codes are exactly the ones this call handed out.
func TestCatalogMatchReviewer_ForcesAClosedSchema(t *testing.T) {
	generator := &fakeGenerator{answer: `{"lines":[]}`}
	lines := reviewLines(2)
	lines[1].Candidates = append(lines[1].Candidates, domain.MatchReviewCandidate{Name: "Tercera"})

	_, err := NewCatalogMatchReviewer(generator).Review(context.Background(), lines)
	if err != nil {
		t.Fatalf("Review() = %v", err)
	}

	answer := generator.req.Schema["properties"].(map[string]any)["lines"].(map[string]any)
	item := answer["items"].(map[string]any)
	properties := item["properties"].(map[string]any)
	verdicts := properties["verdict"].(map[string]any)["enum"]
	if !reflect.DeepEqual(verdicts, []string{"ONE", "SEVERAL", "NONE"}) {
		t.Errorf("verdict enum = %v", verdicts)
	}
	codes := properties["candidates"].(map[string]any)["items"].(map[string]any)["enum"]
	if !reflect.DeepEqual(codes, []string{"1", "2", "3"}) {
		t.Errorf("code enum = %v, want the widest line's three codes", codes)
	}
	if item["additionalProperties"] != false {
		t.Error("a verdict may carry fields the decoder does not know")
	}
	// The lines travel as data, under codes numbered within each line.
	input := generator.req.Input[0].Text
	if !strings.Contains(input, "Review all 2 lines") ||
		!strings.Contains(input, `"code":"3","name":"Tercera"`) {
		t.Errorf("input = %s, want both lines with their codes", input)
	}
}

func TestCatalogMatchReviewer_AttributesItsCallsToTheReview(t *testing.T) {
	generator := &fakeGenerator{answer: `{"lines":[]}`}
	ctx := domain.WithAIOperation(context.Background(), domain.AIOperationCatalogSearch)

	if _, err := NewCatalogMatchReviewer(generator).Review(ctx, reviewLines(1)); err != nil {
		t.Fatalf("Review() = %v", err)
	}
	if generator.operation != domain.AIOperationCatalogMatchReview {
		t.Errorf("operation = %q, want CATALOG_MATCH_REVIEW", generator.operation)
	}
}

// A line the answer skips, repeats or numbers out of range keeps the matcher's own decision.
func TestCatalogMatchReviewer_LeavesALineTheAnswerDidNotSettleEmpty(t *testing.T) {
	generator := &fakeGenerator{answer: `{"lines":[
		{"line":1,"reason":"Primera.","verdict":"ONE","candidates":["1"]},
		{"line":1,"reason":"Repetida.","verdict":"NONE","candidates":[]},
		{"line":9,"reason":"Fuera de rango.","verdict":"ONE","candidates":["1"]}
	]}`}

	reviewer := NewCatalogMatchReviewer(generator)
	decisions, err := reviewer.Review(context.Background(), reviewLines(3))
	if err != nil {
		t.Fatalf("Review() = %v", err)
	}

	if decisions[0].Verdict != domain.MatchReviewVerdictOne || decisions[0].Reason != "Primera." {
		t.Errorf("first line = %+v, want the first answer for it, not the repeat", decisions[0])
	}
	for i := 1; i < 3; i++ {
		if !reflect.DeepEqual(decisions[i], domain.MatchReviewDecision{}) {
			t.Errorf("line %d = %+v, want no verdict", i+1, decisions[i])
		}
	}
}

// chunkGenerator answers every call with ONE on each of its lines, and fails the calls a test
// names, so a review split across calls can be told apart per chunk.
type chunkGenerator struct {
	mu    sync.Mutex
	calls int
	fail  map[string]bool
}

func (g *chunkGenerator) Generate(
	_ context.Context, req domain.GenerationRequest, out any,
) (*domain.GenerationUsage, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	text := req.Input[0].Text
	var prompt []reviewPromptLine
	if err := json.Unmarshal([]byte(text[strings.Index(text, "\n")+1:]), &prompt); err != nil {
		return nil, err
	}
	if g.fail[prompt[0].Description] {
		return nil, domain.ErrAIUnavailable
	}
	answer := reviewAnswer{}
	for _, line := range prompt {
		answer.Lines = append(answer.Lines, reviewVerdict{
			Line: line.Line, Reason: line.Description, Verdict: "ONE", Candidates: []string{"1"},
		})
	}
	*out.(*reviewAnswer) = answer
	return &domain.GenerationUsage{}, nil
}

// A long answer is where a model starts skipping lines, so a review goes out in chunks — and a
// chunk that fails costs only its own lines.
func TestCatalogMatchReviewer_ReviewsInChunksAndSurvivesOneFailing(t *testing.T) {
	generator := &chunkGenerator{fail: map[string]bool{"line 11": true}}

	reviewer := NewCatalogMatchReviewer(generator)
	decisions, err := reviewer.Review(context.Background(), reviewLines(25))
	if err != nil {
		t.Fatalf("Review() = %v, want the chunks that answered kept", err)
	}

	if generator.calls != 3 {
		t.Errorf("calls = %d, want 25 lines in chunks of %d", generator.calls, reviewChunk)
	}
	for i, decision := range decisions {
		failed := i >= 10 && i < 20
		if failed && decision.Verdict != "" {
			t.Errorf("line %d = %+v, want no verdict from the failed chunk", i+1, decision)
		}
		// Each chunk numbers its lines from 1, and the answer lands back on the right line.
		if !failed && (decision.Verdict != domain.MatchReviewVerdictOne ||
			decision.Reason != fmt.Sprintf("line %d", i+1)) {
			t.Errorf("line %d = %+v, want its own verdict", i+1, decision)
		}
	}
}

// When no chunk answers there is nothing to apply, and the caller hears why.
func TestCatalogMatchReviewer_FailsWhenNoChunkAnswers(t *testing.T) {
	generator := &chunkGenerator{fail: map[string]bool{"line 1": true, "line 11": true}}

	_, err := NewCatalogMatchReviewer(generator).Review(context.Background(), reviewLines(15))
	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Errorf("Review() = %v, want the provider's failure", err)
	}
}

func TestCatalogMatchReviewer_AsksNothingForNoLines(t *testing.T) {
	generator := &chunkGenerator{}

	decisions, err := NewCatalogMatchReviewer(generator).Review(context.Background(), nil)
	if err != nil || len(decisions) != 0 || generator.calls != 0 {
		t.Errorf("Review(nil) = %v, %v after %d calls, want nothing asked", decisions, err,
			generator.calls)
	}
}
