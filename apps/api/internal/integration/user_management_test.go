//go:build integration

// Package integration drives the real router against a real PostgreSQL, so the wiring
// between middleware, handler, service and SQL is exercised the way a request exercises it.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/ai"
	"github.com/santialemarino/coti/apps/api/internal/config"
	deliveryhttp "github.com/santialemarino/coti/apps/api/internal/delivery/http"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/ratelimit"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/secrets"
	"github.com/santialemarino/coti/apps/api/internal/services"
	storageprovider "github.com/santialemarino/coti/apps/api/internal/storage/provider"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

// testChannelKey is 32 bytes, so the channel sealer is enabled for the suite; the one test that
// needs a deployment with no key clears it through newEnv's mutator.
var testChannelKey = []byte(testJWTSecret)

type env struct {
	router *gin.Engine
	db     *repository.DB
	tokens *services.TokenService
	mail   *captureMailer
}

type rfqTestProviders struct {
	extractor domain.RFQExtractor
	embedder  domain.Embedder
}

// captureMailer stands in for the transport so a test can read the link that was mailed.
type captureMailer struct {
	mu   sync.Mutex
	sent []domain.EmailMessage
	err  error
}

func (m *captureMailer) Send(_ context.Context, msg domain.EmailMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, msg)
	return nil
}

// count returns how many messages the transport has been handed.
func (m *captureMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

// last returns the most recent message, or false when nothing was sent.
func (m *captureMailer) last() (domain.EmailMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return domain.EmailMessage{}, false
	}
	return m.sent[len(m.sent)-1], true
}

// newEnv builds the real router over a real database with external AI providers disabled.
func newEnv(t *testing.T, mutate ...func(*config.Config)) *env {
	t.Helper()
	return newEnvWithRFQProviders(t, rfqTestProviders{}, mutate...)
}

