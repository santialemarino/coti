//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/ai/provider"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// replyCase is one labelled customer reply to a sent quote.
type replyCase struct {
	Name     string   `json:"name"`
	Messages []string `json:"messages"`
	Context  []struct {
		Author string `json:"author"`
		Text   string `json:"text"`
	} `json:"context"`
	Want domain.CustomerDecision `json:"want"`
	// Also lists the other decisions a seller would accept for an ambiguous reply.
	Also []domain.CustomerDecision `json:"also"`
	// Clear marks an acceptance whose wording leaves no other reading: it should clear the
	// automatic threshold.
	Clear bool `json:"clear"`
	// ForbidConfidentAccept marks a reply that must never read as a confident acceptance.
	ForbidConfidentAccept bool  `json:"forbid_confident_accept"`
	Conditional           *bool `json:"conditional"`
	Partial               *bool `json:"partial"`
}

type replyResult struct {
	Name       string                  `json:"name"`
	Want       domain.CustomerDecision `json:"want"`
	Got        domain.CustomerDecision `json:"got"`
	Label      domain.CustomerDecision `json:"label"`
	Confidence int                     `json:"confidence"`
	Level      string                  `json:"level"`
	Correct    bool                    `json:"correct"`
	Error      string                  `json:"error,omitempty"`
}

// TestReplyClassificationEvaluation runs the change handler's live model over labelled Spanish
// replies and grades the decision the backend acts on. It is opt-in and costs a few cents.
func TestReplyClassificationEvaluation(t *testing.T) {
	if os.Getenv("REPLY_CLASSIFICATION_EVAL") != "1" {
		t.Skip("set REPLY_CLASSIFICATION_EVAL=1, or run pnpm eval:reply-classification, to " +
			"evaluate against a live model")
	}
	_ = godotenv.Load("../../.env")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	providers, err := provider.Bind(cfg.AI, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatalf("bind providers: %v", err)
	}
	if !providers.LanguageModelBound() {
		t.Fatal("AI_LLM_PROVIDER is disabled: the evaluation measures the real language model")
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "reply_classification", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []replyCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	unit := "bolsa"
	items := []domain.ChangeRequestItem{
		{ID: uuid.New(), RequestedDescription: "cemento portland 50 kg",
			Quantity: decimal.NewFromInt(10), Unit: &unit},
		{ID: uuid.New(), RequestedDescription: "arena gruesa", Quantity: decimal.NewFromInt(2)},
	}
	handler := ai.NewChangeRequestHandler(providers.Generator)
	results := make([]replyResult, len(cases))
	group, ctx := errgroup.WithContext(context.Background())
	group.SetLimit(4)
	for i, tc := range cases {
		group.Go(func() error {
			history := make([]domain.ConversationLine, 0, len(tc.Context))
			for _, line := range tc.Context {
				history = append(history, domain.ConversationLine{
					Author: domain.MessageAuthorType(line.Author), Body: line.Text})
			}
			result := replyResult{Name: tc.Name, Want: tc.Want}
			proposal, err := handler.Propose(ctx, domain.ChangeRequestInput{
				Message: strings.Join(tc.Messages, "\n"), Context: history, Items: items})
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Label, result.Got = proposal.CustomerDecision, proposal.EffectiveDecision()
				result.Confidence, result.Level = proposal.DecisionConfidencePercent, string(proposal.Level)
				result.Correct = result.Got == tc.Want || slices.Contains(tc.Also, result.Got)
				if tc.Conditional != nil && proposal.Conditional != *tc.Conditional {
					result.Correct = false
				}
				if tc.Partial != nil && proposal.PartialOrAlternative != *tc.Partial {
					result.Correct = false
				}
			}
			results[i] = result
			return nil
		})
	}
	_ = group.Wait()

	threshold := evalPercent(t, "CONVERSATION_AUTO_ACCEPT_MIN_CONFIDENCE_PERCENT", 90)
	correct, clearBelow := 0, []string{}
	var confidentlyWrong []string
	for i, result := range results {
		if result.Correct {
			correct++
		} else {
			t.Logf("MISSED %-32s want %-8s got %-8s (label %s, %d%%, %s) %s", result.Name, result.Want,
				result.Got, result.Label, result.Confidence, result.Level, result.Error)
		}
		wrongAccept := result.Got == domain.CustomerDecisionAccept && (result.Want != domain.CustomerDecisionAccept ||
			cases[i].ForbidConfidentAccept)
		if wrongAccept && result.Confidence >= threshold {
			confidentlyWrong = append(confidentlyWrong, result.Name)
		}
		if cases[i].Clear && result.Got == domain.CustomerDecisionAccept && result.Confidence < threshold {
			clearBelow = append(clearBelow, result.Name)
		}
	}
	accepts := make([]int, 0)
	for i, result := range results {
		if cases[i].Want == domain.CustomerDecisionAccept && result.Got == domain.CustomerDecisionAccept {
			accepts = append(accepts, result.Confidence)
		}
	}
	sort.Ints(accepts)
	accuracy := float64(correct) / float64(len(results))
	t.Logf("accuracy %.2f (%d/%d); acceptance confidences %v; clear acceptances under %d%%: %v",
		accuracy, correct, len(results), accepts, threshold, clearBelow)
	if path := os.Getenv("REPLY_CLASSIFICATION_EVAL_REPORT"); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			report, _ := json.MarshalIndent(results, "", "  ")
			_ = os.WriteFile(path, report, 0o644)
		}
	}
	if len(confidentlyWrong) > 0 {
		t.Errorf("read as an acceptance at %d%% or more: %v — the error an automatic acceptance "+
			"cannot afford", threshold, confidentlyWrong)
	}
	if minimum := float64(evalPercent(t, "REPLY_CLASSIFICATION_EVAL_MIN_ACCURACY_PERCENT", 90)) / 100; accuracy < minimum {
		t.Errorf("accuracy %.2f, want at least %.2f", accuracy, minimum)
	}
}

func evalPercent(t *testing.T, key string, fallback int) int {
	t.Helper()
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("%s = %q, want an integer percent", key, raw)
	}
	return value
}
