package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// fakeSignupInvites keeps invites by hash and records what was created.
type fakeSignupInvites struct {
	byHash  map[string]domain.SignupInvite
	created *domain.SignupInvite
}

func (f *fakeSignupInvites) GetByHash(_ context.Context, _ repository.Querier, hash string) (*domain.SignupInvite, error) {
	invite, ok := f.byHash[hash]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &invite, nil
}

func (f *fakeSignupInvites) GetByHashForUpdate(ctx context.Context, q repository.Querier, hash string) (*domain.SignupInvite, error) {
	return f.GetByHash(ctx, q, hash)
}

func (f *fakeSignupInvites) Create(_ context.Context, _ repository.Querier, in domain.NewSignupInvite, hash string, expiresAt time.Time) (*domain.SignupInvite, error) {
	f.created = &domain.SignupInvite{ID: uuid.New(), TokenHash: hash, Email: in.Email, Note: in.Note, ExpiresAt: expiresAt}
	return f.created, nil
}

func (f *fakeSignupInvites) MarkUsed(context.Context, repository.Querier, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}

var signupNow = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func TestSignupInviteService_CreateStoresOnlyTheHashAndLinksTheSignupScreen(t *testing.T) {
	invites := &fakeSignupInvites{}
	service := NewSignupInviteService(stubQueueDB{}, invites,
		config.SignupConfig{InviteTTL: 14 * 24 * time.Hour},
		config.WebConfig{BackofficeURL: "https://coti.test/"}, func() time.Time { return signupNow })
	service.newSecret = func() (string, error) { return "raw-token", nil }
	email := "compras@corralon.test"

	link, invite, err := service.Create(context.Background(), domain.NewSignupInvite{Email: &email})
	if err != nil {
		t.Fatalf("Create() = %v, want no error", err)
	}
	if link != "https://coti.test/signup?invite=raw-token" {
		t.Errorf("link = %q, want the signup screen carrying the raw token", link)
	}
	// sha256("raw-token"), computed outside the code under test.
	if invite.TokenHash != "34d328009b123fbbb0dc93f18b3e6de1ecf7b1a5783c33dff7ffe1926f09e943" {
		t.Errorf("stored hash = %q, want the SHA-256 of the token", invite.TokenHash)
	}
	if want := time.Date(2026, 12, 4, 12, 0, 0, 0, time.UTC); !invite.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", invite.ExpiresAt, want)
	}
	if invite.Email == nil || *invite.Email != email {
		t.Errorf("Email = %v, want %q kept beside the invite", invite.Email, email)
	}
}

func TestSignupInviteService_CheckRefusesWhatRegistrationWould(t *testing.T) {
	used := signupNow.Add(-time.Hour)
	invites := &fakeSignupInvites{byHash: map[string]domain.SignupInvite{
		hashToken("fresh"):   {ExpiresAt: signupNow.Add(time.Hour)},
		hashToken("used"):    {ExpiresAt: signupNow.Add(time.Hour), UsedAt: &used},
		hashToken("expired"): {ExpiresAt: signupNow.Add(-time.Second)},
	}}
	service := NewSignupInviteService(stubQueueDB{}, invites, config.SignupConfig{InviteTTL: time.Hour},
		config.WebConfig{}, func() time.Time { return signupNow })

	cases := []struct {
		token string
		want  domain.ErrorCode
	}{
		{"fresh", ""},
		{"used", domain.CodeSignupInviteUsed},
		{"expired", domain.CodeSignupInviteExpired},
		{"unknown", domain.CodeSignupInviteInvalid},
		{"   ", domain.CodeSignupInviteInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.token, func(t *testing.T) {
			err := service.Check(context.Background(), tc.token)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Check(%q) = %v, want nil", tc.token, err)
				}
				return
			}
			if !errors.Is(err, domain.ErrForbidden) || domain.CodeOf(err) != tc.want {
				t.Fatalf("Check(%q) = %v (%s), want %s", tc.token, err, domain.CodeOf(err), tc.want)
			}
		})
	}
}

// registerDB counts the transactions registration opened, and refuses each one.
type registerDB struct {
	fakeDB
	opened int
}

var errRegisterDBReached = errors.New("registration reached the database")

func (d *registerDB) AdminTx(context.Context) (pgx.Tx, error) {
	d.opened++
	return nil, errRegisterDBReached
}

func TestAccountService_RegisterFollowsTheSignupSwitch(t *testing.T) {
	cases := []struct {
		name    string
		enabled bool
		invite  string
		closed  bool
	}{
		{"closed without an invite", false, "", true},
		{"closed with a blank invite", false, "   ", true},
		{"closed with an invite", false, "raw-token", false},
		{"open without an invite", true, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &registerDB{}
			service := (&AccountService{db: db, policy: domain.PasswordPolicy{MinLength: 8}}).
				WithSignup(config.SignupConfig{Enabled: tc.enabled}, &fakeSignupInvites{})

			_, _, err := service.Register(context.Background(), domain.Signup{
				AccountName: "Corralón", BranchName: "Casa Central", AdminName: "Ana",
				AdminEmail: "ana@corralon.test", AdminPassword: "Coti-1234-larga", InviteToken: tc.invite,
			})

			if tc.closed {
				if domain.CodeOf(err) != domain.CodeSignupClosed || !errors.Is(err, domain.ErrForbidden) || db.opened != 0 {
					t.Fatalf("err = %v, transactions = %d, want SIGNUP_CLOSED before any", err, db.opened)
				}
				return
			}
			if !errors.Is(err, errRegisterDBReached) {
				t.Fatalf("err = %v, want registration to go on to the database", err)
			}
		})
	}
}
