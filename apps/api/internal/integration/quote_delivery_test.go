//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/pdf"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
	"github.com/santialemarino/coti/apps/api/internal/storage"
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

func (s *stagedQuoteEmailSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sends
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

type failingRepresentationEnsurer struct{}

func (failingRepresentationEnsurer) Ensure(context.Context, domain.Tenant,
	uuid.UUID) (*domain.QuoteRepresentationResult, error) {
	return nil, domain.ErrRepresentationUnavailable
}

func (failingRepresentationEnsurer) ResolvePublic(context.Context, uuid.UUID, uuid.UUID,
	time.Time, string) (*domain.PublicQuoteRepresentation, error) {
	return nil, domain.ErrRepresentationUnavailable
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

func (e *env) quoteDeliveryService(t *testing.T, whatsapp domain.QuoteWhatsAppSender,
	email interface {
		Send(context.Context, services.OutboundMail) error
	},
	evaluator services.QuoteQualityEvaluator) *services.QuoteDeliveryService {
	t.Helper()
	baseURL, _ := url.Parse("https://files.test")
	objects, err := storage.NewLocalStorage(t.TempDir(), baseURL,
		storage.NewURLSigner([]byte(testJWTSecret), nil))
	if err != nil {
		t.Fatal(err)
	}
	representations := services.NewQuoteRepresentationService(e.db,
		repository.NewQuoteRepresentationRepository(), repository.NewQuoteRepository(),
		objects, nil, pdf.NewQuoteRenderer(), 15*time.Minute, nil, nil)
	return services.NewQuoteDeliveryService(e.db, repository.NewQuoteSendRepository(),
		repository.NewQuoteRepository(), repository.NewChannelRepository(),
		repository.NewBranchRepository(), repository.NewProductPriceRepository(), whatsapp, email, evaluator,
		"https://quotes.test", nil, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithRepresentationService(representations)
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
	service := e.quoteDeliveryService(t, whatsapp, &stagedQuoteEmailSender{},
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

func TestQuoteDelivery_RepresentationFailureContactsNobody(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Delivery representation failure", false)
	whatsapp := &captureWhatsAppSender{}
	email := &stagedQuoteEmailSender{}
	service := e.quoteDeliveryService(t, whatsapp, email, nil).
		WithRepresentationService(failingRepresentationEnsurer{})
	_, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550188"})
	if !errors.Is(err, domain.ErrRepresentationUnavailable) {
		t.Fatalf("Send() = %v, want ErrRepresentationUnavailable", err)
	}
	if whatsapp.count() != 0 || email.sends != 0 {
		t.Fatalf("contacts = %d/%d, want none", whatsapp.count(), email.sends)
	}
}

func TestQuoteDelivery_CorrectionLearnsAndPostCommitEvaluationFailureDoesNotFailSend(t *testing.T) {
	t.Run("corrected quote", func(t *testing.T) {
		e := newEnv(t)
		seed := e.seedSendableQuote(t, "Delivery corrected", true)
		service := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{},
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
		service := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, evaluator)
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
	service := e.quoteDeliveryService(t, whatsapp, email,
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
	service := e.quoteDeliveryService(t, whatsapp, &stagedQuoteEmailSender{}, evaluator)
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
	service := e.quoteDeliveryService(t, whatsapp, &stagedQuoteEmailSender{},
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

// A token is bound to the exact version its send froze, so a newer version of the same quote sent
// later must leave what an earlier customer link serves untouched. The public read never consults
// the quote's current version: the send row owns the snapshot it resolves.
func TestQuoteDelivery_PublicTokenStaysPinnedToTheSentVersion(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Delivery token pinning", false)
	service := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{},
		e.realQualityEvaluator(axisEmbedder{axes: map[string]int{}}))

	first, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550121"})
	if err != nil {
		t.Fatalf("first Send() = %v", err)
	}
	tokenA := strings.TrimPrefix(first.Deliveries[0].PublicURL, "https://quotes.test/quotes/")

	// A second version arrives (the change-request flow) and is sent under its own fresh token; its
	// version and snapshot rows mirror what that flow publishes.
	secondVersion := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(), `
		INSERT INTO quote_version (id, account_id, quote_id, author_id, version_number, currency,
		                           total, is_immutable, frozen_at)
		SELECT $1, account_id, quote_id, NULL, version_number + 1, currency, total, TRUE, now()
		  FROM quote_version WHERE id = $2`, secondVersion, first.VersionID); err != nil {
		t.Fatalf("seed second version: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(), `
		INSERT INTO quote_representation (id, account_id, branch_id, quote_id, version_id,
		                                  schema_version, payload, message, pdf_storage_key,
		                                  pdf_content_type, pdf_size_bytes, pdf_sha256,
		                                  logo_fallback_used)
		SELECT gen_random_uuid(), account_id, branch_id, quote_id, $1, schema_version,
		       jsonb_set(payload, '{version_number}', '2'), message, pdf_storage_key,
		       pdf_content_type, pdf_size_bytes, pdf_sha256, logo_fallback_used
		  FROM quote_representation WHERE version_id = $2`, secondVersion,
		first.VersionID); err != nil {
		t.Fatalf("seed second version snapshot: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET current_version_id = $2 WHERE id = $1`, seed.draft.Quote.ID,
		secondVersion); err != nil {
		t.Fatalf("point quote at second version: %v", err)
	}

	second, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550122"})
	if err != nil {
		t.Fatalf("second Send() = %v", err)
	}
	tokenB := strings.TrimPrefix(second.Deliveries[0].PublicURL, "https://quotes.test/quotes/")

	firstStillPinned, err := service.ResolvePublic(context.Background(), tokenA)
	if err != nil || firstStillPinned.Payload == nil {
		t.Fatalf("old token ResolvePublic() = %+v, %v, want the original snapshot", firstStillPinned, err)
	}
	secondServed, err := service.ResolvePublic(context.Background(), tokenB)
	if err != nil || secondServed.Payload == nil {
		t.Fatalf("new token ResolvePublic() = %+v, %v, want the second snapshot", secondServed, err)
	}
	if firstStillPinned.Payload.VersionNumber != 1 || secondServed.Payload.VersionNumber != 2 {
		t.Errorf("version served = %d / %d, want 1 on the old token and 2 on the new one",
			firstStillPinned.Payload.VersionNumber, secondServed.Payload.VersionNumber)
	}
}

// clientAnswerService is the delivery service with the customer-answer surface wired, which the
// shared helper leaves off because most delivery tests never reach it.
func (e *env) clientAnswerService(t *testing.T, whatsapp domain.QuoteWhatsAppSender,
	email interface {
		Send(context.Context, services.OutboundMail) error
	}) *services.QuoteDeliveryService {
	t.Helper()
	return e.quoteDeliveryService(t, whatsapp, email,
		e.realQualityEvaluator(axisEmbedder{axes: map[string]int{}})).
		WithClientActions(repository.NewClientActionRepository(), repository.NewUserRepository()).
		WithMessages(repository.NewQuoteMessageRepository())
}

// sentQuoteToken sends the seeded quote and hands back the token its delivery published.
func (e *env) sentQuoteToken(t *testing.T, service *services.QuoteDeliveryService,
	seed sendableQuote) (string, uuid.UUID) {
	t.Helper()
	result, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550199"})
	if err != nil {
		t.Fatalf("Send() = %v", err)
	}
	return strings.TrimPrefix(result.Deliveries[0].PublicURL, "https://quotes.test/quotes/"),
		result.Deliveries[0].ID
}

/*
 * The customer's answer has to move the quote and leave the record of why, in one transaction, and
 * the seller has to hear about it. The action is tied to the send it came back through: a quote
 * delivered twice would otherwise leave its answer with no origin.
 */
func TestQuoteClientAction_AcceptMovesTheQuoteRecordsTheActionAndMailsTheSeller(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Client accepts", false)
	mail := &stagedQuoteEmailSender{}
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, mail)
	token, sendID := e.sentQuoteToken(t, service, seed)

	outcome, err := service.RecordClientAction(context.Background(), token,
		domain.ClientActionAccept)
	if err != nil {
		t.Fatalf("RecordClientAction() = %v, want no error", err)
	}
	if outcome.Status != domain.QuoteStatusAccepted {
		t.Errorf("status = %q, want ACCEPTED", outcome.Status)
	}

	var status string
	var actions, changes int
	var actionSend *uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT quote.current_status,
		        (SELECT count(*) FROM client_action a WHERE a.version_id = quote.current_version_id),
		        (SELECT count(*) FROM quote_status_change c
		          WHERE c.quote_id = quote.id AND c.new_status = 'ACCEPTED'),
		        (SELECT a.quote_send_id FROM client_action a
		          WHERE a.version_id = quote.current_version_id LIMIT 1)
		   FROM quote WHERE quote.id = $1`, seed.draft.Quote.ID).
		Scan(&status, &actions, &changes, &actionSend); err != nil {
		t.Fatalf("read the answered quote: %v", err)
	}
	if status != "ACCEPTED" || actions != 1 || changes != 1 {
		t.Errorf("status/actions/changes = %s/%d/%d, want ACCEPTED/1/1", status, actions, changes)
	}
	if actionSend == nil || *actionSend != sendID {
		t.Errorf("action's send = %v, want the delivery it came back through (%v)", actionSend, sendID)
	}
	if mail.count() == 0 {
		t.Error("the seller was not told their quote was answered")
	}
}

func TestQuoteClientAction_RejectMovesTheQuoteToRejected(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Client rejects", false)
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	token, _ := e.sentQuoteToken(t, service, seed)

	outcome, err := service.RecordClientAction(context.Background(), token,
		domain.ClientActionReject)
	if err != nil {
		t.Fatalf("RecordClientAction() = %v, want no error", err)
	}
	if outcome.Status != domain.QuoteStatusRejected {
		t.Errorf("status = %q, want REJECTED", outcome.Status)
	}
}

func TestQuoteClientAction_ChangeRequestCreatesAndResendsReviewedVersion(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Client requests a change", false)
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	token, sendID := e.sentQuoteToken(t, service, seed)
	message := "Add one cement bag"
	var productID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT product_id FROM quote_item WHERE version_id = $1 LIMIT 1`,
		*seed.draft.Quote.CurrentVersionID).Scan(&productID); err != nil {
		t.Fatalf("read original product: %v", err)
	}
	e.openPricePeriod(t, seed.tenant.AccountID, seed.tenant.BranchID, productID, "65", nil)

	result, err := service.RespondPublic(context.Background(), token,
		domain.ClientActionInput{Type: domain.ClientActionRequestChange, Message: &message})
	if err != nil {
		t.Fatalf("RespondPublic() = %v", err)
	}
	if result.QuoteStatus != domain.QuoteStatusChangeRequested {
		t.Errorf("status = %s, want CHANGE_REQUESTED", result.QuoteStatus)
	}

	var versionID, actionSendID, messageActionID, actionID uuid.UUID
	var versionNumber, originalItems, newItems, changes int
	var mutable bool
	var body, status string
	err = e.db.CrossAccount().QueryRow(context.Background(), `
		SELECT q.current_version_id, q.current_status, v.version_number, NOT v.is_immutable,
		  (SELECT count(*) FROM quote_item WHERE version_id = $2),
		  (SELECT count(*) FROM quote_item WHERE version_id = v.id),
		  a.id, a.quote_send_id, m.client_action_id, m.body,
		  (SELECT count(*) FROM quote_status_change c WHERE c.quote_id = q.id
		    AND c.previous_status = 'SENT' AND c.new_status = 'CHANGE_REQUESTED')
		FROM quote q JOIN quote_version v ON v.id = q.current_version_id
		JOIN client_action a ON a.version_id = $2
		JOIN quote_message m ON m.client_action_id = a.id
		WHERE q.id = $1`, seed.draft.Quote.ID, *seed.draft.Quote.CurrentVersionID).
		Scan(&versionID, &status, &versionNumber, &mutable, &originalItems, &newItems,
			&actionID, &actionSendID, &messageActionID, &body, &changes)
	if err != nil {
		t.Fatalf("read change request: %v", err)
	}
	if status != "CHANGE_REQUESTED" || versionNumber != 2 || !mutable ||
		originalItems != newItems || actionSendID != sendID || messageActionID != actionID ||
		body != message || changes != 1 {
		t.Errorf("draft/request = %s v%d mutable=%v items=%d/%d send=%v action=%v/%v body=%q history=%d",
			status, versionNumber, mutable, originalItems, newItems, actionSendID,
			messageActionID, actionID, body, changes)
	}
	if got := e.storedVersionTotal(t, versionID); !got.Equal(decimal.NewFromInt(130)) {
		t.Errorf("v2 total = %s, want 130 at the new branch price", got)
	}
	if got := e.storedVersionTotal(t, *seed.draft.Quote.CurrentVersionID); !got.Equal(decimal.NewFromInt(100)) {
		t.Errorf("v1 total = %s, want the original 100", got)
	}
	lines := e.storedLines(t, versionID)
	if len(lines) != 1 {
		t.Fatalf("v2 stored %d lines, want 1", len(lines))
	}
	assertStored(t, "v2 unit price", lines[0].unitPrice, "65.00")
	assertStored(t, "v2 subtotal", lines[0].subtotal, "130.00")
	assertStored(t, "v1 unit price", e.storedLines(t,
		*seed.draft.Quote.CurrentVersionID)[0].unitPrice, "50.00")
	var draftItemID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT id FROM quote_item WHERE version_id = $1 LIMIT 1`, versionID).
		Scan(&draftItemID); err != nil {
		t.Fatalf("read v2 item id: %v", err)
	}
	otherProductID := e.seedPricedProduct(t, seed.tenant.AccountID, seed.tenant.BranchID,
		"Replacement cement", "80", nil)
	rfqService := e.pipeline(t, stagedExtractor{}, map[string]int{})
	if _, err := rfqService.UpdateItem(context.Background(), seed.tenant,
		seed.draft.Quote.ID, draftItemID,
		domain.QuoteItemUpdate{ProductID: &otherProductID}); err != nil {
		t.Fatalf("replace v2 product: %v", err)
	}
	if got := e.storedLines(t, versionID)[0].unitPrice; got.Valid {
		t.Errorf("v2 price after replacing the product = %s, want null", got.Decimal)
	}
	if got := e.storedVersionTotal(t, versionID); !got.IsZero() {
		t.Errorf("v2 total after replacing the product = %s, want 0 pending review", got)
	}
	if _, err := rfqService.UpdateItem(context.Background(), seed.tenant,
		seed.draft.Quote.ID, draftItemID,
		domain.QuoteItemUpdate{ProductID: &productID}); err != nil {
		t.Fatalf("restore v2 product: %v", err)
	}

	quoteService := services.NewQuoteService(e.db, repository.NewQuoteRepository(),
		repository.NewProductPriceRepository(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	priced, err := quoteService.AcceptMaterials(context.Background(), seed.tenant,
		seed.draft.Quote.ID)
	if err != nil {
		t.Fatalf("AcceptMaterials(v2) = %v", err)
	}
	if priced.Quote.CurrentStatus != domain.QuoteStatusQuoted || priced.Version.ID != versionID {
		t.Errorf("reviewed version = %s/%v, want QUOTED/%v",
			priced.Quote.CurrentStatus, priced.Version.ID, versionID)
	}
	if !priced.Version.Total.Equal(decimal.NewFromInt(130)) {
		t.Errorf("reviewed v2 total = %s, want 130", priced.Version.Total)
	}
	resent, err := service.Send(context.Background(), seed.tenant, seed.draft.Quote.ID,
		domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: "+5491155550199"})
	if err != nil {
		t.Fatalf("Send(v2) = %v", err)
	}
	if resent.CurrentStatus != domain.QuoteStatusSent || resent.VersionID != versionID {
		t.Errorf("resent version = %s/%v, want SENT/%v", resent.CurrentStatus,
			resent.VersionID, versionID)
	}
	old, err := service.ResolvePublic(context.Background(), token)
	if err != nil || old.Payload == nil || old.Payload.VersionNumber != 1 {
		t.Errorf("original link = %+v, %v, want frozen v1", old, err)
	}
}

// A quote answers once. Without this the same link would move an accepted quote to rejected, which
// is a customer overwriting a decision the seller has already started acting on.
func TestQuoteClientAction_RefusesASecondAnswerOnTheSameQuote(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Client answers twice", false)
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	token, _ := e.sentQuoteToken(t, service, seed)

	if _, err := service.RecordClientAction(context.Background(), token,
		domain.ClientActionAccept); err != nil {
		t.Fatalf("first answer = %v, want no error", err)
	}
	_, err := service.RecordClientAction(context.Background(), token, domain.ClientActionReject)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second answer = %v, want a conflict", err)
	}
}

// An expired link showed prices that are no longer on offer, so it cannot close a quote.
func TestQuoteClientAction_RefusesAnExpiredLink(t *testing.T) {
	e := newEnv(t)
	seed := e.seedSendableQuote(t, "Client answers late", false)
	service := e.clientAnswerService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{})
	token, sendID := e.sentQuoteToken(t, service, seed)

	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote_send SET expires_at = $2 WHERE id = $1`, sendID,
		time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("expire the delivery: %v", err)
	}
	_, err := service.RecordClientAction(context.Background(), token, domain.ClientActionAccept)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("answer on an expired link = %v, want a conflict", err)
	}
}
