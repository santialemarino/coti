//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// meteredGenerator answers with a staged extraction and meters itself the way a provider adapter
// does, so the attribution the pipeline sets is what reaches the ledger.
type meteredGenerator struct {
	answer string
	meter  *ai.Meter
}

func (g meteredGenerator) Generate(
	ctx context.Context, _ domain.GenerationRequest, out any,
) (*domain.GenerationUsage, error) {
	g.meter.Observe(ctx, ai.Call{Provider: "integration-provider", Model: "integration-model",
		Method: "generate", Attempts: 1, InputTokens: 1500, OutputTokens: 200}, nil)
	return &domain.GenerationUsage{Provider: "integration-provider", Model: "integration-model"},
		json.Unmarshal([]byte(g.answer), out)
}

// meteredEmbedder is axisEmbedder metered like a provider adapter.
type meteredEmbedder struct {
	axisEmbedder
	meter *ai.Meter
}

func (e meteredEmbedder) Embed(ctx context.Context, texts []string) ([]pgvector.Vector, error) {
	e.meter.Observe(ctx, ai.Call{Provider: "integration-provider", Model: "integration-embedder",
		Method: "embed", Attempts: 1, InputTokens: 8 * len(texts)}, nil)
	return e.axisEmbedder.Embed(ctx, texts)
}

type usageRow struct {
	operation   string
	branchID    *uuid.UUID
	rfqID       *uuid.UUID
	inputTokens int
}

// The seam this proves: an order arrives, every provider call it causes is paid for, and each row
// names the order, the branch and the work — without any stage passing that along by hand.
func TestAIUsage_RecordsEveryCallAnOrderCausesUnderThatOrder(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "AI usage")
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	ctx := context.Background()
	cement := e.seedProduct(t, accountID, "Cemento Portland 50kg", "bolsa de cemento")
	e.stock(t, accountID, branchID, cement)
	e.embedOn(t, cement, 0, 0.95)

	meter := ai.NewMeter(slog.New(slog.NewTextHandler(io.Discard, nil)),
		services.NewAIUsageService(e.db, repository.NewAIUsageRepository(), 5*time.Second, nil))
	extractor := ai.NewRFQExtractor(meteredGenerator{meter: meter, answer: `{"items":[{
		"requested_description":"10 bolsas de cemento","quantity":"10","unit":"bolsa",
		"quantity_source":"EXPLICIT","quantity_rationale":"el cliente pidió 10 bolsas"}]}`}, 200)
	search := services.NewCatalogSearchService(e.db, repository.NewProductRepository(),
		meteredEmbedder{axisEmbedder{axes: map[string]int{"10 bolsas de cemento": 0}}, meter},
		matchConfig())
	pipeline := services.NewRFQService(e.db, repository.NewRFQRepository(),
		repository.NewQuoteRepository(), repository.NewQuoteSendRepository(),
		repository.NewQuoteAIGenerationRepository(), repository.NewChannelRepository(),
		repository.NewUserRepository(), extractor,
		services.NewCatalogMatchService(search, matchConfig()),
		slog.New(slog.NewTextHandler(io.Discard, nil)), rfqConfig())

	draft, err := pipeline.CreateTextDraft(ctx, domain.Tenant{AccountID: accountID,
		BranchID: branchID, UserID: seller.ID, Role: domain.UserRoleAdmin},
		domain.TextRFQDraftInput{ChannelID: channelID, RawText: "10 bolsas de cemento"})
	if err != nil {
		t.Fatalf("CreateTextDraft() = %v, want no error", err)
	}
	e.dropDraft(t, draft.RFQ.ID)

	rows, err := e.db.CrossAccount().Query(ctx,
		`SELECT operation, branch_id, rfq_id, input_tokens FROM ai_usage
		 WHERE account_id = $1 ORDER BY operation::text`, accountID)
	if err != nil {
		t.Fatalf("read the ledger: %v", err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (usageRow, error) {
		var r usageRow
		return r, row.Scan(&r.operation, &r.branchID, &r.rfqID, &r.inputTokens)
	})
	if err != nil {
		t.Fatalf("scan the ledger: %v", err)
	}

	want := []usageRow{
		{operation: "CATALOG_SEARCH", inputTokens: 8},
		{operation: "RFQ_EXTRACTION", inputTokens: 1500},
	}
	if len(got) != len(want) {
		t.Fatalf("ledger = %+v, want one extraction and one search", got)
	}
	for i, row := range got {
		if row.operation != want[i].operation || row.inputTokens != want[i].inputTokens {
			t.Errorf("row %d = %s with %d tokens, want %s with %d", i, row.operation,
				row.inputTokens, want[i].operation, want[i].inputTokens)
		}
		if row.rfqID == nil || *row.rfqID != draft.RFQ.ID {
			t.Errorf("%s rfq_id = %v, want the order it served", row.operation, row.rfqID)
		}
		if row.branchID == nil || *row.branchID != branchID {
			t.Errorf("%s branch_id = %v, want the branch the order arrived at", row.operation,
				row.branchID)
		}
	}
}

// Spend is one account's business and nobody's to rewrite: the request role sees only its own
// account's rows and can neither change nor remove one.
func TestAIUsage_IsAppendOnlyAndSeenOnlyByItsAccount(t *testing.T) {
	e := newEnv(t)
	owner, _ := e.seedAccount(t, "AI usage owner")
	other, _ := e.seedAccount(t, "AI usage other")
	ctx := context.Background()
	service := services.NewAIUsageService(e.db, repository.NewAIUsageRepository(), 5*time.Second,
		nil)
	service.Record(ctx, domain.AIUsage{
		AIUsageScope: domain.AIUsageScope{AccountID: owner,
			Operation: domain.AIOperationAudioTranscription},
		Provider: "openai", Model: "whisper-1", Succeeded: true, Attempts: 1, AudioSeconds: 12.345,
	})

	var seconds string
	if err := e.db.CrossAccount().QueryRow(ctx,
		`SELECT audio_seconds::text FROM ai_usage WHERE account_id = $1`, owner).
		Scan(&seconds); err != nil {
		t.Fatalf("read the recorded row: %v", err)
	}
	if seconds != "12.35" {
		t.Errorf("audio_seconds = %s, want 12.35", seconds)
	}

	count := func(account uuid.UUID) int {
		var n int
		if err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: account},
			func(q repository.Querier) error {
				return q.QueryRow(ctx, `SELECT count(*) FROM ai_usage`).Scan(&n)
			}); err != nil {
			t.Fatalf("count as the request role: %v", err)
		}
		return n
	}
	if got := count(owner); got != 1 {
		t.Errorf("rows the owner sees = %d, want its 1", got)
	}
	if got := count(other); got != 0 {
		t.Errorf("rows another account sees = %d, want none", got)
	}

	for _, statement := range []string{
		`UPDATE ai_usage SET input_tokens = 0`,
		`DELETE FROM ai_usage`,
	} {
		err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: owner},
			func(q repository.Querier) error {
				_, execErr := q.Exec(ctx, statement)
				return execErr
			})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("%s as the request role = %v, want insufficient_privilege", statement, err)
		}
	}
}