// newEnvWithRFQProviders lets an RFQ integration stage deterministic provider answers.
func newEnvWithRFQProviders(
	t *testing.T, providers rfqTestProviders, mutate ...func(*config.Config),
) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)

	appURL := os.Getenv("TEST_DATABASE_URL")
	adminURL := os.Getenv("TEST_DATABASE_ADMIN_URL")
	if appURL == "" || adminURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_DATABASE_ADMIN_URL are required for integration tests")
	}

	cfg := &config.Config{
		Environment: "test",
		Auth: config.AuthConfig{
			JWTSecret:         testJWTSecret,
			AccessTTL:         15 * time.Minute,
			RefreshTTL:        12 * time.Hour,
			MaxFailedAttempts: 5,
			LockoutDuration:   15 * time.Minute,
			PasswordMinLength: 8,
			PasswordResetTTL:  time.Hour,
			VerificationTTL:   48 * time.Hour,
		},
		Catalog: config.CatalogConfig{DefaultPageSize: 50, MaxPageSize: 200},
		RFQ: config.RFQConfig{
			MaxTextCharacters: 20000, MaxItems: 200, PipelineTimeout: 25 * time.Second,
		},
		Branch:  config.BranchConfig{DefaultExpiryDays: 7},
		Channel: config.ChannelConfig{EncryptionKey: testChannelKey},
		// The local adapter, over a directory this test owns: uploads, links and expiry are
		// exercised for real rather than against a double.
		Storage: config.StorageConfig{
			Provider:        config.StorageProviderLocal,
			Dir:             t.TempDir(),
			APIBaseURL:      "http://api.test",
			SigningSecret:   testJWTSecret,
			MaxFileSize:     10 * 1024 * 1024,
			SignedURLExpiry: 15 * time.Minute,
		},
		Web: config.WebConfig{BackofficeURL: "https://backoffice.test"},
		// Off for the suite at large, so an unrelated test cannot trip an allowance. The two
		// that exercise it build their own env.
		RateLimit: config.RateLimitConfig{Enabled: false},
	}

	for _, apply := range mutate {
		apply(cfg)
	}

	db, err := repository.NewDB(context.Background(), config.DatabaseConfig{
		URL: appURL, AdminURL: adminURL, MaxConns: 4, MinConns: 1,
		MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute, ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewDB() = %v, want no error", err)
	}
	t.Cleanup(db.Close)

	userRepo := repository.NewUserRepository()
	branchRepo := repository.NewBranchRepository()
	userBranchRepo := repository.NewUserBranchRepository()
	channelRepo := repository.NewChannelRepository()
	accountRepo := repository.NewAccountRepository()
	refreshTokenRepo := repository.NewRefreshTokenRepository()
	tokenService := services.NewTokenService(cfg.Auth.JWTSecret, cfg.Auth.AccessTTL, nil)
	authService := services.NewAuthService(db, userRepo, branchRepo,
		refreshTokenRepo, tokenService, cfg.Auth, nil)
	userService := services.NewUserService(db, userRepo, userBranchRepo, branchRepo, cfg.Auth)
	mailer := &captureMailer{}
	mailService := services.NewMailService(db, mailer, repository.NewNotificationRepository(),
		accountRepo, nil)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	authTokenRepo := repository.NewAuthTokenRepository()
	passwordService := services.NewPasswordService(db, userRepo, authTokenRepo,
		refreshTokenRepo, mailService, authService, quiet, cfg.Auth, cfg.Web, nil)
	verificationService := services.NewVerificationService(db, userRepo, authTokenRepo,
		mailService, quiet, cfg.Auth, cfg.Web, nil)
	productRepo := repository.NewProductRepository()
	productService := services.NewProductService(db, productRepo,
		repository.NewProductSynonymRepository(), repository.NewProductAlternativeRepository(), cfg.Catalog)
	branchCatalogService := services.NewBranchCatalogService(db, productRepo,
		repository.NewBranchProductRepository(), repository.NewProductPriceRepository(), nil)
	onboardingRepo := repository.NewOnboardingRepository()
	onboardingService := services.NewOnboardingService(db, onboardingRepo)
	// Providers default to disabled; RFQ tests may inject deterministic doubles into this wiring.
	embedder := providers.embedder
	if embedder == nil {
		embedder = ai.DisabledEmbedder{}
	}
	catalogSearchService := services.NewCatalogSearchService(db, productRepo, embedder, cfg.Catalog)
	quoteRepo := repository.NewQuoteRepository()
	extractor := providers.extractor
	if extractor == nil {
		extractor = ai.NewRFQExtractor(ai.DisabledGenerator{}, cfg.RFQ.MaxItems)
	}
	rfqService := services.NewRFQService(db, repository.NewRFQRepository(), quoteRepo, channelRepo,
		extractor,
		services.NewCatalogMatchService(catalogSearchService, cfg.Catalog), quiet, cfg.RFQ)
	quoteService := services.NewQuoteService(db, quoteRepo,
		repository.NewProductPriceRepository(), quiet)
	channelSealer, err := secrets.NewAESGCM(cfg.Channel.EncryptionKey)
	if err != nil {
		t.Fatalf("NewAESGCM() = %v, want no error", err)
	}
	channelService := services.NewChannelService(db, channelRepo, channelSealer)

	objectStorage, err := storageprovider.Bind(cfg.Storage, quiet)
	if err != nil {
		t.Fatalf("storage Bind() = %v, want no error", err)
	}
	rfqAttachmentService := services.NewRFQAttachmentService(db,
		repository.NewRFQAttachmentRepository(), objectStorage.Storage, cfg.Storage, nil)

	limiter := ratelimit.NewMemory(nil)
	mailTargetLimiter := handler.NewMailTargetLimiter(limiter, handler.MailTargetLimitOptions{
		Limit:   cfg.RateLimit.MailPerAddress,
		Window:  cfg.RateLimit.Window,
		Enabled: cfg.RateLimit.Enabled,
	})

	router := deliveryhttp.NewRouter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)),
		deliveryhttp.Handlers{
			Health:        handler.NewHealthHandler(db),
			Auth:          handler.NewAuthHandler(authService),
			Password:      handler.NewPasswordHandler(passwordService, mailTargetLimiter),
			Verification:  handler.NewVerificationHandler(verificationService, mailTargetLimiter),
			User:          handler.NewUserHandler(userService),
			Branch:        handler.NewBranchHandler(services.NewBranchService(db, branchRepo, channelRepo, cfg.Branch.DefaultExpiryDays)),
			Channel:       handler.NewChannelHandler(channelService),
			Product:       handler.NewProductHandler(productService),
			BranchCatalog: handler.NewBranchCatalogHandler(branchCatalogService),
			RFQ:           handler.NewRFQHandler(rfqService),
			RFQAttachment: handler.NewRFQAttachmentHandler(rfqAttachmentService, cfg.Storage.MaxFileSize),
			File:          handler.NewFileHandler(objectStorage.Local),
			Quote:         handler.NewQuoteHandler(quoteService),
			Account: handler.NewAccountHandler(services.NewAccountService(db, accountRepo,
				branchRepo, channelRepo, userRepo, onboardingRepo, authService, verificationService, quiet,
				cfg.Auth, cfg.Branch)),
			Onboarding: handler.NewOnboardingHandler(onboardingService),
		},
		deliveryhttp.Auth{Verifier: tokenService, Resolver: authService},
		deliveryhttp.RateLimit{Limiter: limiter})

	return &env{router: router, db: db, tokens: tokenService, mail: mailer}
}

