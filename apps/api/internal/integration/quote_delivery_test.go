//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

type captureWhatsAppSender struct {
	mu       sync.Mutex
	messages []domain.QuoteWhatsAppMessage
	err      error
}

func (s *captureWhatsAppSender) SendQuote(_ context.Context,
	message domain.QuoteWhatsAppMessage) (*domain.DeliveryReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	if s.err != nil {
		return nil, s.err
	}
	return &domain.DeliveryReceipt{ProviderReference: "wa-" + message.DeliveryID.String()}, nil
}

func (s *captureWhatsAppSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

type stagedQuoteEmailSender struct {
	mu    sync.Mutex
	sends int
	err   error
}

func (s *stagedQuoteEmailSender) Send(context.Context, services.OutboundMail) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sends++
	return s.err
}

type failingQuoteEvaluator struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (e *failingQuoteEvaluator) EvaluateFinalQuote(context.Context, domain.Tenant,
	uuid.UUID, uuid.UUID) (*domain.QuoteQualityEvaluation, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	return nil, e.err
}

func (e *failingQuoteEvaluator) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

type sendableQuote struct {
	tenant domain.Tenant
	draft  *domain.TextRFQDraft
}

func (e *env) seedSendableQuote(t *testing.T, name string, corrected bool) sendableQuote {
	t.Helper()
	accountID, branchID := e.seedAccount(t, name)
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	seller := e.seedUser(t, accountID, domain.UserRoleAdmin)
	productID := e.seedPricedProduct(t, accountID, branchID, name+" cement", "50", nil)
	e.embedOn(t, productID, 0, 0.98)
	unit := "bag"
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: seller.ID,
		Role: domain.UserRoleAdmin}
	draft, err := e.pipeline(t, stagedExtractor{lines: []domain.ExtractedRFQLine{{
		RequestedDescription: "two cement bags", Quantity: decimal.NewFromInt(2), Unit: &unit,
		Source: domain.QuantitySourceExplicit, QuantityRationale: "two bags were requested",
	}}}, map[string]int{"two cement bags": 0}).CreateTextDraft(context.Background(), tenant,
		domain.TextRFQDraftInput{ChannelID: channelID, RawText: "two cement bags"})
	if err != nil {
		t.Fatalf("CreateTextDraft() = %v", err)
	}
	e.dropDraft(t, draft.RFQ.ID)
	if corrected {
		if _, err := e.db.CrossAccount().Exec(context.Background(),
			`UPDATE quote_item SET quantity = 3 WHERE id = $1`, draft.Items[0].ID); err != nil {
			t.Fatalf("apply seller correction: %v", err)
		}
	}
	quoteService := services.NewQuoteService(e.db, repository.NewQuoteRepository(),
		repository.NewProductPriceRepository(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := quoteService.AcceptMaterials(context.Background(), tenant,
		draft.Quote.ID); err != nil {
		t.Fatalf("AcceptMaterials() = %v", err)
	}
	return sendableQuote{tenant: tenant, draft: draft}
}

func (e *env) quoteDeliveryService(whatsapp domain.QuoteWhatsAppSender,
	email interface {
		Send(context.Context, services.OutboundMail) error
	},
	evaluator services.QuoteQualityEvaluator) *services.QuoteDeliveryService {
	return services.NewQuoteDeliveryService(e.db, repository.NewQuoteSendRepository(),
		repository.NewQuoteRepository(), repository.NewRFQRepository(),
		repository.NewClientRepository(), repository.NewChannelRepository(),
		repository.NewBranchRepository(), whatsapp, email, evaluator,
		"https://quotes.test", nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (e *env) realQualityEvaluator(embedder domain.Embedder) services.QuoteQualityEvaluator {
	corrections := services.NewQuoteCorrectionService(e.db,
		repository.NewQuoteCorrectionRepository(), embedder, config.QuoteCorrectionConfig{
			SimilarityPercent: 80, MaxPatternsPerAccount: 1000,
			MaxInterpretationExamples: 3, ProcessingBatchSize: 100,
		}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return services.NewQuoteQualityService(e.db, repository.NewQuoteQualityRepository()).
		WithCorrectionLearning(corrections)
}

func TestQuoteDelivery_SuccessFreezesSendsAndEvaluatesUnchangedQuote(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Delivery unchanged", false)
	whatsapp := &captureWhatsAppSender{}
	service := e.quoteDeliveryService(whatsapp, &stagedQuoteEmailSender{},
		e.realQualityEvaluator(axisEmbedder{axes: map[string]int{}}))
	key := uuid.New()

	result, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: key, Phone: "+5491155550101"})
	if err != nil {
		t.Fatalf("Send() = %v", err)
	}
	if result.CurrentStatus != domain.QuoteStatusSent || len(result.Deliveries) != 1 ||
		result.Deliveries[0].TrackingStatus != domain.SendTrackingStatusSent {
		t.Fatalf("result = %+v, want one successful WhatsApp delivery", result)
	}
	if whatsapp.count() != 1 || result.ExpiresAt == nil {
		t.Fatalf("whatsapp calls/expires = %d/%v, want 1/non-nil", whatsapp.count(), result.ExpiresAt)
	}

	var immutable, correct bool
	var evaluations, memories int
	err = e.db.CrossAccount().QueryRow(context.Background(), `SELECT version.is_immutable,
		  evaluation.whole_quote_correct,
		  (SELECT count(*) FROM quote_quality_evaluation e WHERE e.id = evaluation.id),
		  (SELECT count(*) FROM quote_correction_memory m WHERE m.account_id = quote.account_id)
		FROM quote JOIN quote_version version ON version.id = quote.current_version_id
		JOIN quote_ai_generation generation ON generation.quote_id = quote.id
		JOIN quote_quality_evaluation evaluation ON evaluation.generation_id = generation.id
		WHERE quote.id = $1`, seed.draft.Quote.ID).
		Scan(&immutable, &correct, &evaluations, &memories)
	if err != nil {
		t.Fatalf("read committed delivery evaluation: %v", err)
	}
	if !immutable || !correct || evaluations != 1 || memories != 0 {
		t.Errorf("immutable/correct/evaluations/memories = %v/%v/%d/%d, want true/true/1/0",
			immutable, correct, evaluations, memories)
	}
	token := strings.TrimPrefix(result.Deliveries[0].PublicURL, "https://quotes.test/quotes/")
	public, err := service.ResolvePublic(context.Background(), token)
	if err != nil || public.Status != "ACTIVE" {
		t.Fatalf("ResolvePublic() = %+v, %v, want ACTIVE", public, err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_send SET expires_at = $2 WHERE id = $1`, result.Deliveries[0].ID,
		time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("expire public delivery: %v", err)
	}
	public, err = service.ResolvePublic(context.Background(), token)
	if err != nil || public.Status != "EXPIRED" {
		t.Fatalf("expired ResolvePublic() = %+v, %v, want EXPIRED", public, err)
	}
	if _, err := service.ResolvePublic(context.Background(), "unknown-token"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown ResolvePublic() = %v, want not found", err)
	}

	replay, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: key, Phone: "+5491155550101"})
	if err != nil || !replay.Replay || whatsapp.count() != 1 ||
		replay.Deliveries[0].ID != result.Deliveries[0].ID {
		t.Errorf("idempotent replay = %+v, %v, calls %d", replay, err, whatsapp.count())
	}
	if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: key, Phone: "+5491155550199"}); domain.CodeOf(err) != domain.CodeIdempotencyMismatch {
		t.Errorf("mismatched replay = %v, want %s", err, domain.CodeIdempotencyMismatch)
	}
}

func TestQuoteDelivery_CorrectionLearnsAndPostCommitEvaluationFailureDoesNotFailSend(t *testing.T) {
	t.Run("corrected quote", func(t *testing.T) {
		e := newEnv(t)
		seed := e.seedSendableQuote(t, "Delivery corrected", true)
		service := e.quoteDeliveryService(&captureWhatsAppSender{}, &stagedQuoteEmailSender{},
			e.realQualityEvaluator(failingCorrectionEmbedder{}))
		if _, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
			domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(),
				Phone: "+5491155550102"}); err != nil {
			t.Fatalf("Send() = %v", err)
		}
		var correct bool
		var differences, memories int
		if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT
		  evaluation.whole_quote_correct,
		  (SELECT count(*) FROM quote_quality_difference d WHERE d.evaluation_id = evaluation.id),
		  (SELECT count(*) FROM quote_correction_memory m WHERE m.account_id = quote.account_id)
		FROM quote JOIN quote_ai_generation generation ON generation.quote_id = quote.id
		JOIN quote_quality_evaluation evaluation ON evaluation.generation_id = generation.id
		WHERE quote.id = $1`, seed.draft.Quote.ID).Scan(&correct, &differences, &memories); err != nil {
			t.Fatalf("read corrected evaluation: %v", err)
		}
		if correct || differences == 0 || memories == 0 {
			t.Errorf("correct/differences/memories = %v/%d/%d, want false/nonzero/nonzero",
				correct, differences, memories)
		}
	})

	t.Run("evaluation failure", func(t *testing.T) {
		e := newEnv(t)
		seed := e.seedSendableQuote(t, "Delivery evaluation outage", false)
		evaluator := &failingQuoteEvaluator{err: errors.New("evaluation outage")}
		service := e.quoteDeliveryService(&captureWhatsAppSender{}, &stagedQuoteEmailSender{}, evaluator)
		result, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
			domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550103"})
		if err != nil || result.CurrentStatus != domain.QuoteStatusSent || evaluator.count() != 1 {
			t.Fatalf("result/error/evaluations = %+v/%v/%d, want SENT/nil/1", result, err,
				evaluator.count())
		}
		realEvaluator := e.realQualityEvaluator(axisEmbedder{axes: map[string]int{}})
		job := services.NewQuoteQualityJob(repository.NewQuoteSendRepository(), realEvaluator,
			config.QuoteQualityConfig{ProcessingBatchSize: 100})
		first, err := job.Run(context.Background(), e.db.CrossAccount())
		if err != nil || first.Changed != 1 {
			t.Fatalf("first retry = %+v, %v, want one recovered evaluation", first, err)
		}
		second, err := job.Run(context.Background(), e.db.CrossAccount())
		if err != nil || second.Changed != 0 {
			t.Fatalf("idempotent retry = %+v, %v, want no duplicate evaluation", second, err)
		}
	})
}

func TestQuoteDelivery_ChannelsAreIndependentAndTenantBoundariesHold(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Delivery isolation", false)
	emailChannel := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO channel
	  (id, account_id, branch_id, type, identifier, is_active)
	  VALUES ($1, $2, $3, 'EMAIL', 'quotes@test.local', TRUE)`, emailChannel,
		seed.tenant.AccountID, seed.tenant.BranchID); err != nil {
		t.Fatalf("seed email channel: %v", err)
	}
	whatsapp := &captureWhatsAppSender{err: errors.New("whatsapp outage")}
	email := &stagedQuoteEmailSender{}
	service := e.quoteDeliveryService(whatsapp, email,
		e.realQualityEvaluator(axisEmbedder{axes: map[string]int{}}))
	address := "client@test.local"
	result, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550104",
			Email: &address})
	if err != nil || result.CurrentStatus != domain.QuoteStatusSent {
		t.Fatalf("partial Send() = %+v, %v, want committed SENT", result, err)
	}
	statuses := map[domain.ChannelType]domain.SendTrackingStatus{}
	for _, delivery := range result.Deliveries {
		statuses[delivery.ChannelType] = delivery.TrackingStatus
	}
	if statuses[domain.ChannelTypeWhatsApp] != domain.SendTrackingStatusFailed ||
		statuses[domain.ChannelTypeEmail] != domain.SendTrackingStatusSent {
		t.Errorf("statuses = %+v, want WhatsApp failed and email sent", statuses)
	}

	otherAccount, otherBranch := e.seedAccount(t, "Delivery intruder")
	other := domain.Tenant{AccountID: otherAccount, BranchID: otherBranch,
		UserID: uuid.New(), Role: domain.UserRoleAdmin}
	if _, err := service.Send(context.Background(), other, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(),
			Phone: "+5491155550105"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cross-account Send() = %v, want not found", err)
	}
	otherBranchSameAccount := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO branch
	  (id, account_id, name) VALUES ($1, $2, 'Other branch')`, otherBranchSameAccount,
		seed.tenant.AccountID); err != nil {
		t.Fatalf("seed other branch: %v", err)
	}
	t.Cleanup(func() { e.mustCleanup(t, `DELETE FROM branch WHERE id = $1`, otherBranchSameAccount) })
	wrongBranch := seed.tenant
	wrongBranch.BranchID = otherBranchSameAccount
	if _, err := service.Send(context.Background(), wrongBranch, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(),
			Phone: "+5491155550106"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cross-branch Send() = %v, want not found", err)
	}
}

