//go:build integration

package integration

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/secrets"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

type fakeGmailAuthorizationProvider struct {
	grant domain.GmailGrant
}

func (f *fakeGmailAuthorizationProvider) AuthorizationURL(state, verifier string) string {
	return "https://google.example/consent?state=" + url.QueryEscape(state)
}

func (f *fakeGmailAuthorizationProvider) Exchange(_ context.Context, code, verifier string) (domain.GmailGrant, error) {
	if code != "valid-code" || len(verifier) < 43 {
		return domain.GmailGrant{}, errors.New("invalid authorization")
	}
	return f.grant, nil
}

func TestGmailAuthorization_BindsMailboxToBranchAndConsumesState(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Gmail authorization")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM gmail_authorization_state WHERE account_id = $1`, accountID)
		e.mustCleanup(t, `DELETE FROM gmail_mailbox WHERE account_id = $1`, accountID)
	})
	address := uuid.NewString() + "@example.test"
	channelID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO channel (id, account_id, branch_id, type, identifier)
		 VALUES ($1, $2, $3, 'EMAIL', $4)`, channelID, accountID, branchID, address); err != nil {
		t.Fatal(err)
	}
	cipher, err := secrets.NewAESGCM([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeGmailAuthorizationProvider{grant: domain.GmailGrant{Address: address,
		HistoryID: "100", RefreshToken: "private-refresh-token"}}
	store := repository.NewGmailMailboxRepository()
	service := services.NewGmailAuthorizationService(e.db, store, repository.NewChannelRepository(),
		repository.NewUserRepository(), provider, cipher, 10*time.Minute, true)
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID, UserID: admin.ID,
		Role: domain.UserRoleAdmin, EmailVerified: true}
	start := func() string {
		t.Helper()
		consent, err := service.Start(context.Background(), tenant, channelID)
		if err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(consent)
		if err != nil {
			t.Fatal(err)
		}
		return u.Query().Get("state")
	}
	state := start()
	if err := service.Complete(context.Background(), state, "valid-code"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := service.Complete(context.Background(), state, "valid-code"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("replayed state error = %v", err)
	}
	var mailbox *domain.GmailMailbox
	if err := e.db.InTenantTx(context.Background(), tenant, func(q repository.Querier) error {
		var err error
		mailbox, err = store.GetConnected(context.Background(), q, accountID, branchID, channelID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if mailbox.Address != address || mailbox.HistoryID != "100" || !mailbox.InitialSyncPending ||
		strings.Contains(mailbox.RefreshTokenCiphertext, provider.grant.RefreshToken) {
		t.Fatalf("stored mailbox has an unexpected state: %+v", mailbox)
	}
	if decrypted, err := cipher.Open(mailbox.RefreshTokenCiphertext); err != nil || decrypted != provider.grant.RefreshToken {
		t.Fatalf("stored credential did not decrypt: %v", err)
	}
	provider.grant.Address = "different@example.test"
	if err := service.Complete(context.Background(), start(), "valid-code"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("wrong mailbox error = %v", err)
	}
	provider.grant.Address = address
	state = start()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE app_user SET session_epoch = session_epoch + 1 WHERE account_id = $1 AND id = $2`,
		accountID, admin.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Complete(context.Background(), state, "valid-code"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("revoked administrator error = %v", err)
	}
}