// mustCleanup runs a teardown delete on the owner pool and fails the test when it cannot. A
// discarded error leaves rows behind while the suite still passes, and a cleanup belongs in
// t.Cleanup rather than a defer: cleanups run after the body's defers, so a pool one of them closed
// is already gone.
func (e *env) mustCleanup(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := e.db.CrossAccount().Exec(context.Background(), query, args...); err != nil {
		t.Errorf("cleanup %q: %v", query, err)
	}
}

// seedAccount creates an account with one branch through the owner pool, and removes both at
// the end of the test.
func (e *env) seedAccount(t *testing.T, name string) (accountID, branchID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	accountID, branchID = uuid.New(), uuid.New()

	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO account (id, name) VALUES ($1, $2)`, accountID, name); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, $3)`,
		branchID, accountID, name+" Centro"); err != nil {
		t.Fatalf("seed branch: %v", err)
	}

	t.Cleanup(func() {
		for _, stmt := range []string{
			`DELETE FROM onboarding_step_progress WHERE account_id = $1`,
			`DELETE FROM account_onboarding WHERE account_id = $1`,
			`DELETE FROM user_branch WHERE account_id = $1`,
			`DELETE FROM auth_token WHERE account_id = $1`,
			`DELETE FROM notification WHERE account_id = $1`,
			`DELETE FROM refresh_token WHERE account_id = $1`,
			`DELETE FROM app_user WHERE account_id = $1`,
			`DELETE FROM channel WHERE account_id = $1`,
			`DELETE FROM branch WHERE account_id = $1`,
			`DELETE FROM account WHERE id = $1`,
		} {
			e.mustCleanup(t, stmt, accountID)
		}
	})
	return accountID, branchID
}

