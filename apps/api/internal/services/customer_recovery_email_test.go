package services

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestCustomerRecoveryEmailPhoneSignupRequiresProofAndInvalidatesOldLinks(t *testing.T) {
	svc, store, mail := customerIdentityFixture()
	ctx := context.Background()
	login, err := svc.Register(ctx, store.site.Hostname, domain.CustomerRegistration{Name: "Juan Perez", TaxID: "20123456786", Password: "Customer-pass123!", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeWhatsApp, Value: "+5491112345678", Primary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := svc.GetRecoveryEmail(ctx, login.Principal)
	if err != nil || initial.Email != nil || initial.Verified || len(mail.sent) != 0 {
		t.Fatalf("phone signup recovery: %+v %v", initial, err)
	}
	foreign := login.Principal
	foreign.AccountID = uuid.New()
	if err := svc.UpdateRecoveryEmail(ctx, store.site.Hostname, foreign, "attacker@example.test"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("foreign scope: %v", err)
	}
	if err := svc.UpdateRecoveryEmail(ctx, store.site.Hostname, login.Principal, "recovery@example.test"); err != nil {
		t.Fatal(err)
	}
	if len(mail.sent) != 1 {
		t.Fatal("verification not sent")
	}
	if err := svc.ForgotCustomerPassword(ctx, store.site.Hostname, login.Principal.TaxID, "recovery@example.test"); err != nil || len(mail.sent) != 1 {
		t.Fatal("unverified recovery allowed")
	}
	link, _ := url.Parse(mail.sent[0].ActionURL)
	if err := svc.VerifyCustomerEmail(ctx, store.site.Hostname, link.Query().Get("token")); err != nil {
		t.Fatal(err)
	}
	verified, err := svc.GetRecoveryEmail(ctx, login.Principal)
	if err != nil || !verified.Verified || verified.Email == nil || *verified.Email != "recovery@example.test" {
		t.Fatalf("proof: %+v %v", verified, err)
	}
	if err := svc.UpdateRecoveryEmail(ctx, store.site.Hostname, login.Principal, "recovery@example.test"); err != nil || len(mail.sent) != 1 {
		t.Fatal("same verified address was reset")
	}
	if err := svc.ForgotCustomerPassword(ctx, store.site.Hostname, login.Principal.TaxID, "recovery@example.test"); err != nil || len(mail.sent) != 2 {
		t.Fatal("verified recovery failed")
	}
	reset, _ := url.Parse(mail.sent[1].ActionURL)
	if err := svc.UpdateRecoveryEmail(ctx, store.site.Hostname, login.Principal, "replacement@example.test"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetCustomerPassword(ctx, store.site.Hostname, reset.Query().Get("token"), "Replacement-pass123!"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("old reset survived address change: %v", err)
	}
	pending, _ := url.Parse(mail.sent[2].ActionURL)
	if err := svc.UpdateRecoveryEmail(ctx, store.site.Hostname, login.Principal, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.VerifyCustomerEmail(ctx, store.site.Hostname, pending.Query().Get("token")); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("removed email proof accepted: %v", err)
	}
	cleared, err := svc.GetRecoveryEmail(ctx, login.Principal)
	if err != nil || cleared.Email != nil || cleared.Verified {
		t.Fatalf("clear recovery: %+v %v", cleared, err)
	}
}

func TestCustomerRecoveryEmailReportsAuthenticatedDeliveryFailure(t *testing.T) {
	svc, store, mail := customerIdentityFixture()
	ctx := context.Background()
	login, err := svc.Register(ctx, store.site.Hostname, domain.CustomerRegistration{Name: "Juan Perez", TaxID: "20123456786", Password: "Customer-pass123!", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: "commercial@example.test", Primary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	unavailable := errors.New("mail unavailable")
	mail.sendErr = unavailable
	if err := svc.UpdateRecoveryEmail(ctx, store.site.Hostname, login.Principal, "recovery@example.test"); !errors.Is(err, unavailable) {
		t.Fatalf("hidden authenticated delivery failure: %v", err)
	}
	state, err := svc.GetRecoveryEmail(ctx, login.Principal)
	if err != nil || state.Verified || state.Email == nil {
		t.Fatalf("failed delivery verification: %+v %v", state, err)
	}
	link, _ := url.Parse(mail.sent[0].ActionURL)
	if err := svc.VerifyCustomerEmail(ctx, store.site.Hostname, link.Query().Get("token")); err != nil {
		t.Fatal(err)
	}
	if err := svc.ForgotCustomerPassword(ctx, store.site.Hostname, login.Principal.TaxID, "recovery@example.test"); err != nil {
		t.Fatalf("anonymous recovery exposed delivery failure: %v", err)
	}
}