func TestQuoteDelivery_AllChannelsFailLeavesQuoteQuotedAndDoesNotEvaluate(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Delivery provider failure", false)
	whatsapp := &captureWhatsAppSender{err: errors.New("provider outage")}
	evaluator := &failingQuoteEvaluator{}
	service := e.quoteDeliveryService(whatsapp, &stagedQuoteEmailSender{}, evaluator)
	_, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550107"})
	if !errors.Is(err, domain.ErrDeliveryUnavailable) {
		t.Fatalf("Send() = %v, want delivery unavailable", err)
	}
	var status string
	var immutable bool
	var sendStatus string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT quote.current_status,
	  version.is_immutable, send.tracking_status
	  FROM quote JOIN quote_version version ON version.id = quote.current_version_id
	  JOIN quote_send send ON send.version_id = version.id WHERE quote.id = $1`,
		seed.draft.Quote.ID).Scan(&status, &immutable, &sendStatus); err != nil {
		t.Fatalf("read failed delivery: %v", err)
	}
	if status != "QUOTED" || !immutable || sendStatus != "FAILED" || evaluator.count() != 0 {
		t.Errorf("status/immutable/send/evaluations = %s/%v/%s/%d, want QUOTED/true/FAILED/0",
			status, immutable, sendStatus, evaluator.count())
	}
}

func TestQuoteDelivery_ConcurrentIdempotentRequestsDeliverOnce(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Delivery concurrent idempotency", false)
	whatsapp := &captureWhatsAppSender{}
	service := e.quoteDeliveryService(whatsapp, &stagedQuoteEmailSender{},
		e.realQualityEvaluator(axisEmbedder{axes: map[string]int{}}))
	input := domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550108"}
	results := make([]*domain.QuoteDeliveryResult, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], errs[index] = service.Send(context.Background(), seed.tenant,
				seed.draft.Quote.ID, input)
		}(i)
	}
	close(start)
	wg.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("concurrent Send() errors = %v / %v", errs[0], errs[1])
	}
	if whatsapp.count() != 1 || results[0].Deliveries[0].ID != results[1].Deliveries[0].ID ||
		results[0].Replay == results[1].Replay {
		t.Errorf("calls/results = %d / %+v / %+v, want one delivery and one replay",
			whatsapp.count(), results[0], results[1])
	}
}