func (e *env) seedUser(t *testing.T, accountID uuid.UUID, role domain.UserRole) domain.AppUser {
	t.Helper()
	id := uuid.New()
	user := domain.AppUser{
		ID: id, AccountID: accountID, Name: "Seed", Email: id.String() + "@test.local",
		Role: role, IsActive: true, SessionEpoch: 1,
	}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO app_user (id, account_id, name, email, password_hash, role, session_epoch)
		 VALUES ($1, $2, $3, $4, 'x', $5, $6)`,
		user.ID, user.AccountID, user.Name, user.Email, user.Role, user.SessionEpoch); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return user
}

func (e *env) tokenFor(t *testing.T, user domain.AppUser) string {
	t.Helper()
	raw, _, err := e.tokens.IssueAccessToken(user)
	if err != nil {
		t.Fatalf("IssueAccessToken() = %v, want no error", err)
	}
	return raw
}

type request struct {
	method string
	path   string
	token  string
	branch string
	body   any
}

func (e *env) do(t *testing.T, r request) *httptest.ResponseRecorder {
	t.Helper()
	var payload io.Reader
	if r.body != nil {
		encoded, err := json.Marshal(r.body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = bytes.NewReader(encoded)
	}

	req := httptest.NewRequest(r.method, r.path, payload)
	if r.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	if r.branch != "" {
		req.Header.Set("X-Branch-Id", r.branch)
	}

	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// errorCode reads the envelope's stable code, which is what a client branches on when one route
// answers the same status for more than one reason.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %s: %v", rec.Body, err)
	}
	return body.Code
}

func createUserBody(email string, role domain.UserRole, branchIDs []uuid.UUID) map[string]any {
	return map[string]any{
		"name": "Nuevo", "email": email, "password": "Una-clave-larga1",
		"role": string(role), "branch_ids": branchIDs,
	}
}

// RequireAdmin guards the whole /v1/users group. Before this it was written and mounted
// nowhere, so "a seller only reaches their role's functions" was undemonstrable.
func TestUsers_SellerIsForbidden(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón A")
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	token := e.tokenFor(t, seller)

	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/v1/users", nil},
		{http.MethodPost, "/v1/users", createUserBody("x@test.local", domain.UserRoleSeller, nil)},
		{http.MethodGet, "/v1/users/" + seller.ID.String(), nil},
		{http.MethodPut, "/v1/users/" + seller.ID.String(), map[string]any{
			"name": "X", "email": "x@test.local", "role": "SELLER"}},
		{http.MethodDelete, "/v1/users/" + seller.ID.String(), nil},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := e.do(t, request{method: tc.method, path: tc.path, token: token, body: tc.body})
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body)
			}
		})
	}
}

// The screen that asks a user to confirm their address needs to know whether it already is,
// or it tells someone who is done that a mail is on its way. The flag also depends on it being
// dropped the moment the address changes — a stamp proves one mailbox, not the next one.
func TestMe_ReportsWhetherTheAddressIsVerified(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)

	verified := func(t *testing.T) bool {
		t.Helper()
		rec := e.do(t, request{method: http.MethodGet, path: "/v1/me", token: token})
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v1/me: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
		}
		var me struct {
			EmailVerified bool `json:"email_verified"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
			t.Fatalf("decode /v1/me: %v", err)
		}
		return me.EmailVerified
	}

	if verified(t) {
		t.Fatal("a seeded user reports email_verified true, want false")
	}

	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE app_user SET email_verified_at = now() WHERE id = $1`, admin.ID); err != nil {
		t.Fatalf("stamp verification: %v", err)
	}
	if !verified(t) {
		t.Fatal("a confirmed address reports email_verified false, want true")
	}

	// Changing the address through the real endpoint has to drop it again.
	rec := e.do(t, request{method: http.MethodPut, path: "/v1/users/" + admin.ID.String(),
		token: token,
		body: map[string]any{"name": "Admin", "email": "otra+" + uuid.NewString() + "@corralon.test",
			"role": string(domain.UserRoleAdmin), "branch_ids": []uuid.UUID{branchID}}})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /v1/users: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
	}
	if verified(t) {
		t.Error("email_verified stayed true after the address changed")
	}
}

// The account comes from the session. There is no account field on the wire, and this asserts
// the created row lands in the caller's account rather than anywhere a body could name.
func TestUsers_CreatedInTheCallersAccount(t *testing.T) {
	e := newEnv(t)
	accountA, branchA := e.seedAccount(t, "Corralón A")
	accountB, branchB := e.seedAccount(t, "Corralón B")
	admin := e.seedUser(t, accountA, domain.UserRoleAdmin)

	// The body names account B's branch, which the admin of A may not assign.
	rec := e.do(t, request{method: http.MethodPost, path: "/v1/users",
		token: e.tokenFor(t, admin),
		body:  createUserBody("nuevo@test.local", domain.UserRoleSeller, []uuid.UUID{branchB})})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("assigning another account's branch: status = %d, want %d; body = %s",
			rec.Code, http.StatusUnprocessableEntity, rec.Body)
	}

	rec = e.do(t, request{method: http.MethodPost, path: "/v1/users",
		token: e.tokenFor(t, admin),
		body:  createUserBody("nuevo@test.local", domain.UserRoleSeller, []uuid.UUID{branchA})})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var created struct {
		ID        uuid.UUID   `json:"id"`
		BranchIDs []uuid.UUID `json:"branch_ids"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var storedAccount uuid.UUID
	var verifiedAt *time.Time
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT account_id, email_verified_at FROM app_user WHERE id = $1`,
		created.ID).Scan(&storedAccount, &verifiedAt); err != nil {
		t.Fatalf("read created user: %v", err)
	}
	if storedAccount != accountA {
		t.Errorf("created in account %v, want %v", storedAccount, accountA)
	}
	if storedAccount == accountB {
		t.Error("an admin of one account created a user in another")
	}
	// Nothing mails an admin-created user a confirmation link, so the row has to carry the
	// stamp already or AUTH_REQUIRE_VERIFIED_EMAIL would lock them out permanently.
	if verifiedAt == nil {
		t.Error("email_verified_at is null on an admin-created user")
	}
	if len(created.BranchIDs) != 1 || created.BranchIDs[0] != branchA {
		t.Errorf("branch_ids = %v, want [%v]", created.BranchIDs, branchA)
	}
}

// The response must never carry the password hash, whatever else changes about the shape.
func TestUsers_ResponseNeverCarriesThePasswordHash(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón A")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)

	rec := e.do(t, request{method: http.MethodPost, path: "/v1/users",
		token: e.tokenFor(t, admin),
		body:  createUserBody("nuevo@test.local", domain.UserRoleSeller, []uuid.UUID{branchID})})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, forbidden := range []string{"password", "password_hash", "PasswordHash"} {
		if _, present := raw[forbidden]; present {
			t.Errorf("response carries %q", forbidden)
		}
	}
}

// Another account's user is not found, never forbidden: a distinct answer would confirm the
// row exists.
func TestUsers_AnotherAccountsUserIsNotFound(t *testing.T) {
	e := newEnv(t)
	accountA, _ := e.seedAccount(t, "Corralón A")
	accountB, _ := e.seedAccount(t, "Corralón B")
	admin := e.seedUser(t, accountA, domain.UserRoleAdmin)
	victim := e.seedUser(t, accountB, domain.UserRoleSeller)
	token := e.tokenFor(t, admin)

	cases := []struct {
		method string
		body   any
	}{
		{http.MethodGet, nil},
		{http.MethodPut, map[string]any{"name": "Secuestrado", "email": "h@test.local", "role": "ADMIN"}},
		{http.MethodDelete, nil},
	}

	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			rec := e.do(t, request{method: tc.method, path: "/v1/users/" + victim.ID.String(),
				token: token, body: tc.body})
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body)
			}
		})
	}

	// Nothing above may have touched the row.
	var name string
	var active bool
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT name, is_active FROM app_user WHERE id = $1`, victim.ID).Scan(&name, &active); err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if name != "Seed" || !active {
		t.Errorf("victim is now name=%q active=%v; another account modified it", name, active)
	}
}

