//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

const (
	routedPhone = "+5491155550177"
	routedEmail = "counter-buyer@example.test"
)

// mergedRouting is a provisional profile with WhatsApp routedPhone and email routedEmail, merged into
// the registered profile of customer 20123456786 when that customer proved routedPhone.
type mergedRouting struct {
	e         *env
	account   uuid.UUID
	route     domain.InboundChannelRoute
	service   *services.ClientContactService
	messenger *captureContactMessenger
	host      string
	token     string
	source    uuid.UUID
	target    uuid.UUID
}

func (e *env) mergedRouting(t *testing.T) *mergedRouting {
	t.Helper()
	ctx := context.Background()
	account, branch := e.seedAccount(t, "Merge routing")
	store := repository.NewClientContactRepository()
	profiles := repository.NewClientRepository()
	m := &mergedRouting{e: e, account: account, messenger: &captureContactMessenger{}}
	m.service = services.NewClientContactService(e.db, profiles, store)
	m.service.WithMessaging(m.messenger)
	m.route = domain.InboundChannelRoute{AccountID: account, BranchID: branch, ChannelID: e.historyChannel(t, account, branch, "EMAIL"), ChannelType: domain.ChannelTypeEmail}
	name := "Counter buyer"
	err := e.db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q repository.Querier) error {
		c, err := profiles.Create(ctx, q, account, domain.NewClient{Name: &name})
		if err != nil {
			return err
		}
		m.source = c.ID
		return store.Save(ctx, q, account, c.ID, []domain.ClientContact{
			{Kind: domain.ChannelTypeWhatsApp, Value: routedPhone, Primary: true},
			{Kind: domain.ChannelTypeEmail, Value: routedEmail, Primary: true},
		}, "CHANNEL")
	})
	if err != nil {
		t.Fatal(err)
	}
	m.host, m.token = e.registerCustomer(t, account, "20123456786")
	e.verifyCustomerContact(t, account, "20123456786", domain.ChannelTypeWhatsApp, routedPhone)
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT merged_into FROM client WHERE account_id=$1 AND id=$2`, account, m.source).Scan(&m.target); err != nil {
		t.Fatalf("provisional profile was not merged: %v", err)
	}
	return m
}

// deliver records one inbound email from value the way the email ingress does, and returns its request.
func (m *mergedRouting) deliver(t *testing.T, value string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var rfq, message uuid.UUID
	if err := m.e.db.CrossAccount().QueryRow(ctx, `INSERT INTO rfq(account_id,branch_id,channel_id,raw_text) VALUES($1,$2,$3,'Routed order') RETURNING id`, m.account, m.route.BranchID, m.route.ChannelID).Scan(&rfq); err != nil {
		t.Fatal(err)
	}
	if err := m.e.db.CrossAccount().QueryRow(ctx, `INSERT INTO inbound_channel_message(account_id,branch_id,channel_id,rfq_id,external_message_id,sender_id,body,payload,provider_received_at) VALUES($1,$2,$3,$4,$5,$6,'Routed order','{}',$7) RETURNING id`,
		m.account, m.route.BranchID, m.route.ChannelID, rfq, uuid.NewString(), value, time.Now()).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if err := m.e.db.InTenantTx(ctx, m.route.Tenant(), func(q repository.Querier) error {
		return m.service.RecordInbound(ctx, q, m.route, message, domain.ChannelTypeEmail, value, "Routed order", domain.ClientContactHints{})
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.service.CompleteInbound(ctx, m.route, domain.ChannelTypeEmail, value, domain.ContactReplyContext{}); err != nil {
		t.Fatal(err)
	}
	return rfq
}

func (m *mergedRouting) requestClient(t *testing.T, rfq uuid.UUID) uuid.UUID {
	t.Helper()
	var client *uuid.UUID
	if err := m.e.db.CrossAccount().QueryRow(context.Background(), `SELECT client_id FROM rfq WHERE account_id=$1 AND id=$2`, m.account, rfq).Scan(&client); err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("the request was not attached to a profile")
	}
	return *client
}

func (m *mergedRouting) liveProfiles(t *testing.T) int {
	t.Helper()
	var count int
	if err := m.e.db.CrossAccount().QueryRow(context.Background(), `SELECT count(*) FROM client WHERE account_id=$1 AND merged_into IS NULL`, m.account).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (m *mergedRouting) principal(t *testing.T, taxID string) domain.CustomerPrincipal {
	t.Helper()
	p := domain.CustomerPrincipal{AccountID: m.account, TaxID: taxID}
	if err := m.e.db.CrossAccount().QueryRow(context.Background(), `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, m.account, taxID).Scan(&p.CustomerID); err != nil {
		t.Fatal(err)
	}
	return p
}

func (m *mergedRouting) setContacts(t *testing.T, p domain.CustomerPrincipal, contacts []domain.ClientContact) {
	t.Helper()
	if err := m.service.UpdateProfile(context.Background(), p, domain.ClientContactUpdate{Name: "Registered Name", Contacts: contacts}); err != nil {
		t.Fatal(err)
	}
}

