//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// storedMessage seeds one received request and the inbound message carrying it, for an observation to
// point at.
func (e *env) storedMessage(t *testing.T, account, branch uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var channel, rfq, id uuid.UUID
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO channel(account_id,branch_id,type) VALUES($1,$2,'WEBAPP') ON CONFLICT DO NOTHING`, account, branch); err != nil {
		t.Fatal(err)
	}
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM channel WHERE account_id=$1 AND branch_id=$2 AND type='WEBAPP' AND identifier IS NULL`, account, branch).Scan(&channel); err != nil {
		t.Fatal(err)
	}
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO rfq(account_id,branch_id,channel_id,raw_text) VALUES($1,$2,$3,'Pedido') RETURNING id`, account, branch, channel).Scan(&rfq); err != nil {
		t.Fatal(err)
	}
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO inbound_channel_message(account_id,branch_id,channel_id,rfq_id,external_message_id,sender_id,body,payload,provider_received_at) VALUES($1,$2,$3,$4,$5,'sender','Pedido','{}',now()) RETURNING id`,
		account, branch, channel, rfq, uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// A registration link needs the supplier's portal hostname, and by WhatsApp it needs the customer to
// have written to the branch within the last 24 hours; each refusal carries its own code.
func TestClientContacts_RegistrationLinkRefusesWithoutPortalOrWindow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	account, branch := e.seedAccount(t, "Registration link window")
	digits := fmt.Sprintf("54911%08d", rand.IntN(100000000))
	name, phone, email := "Juan Perez", "+"+digits, "window-"+uuid.NewString()+"@example.test"
	var clientID uuid.UUID
	if err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q repository.Querier) error {
		c, err := repository.NewClientRepository().Create(ctx, q, account, domain.NewClient{Name: &name, Phone: &phone, Email: &email})
		if err == nil {
			clientID = c.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	messenger := &captureContactMessenger{}
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository()).WithMessaging(messenger)
	tenant := domain.Tenant{AccountID: account, BranchID: branch, Role: domain.UserRoleAdmin}

	err := service.SendRegistrationLink(ctx, tenant, clientID, domain.ChannelTypeEmail)
	if !errors.Is(err, domain.ErrNotConfigured) || domain.CodeOf(err) != domain.CodePortalNotConfigured {
		t.Fatalf("no hostname: err = %v (code %s), want %s", err, domain.CodeOf(err), domain.CodePortalNotConfigured)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE account SET public_hostname=$2 WHERE id=$1`, account, "window-"+account.String()+".coti.test"); err != nil {
		t.Fatal(err)
	}

	channel := e.seedWhatsAppInbox(t, account, branch)
	wrote := func(ago time.Duration) {
		t.Helper()
		var rfq uuid.UUID
		at := time.Now().Add(-ago)
		if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO rfq(account_id,branch_id,channel_id,raw_text,received_at) VALUES($1,$2,$3,'Pedido',$4) RETURNING id`, account, branch, channel.id, at).Scan(&rfq); err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO inbound_channel_message(account_id,branch_id,channel_id,rfq_id,external_message_id,sender_id,body,payload,provider_received_at) VALUES($1,$2,$3,$4,$5,$6,'Pedido','{}',$7)`,
			account, branch, channel.id, rfq, uuid.NewString(), digits, at); err != nil {
			t.Fatal(err)
		}
	}
	closed := func(when string) {
		t.Helper()
		err := service.SendRegistrationLink(ctx, tenant, clientID, domain.ChannelTypeWhatsApp)
		if !errors.Is(err, domain.ErrInvalidInput) || domain.CodeOf(err) != domain.CodeWhatsAppWindowClosed {
			t.Fatalf("%s: err = %v (code %s), want %s", when, err, domain.CodeOf(err), domain.CodeWhatsAppWindowClosed)
		}
	}
	closed("never wrote")
	wrote(25 * time.Hour)
	closed("wrote 25 hours ago")
	if len(messenger.messages) != 0 {
		t.Fatalf("a refused link reached the provider: %+v", messenger.messages)
	}
	wrote(time.Hour)
	if err := service.SendRegistrationLink(ctx, tenant, clientID, domain.ChannelTypeWhatsApp); err != nil {
		t.Fatalf("wrote an hour ago: %v", err)
	}
	if len(messenger.messages) != 1 || messenger.messages[0].To != phone || messenger.messages[0].Kind != domain.ChannelTypeWhatsApp {
		t.Fatalf("link within the window: %+v", messenger.messages)
	}
}

// An observation's message belongs to its own account: a message of another account, or none at
// all, is refused. 00069 points a portal request's observation, which held the request id, at its
// message, and unlinks the observations that still point nowhere.
func TestClientContacts_ObservationMessageBelongsToItsAccount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	account, branch := e.seedAccount(t, "Observation message")
	other, otherBranch := e.seedAccount(t, "Observation foreign message")
	channel := e.seedWhatsAppInbox(t, other, otherBranch)
	message := func(account, branch, channel uuid.UUID, external string) uuid.UUID {
		t.Helper()
		var rfq, id uuid.UUID
		if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO rfq(account_id,branch_id,channel_id,raw_text) VALUES($1,$2,$3,'Pedido') RETURNING id`, account, branch, channel).Scan(&rfq); err != nil {
			t.Fatal(err)
		}
		if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO inbound_channel_message(account_id,branch_id,channel_id,rfq_id,external_message_id,sender_id,body,payload,provider_received_at) VALUES($1,$2,$3,$4,$5,'5491100000000','Pedido','{}',now()) RETURNING id`,
			account, branch, channel, rfq, external).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	foreign := message(other, otherBranch, channel.id, uuid.NewString())
	request := uuid.New()
	portal := message(account, branch, e.historyChannel(t, account, branch, "EMAIL"), request.String())
	insert := `INSERT INTO client_contact_observation(account_id,branch_id,message_id,kind,value,original_text,status) VALUES($1,$2,$3,'WHATSAPP','+5491100000000','Pedido','RECORDED')`
	for name, message := range map[string]uuid.UUID{"another account's message": foreign, "a missing message": uuid.New()} {
		if _, err := e.db.CrossAccount().Exec(ctx, insert, account, branch, message); err == nil {
			t.Fatalf("an observation took %s", name)
		}
	}

	migration, err := os.ReadFile("../../migrations/00069_contact_guards.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down, ok := strings.Cut(string(migration), "-- +goose Down")
	if !ok {
		t.Fatal("migration has no Down section")
	}
	markers := strings.NewReplacer("-- +goose StatementBegin", "", "-- +goose StatementEnd", "")
	tx, err := e.db.AdminTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, markers.Replace(down)); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	var orphan, kept, requested uuid.UUID
	if err := tx.QueryRow(ctx, insert+` RETURNING id`, account, branch, foreign).Scan(&orphan); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, insert+` RETURNING id`, account, branch, request).Scan(&requested); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, insert+` RETURNING id`, other, otherBranch, foreign).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, markers.Replace(up)); err != nil {
		t.Fatalf("migrate up over an orphan: %v", err)
	}
	var orphanMessage, keptMessage, requestedMessage *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT (SELECT message_id FROM client_contact_observation WHERE id=$1),(SELECT message_id FROM client_contact_observation WHERE id=$2),(SELECT message_id FROM client_contact_observation WHERE id=$3)`,
		orphan, kept, requested).Scan(&orphanMessage, &keptMessage, &requestedMessage); err != nil {
		t.Fatal(err)
	}
	if orphanMessage != nil || keptMessage == nil || *keptMessage != foreign || requestedMessage == nil || *requestedMessage != portal {
		t.Fatalf("after 00069: orphan %v, kept %v, portal request %v; want nil, %s and %s", orphanMessage, keptMessage, requestedMessage, foreign, portal)
	}
}