// An address identifies exactly one user everywhere, because login resolves by email alone.
func TestUsers_DuplicateEmailIsAConflictAcrossAccounts(t *testing.T) {
	e := newEnv(t)
	accountA, branchA := e.seedAccount(t, "Corralón A")
	accountB, branchB := e.seedAccount(t, "Corralón B")
	adminA := e.seedUser(t, accountA, domain.UserRoleAdmin)
	adminB := e.seedUser(t, accountB, domain.UserRoleAdmin)
	// Unique per run: the address has to be shared inside this test and nowhere else, because
	// the index is global and `go test ./internal/...` runs this package beside the repository
	// one, which exercises the same rule against the same table.
	shared := "compras+" + uuid.NewString() + "@corralon.test"

	first := e.do(t, request{method: http.MethodPost, path: "/v1/users", token: e.tokenFor(t, adminA),
		body: createUserBody(shared, domain.UserRoleSeller, []uuid.UUID{branchA})})
	if first.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want %d; body = %s", first.Code, http.StatusCreated, first.Body)
	}

	dup := e.do(t, request{method: http.MethodPost, path: "/v1/users", token: e.tokenFor(t, adminA),
		body: createUserBody(shared, domain.UserRoleSeller, []uuid.UUID{branchA})})
	if dup.Code != http.StatusConflict {
		t.Errorf("duplicate in the same account: status = %d, want %d; body = %s",
			dup.Code, http.StatusConflict, dup.Body)
	}
	if got := errorCode(t, dup); got != string(domain.CodeEmailTaken) {
		t.Errorf("duplicate in the same account: code = %q, want %q", got, domain.CodeEmailTaken)
	}

	other := e.do(t, request{method: http.MethodPost, path: "/v1/users", token: e.tokenFor(t, adminB),
		body: createUserBody(shared, domain.UserRoleSeller, []uuid.UUID{branchB})})
	if other.Code != http.StatusConflict {
		t.Errorf("same email in another account: status = %d, want %d; body = %s",
			other.Code, http.StatusConflict, other.Body)
	}
}