func TestMergeRouting_MergedContactRoutesToTheRegisteredProfile(t *testing.T) {
	e := newEnv(t)
	m := e.mergedRouting(t)
	before := m.liveProfiles(t)

	routed := m.deliver(t, routedEmail)
	if got := m.requestClient(t, routed); got != m.target {
		t.Fatalf("routed request went to %s, want the registered profile %s", got, m.target)
	}
	if after := m.liveProfiles(t); after != before {
		t.Fatalf("live profiles %d -> %d: the merged contact started a new provisional profile", before, after)
	}
	if len(m.messenger.messages) != 0 {
		t.Fatalf("a merged contact was asked its name: %d prompts", len(m.messenger.messages))
	}
}

func TestMergeRouting_RoutingGivesNoOwnershipOrHistory(t *testing.T) {
	e := newEnv(t)
	m := e.mergedRouting(t)
	ctx := context.Background()
	routed := m.deliver(t, routedEmail)

	var owned int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM customer_contact_ownership o JOIN client_contact c ON c.account_id=o.account_id AND c.id=o.contact_id WHERE o.account_id=$1 AND c.value=$2`, m.account, routedEmail).Scan(&owned); err != nil {
		t.Fatal(err)
	}
	if owned != 0 {
		t.Fatalf("routing created %d ownership rows", owned)
	}
	profile, err := m.service.GetProfile(ctx, m.principal(t, "20123456786"))
	if err != nil {
		t.Fatal(err)
	}
	shown := false
	for _, c := range profile.Contacts {
		if c.Value == routedEmail {
			shown = true
			if c.Verified {
				t.Fatal("a routing-only contact shows as verified")
			}
		}
	}
	if !shown {
		t.Fatalf("the routing contact left the profile: %+v", profile.Contacts)
	}

	if !e.customerOrderIDs(t, m.host, m.token)[routed] {
		t.Fatal("the customer does not see an order routed to the profile it proved")
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE customer_contact_ownership SET includes_profile_history=FALSE WHERE account_id=$1`, m.account); err != nil {
		t.Fatal(err)
	}
	if e.customerOrderIDs(t, m.host, m.token)[routed] {
		t.Fatal("a routing-only contact opened its order outside the profile's history")
	}
}

func TestMergeRouting_VerifiedOwnershipElsewhereTakesTheContact(t *testing.T) {
	e := newEnv(t)
	m := e.mergedRouting(t)
	ctx := context.Background()

	e.registerCustomer(t, m.account, "30712345671")
	e.verifyCustomerContact(t, m.account, "30712345671", domain.ChannelTypeEmail, routedEmail)
	var other uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT id FROM client WHERE account_id=$1 AND tax_id='30712345671'`, m.account).Scan(&other); err != nil {
		t.Fatal(err)
	}
	before := m.liveProfiles(t)

	routed := m.deliver(t, routedEmail)
	if got := m.requestClient(t, routed); got != other {
		t.Fatalf("request went to %s, want the profile that proved the contact %s", got, other)
	}
	if after := m.liveProfiles(t); after != before || len(m.messenger.messages) != 0 {
		t.Fatalf("live profiles %d -> %d, prompts %d", before, after, len(m.messenger.messages))
	}
	var merged *uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT merged_into FROM client WHERE account_id=$1 AND id=$2`, m.account, m.target).Scan(&merged); err != nil {
		t.Fatal(err)
	}
	if merged != nil {
		t.Fatal("proving the contact elsewhere merged the first customer's profile")
	}
}

func TestMergeRouting_UnrelatedAndRemovedContactsDoNotRoute(t *testing.T) {
	e := newEnv(t)
	m := e.mergedRouting(t)
	ctx := context.Background()
	p := m.principal(t, "20123456786")
	declared := "declared-only@example.test"
	m.setContacts(t, p, []domain.ClientContact{
		{Kind: domain.ChannelTypeWhatsApp, Value: routedPhone, Primary: true},
		{Kind: domain.ChannelTypeEmail, Value: routedEmail, Primary: true},
		{Kind: domain.ChannelTypeEmail, Value: declared},
	})

	before := m.liveProfiles(t)
	typed := m.deliver(t, declared)
	if got := m.requestClient(t, typed); got == m.target {
		t.Fatal("a contact the customer only typed routed to its profile")
	}
	if m.liveProfiles(t) != before+1 || len(m.messenger.messages) != 1 {
		t.Fatal("an unrelated sender did not get its own provisional profile and name prompt")
	}

	m.setContacts(t, p, []domain.ClientContact{{Kind: domain.ChannelTypeWhatsApp, Value: routedPhone, Primary: true}})
	removed := m.deliver(t, routedEmail)
	if got := m.requestClient(t, removed); got == m.target {
		t.Fatal("a merged contact the customer removed still routes to its profile")
	}
	if m.liveProfiles(t) != before+2 {
		t.Fatal("a removed merged contact did not start its own provisional profile")
	}

	var verifiedProfiles int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM client_contact WHERE account_id=$1 AND value=$2 AND verified_at IS NOT NULL AND retired_at IS NULL`, m.account, routedEmail).Scan(&verifiedProfiles); err != nil {
		t.Fatal(err)
	}
	if verifiedProfiles != 1 {
		t.Fatalf("live verified rows for the removed contact=%d", verifiedProfiles)
	}
}