// Deactivating bumps the session epoch, so the tokens the user already holds stop working now
// rather than whenever they happen to expire.
func TestUsers_DeactivationKillsOutstandingTokens(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón A")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO user_branch (account_id, user_id, branch_id) VALUES ($1, $2, $3)`,
		accountID, seller.ID, branchID); err != nil {
		t.Fatalf("assign seller: %v", err)
	}
	sellerToken := e.tokenFor(t, seller)

	// The token works while the user is active.
	if rec := e.do(t, request{method: http.MethodGet, path: "/v1/branches",
		token: sellerToken, branch: branchID.String()}); rec.Code != http.StatusOK {
		t.Fatalf("before deactivation: status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
	}

	if rec := e.do(t, request{method: http.MethodDelete, path: "/v1/users/" + seller.ID.String(),
		token: e.tokenFor(t, admin)}); rec.Code != http.StatusNoContent {
		t.Fatalf("deactivate: status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body)
	}

	// Same token, unchanged and unexpired.
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/branches",
		token: sellerToken, branch: branchID.String()})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("after deactivation: status = %d, want %d; body = %s",
			rec.Code, http.StatusUnauthorized, rec.Body)
	}

	var epoch int
	var active bool
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT session_epoch, is_active FROM app_user WHERE id = $1`, seller.ID).Scan(&epoch, &active); err != nil {
		t.Fatalf("read seller: %v", err)
	}
	if active {
		t.Error("is_active = true after deactivation")
	}
	if epoch != seller.SessionEpoch+1 {
		t.Errorf("session_epoch = %d, want %d", epoch, seller.SessionEpoch+1)
	}
}

// An admin who deactivates themselves has no way back: there is no account-level recovery and
// no invitation flow.
func TestUsers_AdminCannotDeactivateThemselves(t *testing.T) {
	e := newEnv(t)
	accountID, _ := e.seedAccount(t, "Corralón A")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	token := e.tokenFor(t, admin)

	rec := e.do(t, request{method: http.MethodDelete, path: "/v1/users/" + admin.ID.String(), token: token})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("DELETE self: status = %d, want %d; body = %s",
			rec.Code, http.StatusUnprocessableEntity, rec.Body)
	}
	if got := errorCode(t, rec); got != string(domain.CodeSelfDeactivation) {
		t.Errorf("DELETE self: code = %q, want %q", got, domain.CodeSelfDeactivation)
	}

	rec = e.do(t, request{method: http.MethodPut, path: "/v1/users/" + admin.ID.String(), token: token,
		body: map[string]any{"name": "Admin", "email": admin.Email, "role": "ADMIN", "is_active": false}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("PUT self is_active=false: status = %d, want %d; body = %s",
			rec.Code, http.StatusUnprocessableEntity, rec.Body)
	}

	rec = e.do(t, request{method: http.MethodPut, path: "/v1/users/" + admin.ID.String(), token: token,
		body: map[string]any{"name": "Admin", "email": admin.Email, "role": "SELLER"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("PUT self role=SELLER: status = %d, want %d; body = %s",
			rec.Code, http.StatusUnprocessableEntity, rec.Body)
	}
	// Two refusals, one status, two codes: the screen removes the role control for the caller's
	// own row and keeps a message for the deactivation, and it can only tell them apart by code.
	if got := errorCode(t, rec); got != string(domain.CodeSelfRoleChange) {
		t.Errorf("PUT self role=SELLER: code = %q, want %q", got, domain.CodeSelfRoleChange)
	}

	// The admin must still be an active admin after all three refusals.
	var role string
	var active bool
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT role, is_active FROM app_user WHERE id = $1`, admin.ID).Scan(&role, &active); err != nil {
		t.Fatalf("read admin: %v", err)
	}
	if role != string(domain.UserRoleAdmin) || !active {
		t.Errorf("admin is now role=%s active=%v, want an active ADMIN", role, active)
	}
}

// The branch list is what the switcher reads: an admin sees the account, a seller sees their
// assignments.
func TestBranches_ListIsScopedToTheCaller(t *testing.T) {
	e := newEnv(t)
	accountID, firstBranch := e.seedAccount(t, "Corralón A")
	secondBranch := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Sucursal Dos')`,
		secondBranch, accountID); err != nil {
		t.Fatalf("seed second branch: %v", err)
	}

	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO user_branch (account_id, user_id, branch_id) VALUES ($1, $2, $3)`,
		accountID, seller.ID, firstBranch); err != nil {
		t.Fatalf("assign seller: %v", err)
	}

	list := func(t *testing.T, user domain.AppUser) []uuid.UUID {
		t.Helper()
		rec := e.do(t, request{method: http.MethodGet, path: "/v1/branches", token: e.tokenFor(t, user)})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
		}
		var body struct {
			Items []struct {
				ID uuid.UUID `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		ids := make([]uuid.UUID, 0, len(body.Items))
		for _, item := range body.Items {
			ids = append(ids, item.ID)
		}
		return ids
	}

	if got := list(t, admin); len(got) != 2 {
		t.Errorf("admin sees %v, want both branches", got)
	}
	got := list(t, seller)
	if len(got) != 1 || got[0] != firstBranch {
		t.Errorf("seller sees %v, want [%v]", got, firstBranch)
	}
}

// Omitting X-Branch-Id must not let a seller read a branch they are not assigned to. This is
// the boundary the whole branch scope exists for.
func TestPrices_SellerCannotReadAnUnassignedBranchByOmittingTheHeader(t *testing.T) {
	e := newEnv(t)
	accountID, assigned := e.seedAccount(t, "Corralón A")
	unassigned := uuid.New()
	ctx := context.Background()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, 'Sucursal Ajena')`,
		unassigned, accountID); err != nil {
		t.Fatalf("seed second branch: %v", err)
	}

	productID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO product (id, account_id, canonical_name) VALUES ($1, $2, 'Cemento')`,
		productID, accountID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	for _, branchID := range []uuid.UUID{assigned, unassigned} {
		if _, err := e.db.CrossAccount().Exec(ctx,
			`INSERT INTO product_price (account_id, branch_id, product_id, price, valid_from)
			 VALUES ($1, $2, $3, 1000.00, now() - interval '1 day')`,
			accountID, branchID, productID); err != nil {
			t.Fatalf("seed price: %v", err)
		}
	}
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM product_price WHERE product_id = $1`, productID)
		e.mustCleanup(t, `DELETE FROM product WHERE id = $1`, productID)
	})

	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	seller := e.seedUser(t, accountID, domain.UserRoleSeller)
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO user_branch (account_id, user_id, branch_id) VALUES ($1, $2, $3)`,
		accountID, seller.ID, assigned); err != nil {
		t.Fatalf("assign seller: %v", err)
	}

	countPrices := func(t *testing.T, user domain.AppUser) []uuid.UUID {
		t.Helper()
		rec := e.do(t, request{method: http.MethodGet,
			path: fmt.Sprintf("/v1/products/%s/prices", productID), token: e.tokenFor(t, user)})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body)
		}
		var body struct {
			Items []struct {
				BranchID uuid.UUID `json:"branch_id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		branches := make([]uuid.UUID, 0, len(body.Items))
		for _, item := range body.Items {
			branches = append(branches, item.BranchID)
		}
		return branches
	}

	// The admin legitimately compares branches.
	if got := countPrices(t, admin); len(got) != 2 {
		t.Errorf("admin with no branch header sees %v, want both branches", got)
	}

	// The seller, with the header omitted, must see only their own branch.
	got := countPrices(t, seller)
	if len(got) != 1 {
		t.Fatalf("seller with no branch header sees %d rows, want 1: %v", len(got), got)
	}
	if got[0] != assigned {
		t.Errorf("seller sees branch %v, want %v", got[0], assigned)
	}
	if got[0] == unassigned {
		t.Error("a seller read a branch they are not assigned to by omitting X-Branch-Id")
	}
}
