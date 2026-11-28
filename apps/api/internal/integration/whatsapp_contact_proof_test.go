//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

// proofFixture is a branch number customers write to, with two quotes sent by WhatsApp to the same
// customer number before that customer had an account.
type proofFixture struct {
	invited, earlier sendableQuote
	channel          inboundChannel
	phone            string
	invitedToken     string
	earlierToken     string
}

func (e *env) seedProofFixture(t *testing.T, name string) proofFixture {
	t.Helper()
	ctx := context.Background()
	f := proofFixture{phone: "+5491155550291"}
	f.invited = e.seedSendableQuote(t, name, false)
	f.earlier = e.seedSendableQuoteIn(t, f.invited, name+" earlier")
	f.channel = inboundChannel{id: f.invited.draft.RFQ.ChannelID, phoneNumberID: uuid.NewString()}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE channel SET identifier='+54 9 11 4000-0291',
 config=jsonb_build_object('phone_number_id', $2::text) WHERE id=$1`, f.channel.id, f.channel.phoneNumberID); err != nil {
		t.Fatal(err)
	}
	delivery := e.quoteDeliveryService(t, &captureWhatsAppSender{}, &stagedQuoteEmailSender{}, nil)
	tokenFor := func(seed sendableQuote) string {
		t.Helper()
		sent, err := delivery.Send(ctx, seed.tenant, seed.draft.Quote.ID, domain.QuoteDeliveryInput{IdempotencyKey: uuid.New(), Phone: f.phone})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range sent.Deliveries {
			if d.ChannelType == domain.ChannelTypeWhatsApp {
				return d.PublicToken
			}
		}
		t.Fatal("no WhatsApp delivery")
		return ""
	}
	f.earlierToken = tokenFor(f.earlier)
	f.invitedToken = tokenFor(f.invited)
	return f
}

func (e *env) customerPrincipal(t *testing.T, account uuid.UUID, tax string) domain.CustomerPrincipal {
	t.Helper()
	p := domain.CustomerPrincipal{AccountID: account, TaxID: tax}
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT id FROM customer_identity WHERE account_id=$1 AND tax_id=$2`, account, tax).Scan(&p.CustomerID); err != nil {
		t.Fatal(err)
	}
	return p
}

func inboundVerifier(e *env, messenger *captureContactMessenger) *services.ClientContactVerificationService {
	contacts := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository())
	return services.NewClientContactVerificationService(contacts, repository.NewClientContactRepository(), messenger,
		sharedDestinationCaps(config.CustomerAuthConfig{ContactVerificationTTL: 15 * time.Minute,
			ContactVerificationCooldown: time.Minute, ContactVerificationAttempts: 3})).
		WithInboundProofs([]byte("integration-inbound-proof-secret-0123456789"))
}

// TC-10 over WhatsApp: a customer the quote reached only by WhatsApp signs up without typing their
// number, sends «VERIFICAR <code>» from it to the branch, and sees that quote; the quote sent earlier
// to the same number stays hidden. The message becomes no order, no reply and no non-order.
func TestWhatsAppProof_AQuoteLinkProvedByMessageOpensThatQuoteOnly(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	f := e.seedProofFixture(t, "Inbound proof")
	account := f.invited.tenant.AccountID
	host, session := e.registerCustomer(t, account, "20123456786")
	p := e.customerPrincipal(t, account, "20123456786")
	opens := func(token string) int {
		t.Helper()
		return e.do(t, request{method: http.MethodGet, path: "/v1/customer/sites/" + host + "/quote-sends/" + token + "/order", token: session}).Code
	}
	if code := opens(f.invitedToken); code != http.StatusNotFound {
		t.Fatalf("the quote opened before any proof: %d", code)
	}

	messenger := &captureContactMessenger{}
	verifier := inboundVerifier(e, messenger)
	proof, err := verifier.Request(ctx, p, domain.ChannelTypeWhatsApp, "", "/quotes/"+f.invitedToken)
	if err != nil {
		t.Fatalf("Request() = %v", err)
	}
	wantLink := "https://wa.me/5491140000291?text=VERIFICAR%20" + proof.InboundCode
	if len(proof.InboundCode) != 6 || proof.WhatsAppLink != wantLink || len(messenger.messages) != 0 {
		t.Fatalf("proof = %q %q after %d messages, want a code, the branch chat and nothing sent",
			proof.InboundCode, proof.WhatsAppLink, len(messenger.messages))
	}
	again, err := verifier.Request(ctx, p, domain.ChannelTypeWhatsApp, "", "/profile")
	if err != nil || again.ID != proof.ID || again.InboundCode != proof.InboundCode {
		t.Errorf("a second request = %+v, %v; want the live proof again", again, err)
	}
	view, err := verifier.Describe(ctx, p, proof.ID)
	if err != nil || view.Status != domain.ContactChallengePending || view.Code != proof.InboundCode || view.WhatsAppLink != wantLink {
		t.Fatalf("Describe() = %+v, %v; want the pending proof with its code and chat", view, err)
	}

	if _, err := verifier.Confirm(ctx, p, proof.ID, proof.InboundCode); err == nil {
		t.Fatal("the code typed into the portal redeemed a proof only the customer's message may redeem")
	}
	var failures, attempts int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT (SELECT count(*) FROM client_contact_challenge_failure WHERE challenge_id=$1),
 (SELECT attempts FROM client_contact_challenge WHERE id=$1)`, proof.ID).Scan(&failures, &attempts); err != nil {
		t.Fatal(err)
	}
	if failures != 0 || attempts != 0 {
		t.Errorf("a typed code cost the proof %d failures and %d attempts, want none", failures, attempts)
	}

	rfqs := &recordingInboundRFQs{}
	inbound := e.inboundService(t, rfqs).WithContactProofs(verifier)
	sender := strings.TrimPrefix(f.phone, "+")
	message := whatsAppReply(f.channel, "wamid."+uuid.NewString(), sender, " verificar "+proof.InboundCode+" ")
	if err := inbound.Ingest(ctx, []domain.InboundTextMessage{message}); err != nil {
		t.Fatalf("Ingest() = %v", err)
	}
	var reserved int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM inbound_channel_message WHERE channel_id=$1`, f.channel.id).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if messages, _ := e.conversationOf(t, f.invited.draft.Quote.ID); reserved != 0 || len(rfqs.rfqIDs) != 0 || messages != 0 {
		t.Errorf("reserved/orders/replies = %d/%d/%d, want the proof kept out of intake", reserved, len(rfqs.rfqIDs), messages)
	}
	if len(messenger.messages) != 1 || messenger.messages[0].To != f.phone || messenger.messages[0].ChannelID != f.channel.id ||
		!strings.Contains(messenger.messages[0].Body, "verificamos este número") {
		t.Fatalf("answers = %+v, want one fixed confirmation to the sender through the branch number", messenger.messages)
	}
	view, err = verifier.Describe(ctx, p, proof.ID)
	if err != nil || view.Status != domain.ContactChallengeVerified || view.Value != f.phone || view.Code != "" {
		t.Errorf("Describe() after the message = %+v, %v; want the sender's number verified", view, err)
	}

	for label, tc := range map[string]struct {
		token string
		want  int
	}{
		"the quote whose link the proof came from":      {f.invitedToken, http.StatusOK},
		"another quote sent earlier to the same number": {f.earlierToken, http.StatusNotFound},
	} {
		if code := opens(tc.token); code != tc.want {
			t.Errorf("%s: %d, want %d", label, code, tc.want)
		}
	}

	if err := inbound.Ingest(ctx, []domain.InboundTextMessage{message}); err != nil {
		t.Fatalf("redelivered Ingest() = %v", err)
	}
	stranger := whatsAppReply(f.channel, "wamid."+uuid.NewString(), "5491155550299", "VERIFICAR "+proof.InboundCode)
	if err := inbound.Ingest(ctx, []domain.InboundTextMessage{stranger}); err != nil {
		t.Fatalf("stranger Ingest() = %v", err)
	}
	if len(messenger.messages) != 3 || !strings.Contains(messenger.messages[1].Body, "verificamos") ||
		messenger.messages[2].To != "+5491155550299" || !strings.Contains(messenger.messages[2].Body, "no es válido") {
		t.Errorf("answers = %+v, want the redelivery confirmed again and the spent code refused", messenger.messages)
	}
}

// A number another customer of the supplier already verified is refused: the proof stays pending
// and the number never joins this customer's profile.
func TestWhatsAppProof_RefusesANumberVerifiedOnAnotherAccount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	f := e.seedProofFixture(t, "Inbound proof taken")
	account := f.invited.tenant.AccountID
	e.registerCustomer(t, account, "20123456786")
	e.registerCustomer(t, account, "27234567891")
	owner := e.customerPrincipal(t, account, "27234567891")
	p := e.customerPrincipal(t, account, "20123456786")

	messenger := &captureContactMessenger{}
	verifier := inboundVerifier(e, messenger)
	inbound := e.inboundService(t, &recordingInboundRFQs{}).WithContactProofs(verifier)
	sender := strings.TrimPrefix(f.phone, "+")
	owned, err := verifier.Request(ctx, owner, domain.ChannelTypeWhatsApp, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := inbound.Ingest(ctx, []domain.InboundTextMessage{whatsAppReply(f.channel, "wamid."+uuid.NewString(), sender, "VERIFICAR "+owned.InboundCode)}); err != nil {
		t.Fatal(err)
	}

	proof, err := verifier.Request(ctx, p, domain.ChannelTypeWhatsApp, "", "/quotes/"+f.invitedToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := inbound.Ingest(ctx, []domain.InboundTextMessage{whatsAppReply(f.channel, "wamid."+uuid.NewString(), sender, "VERIFICAR "+proof.InboundCode)}); err != nil {
		t.Fatalf("Ingest() = %v", err)
	}
	last := messenger.messages[len(messenger.messages)-1]
	if !strings.Contains(last.Body, "otra cuenta") {
		t.Errorf("answer = %q, want the number refused as verified elsewhere", last.Body)
	}
	view, err := verifier.Describe(ctx, p, proof.ID)
	if err != nil || view.Status != domain.ContactChallengePending {
		t.Errorf("Describe() = %+v, %v; want the proof still pending", view, err)
	}
	var held int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM client_contact cc JOIN client c ON c.account_id=cc.account_id AND c.id=cc.client_id
 WHERE cc.account_id=$1 AND c.tax_id=$2 AND cc.value=$3 AND cc.retired_at IS NULL`, account, p.TaxID, f.phone).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held != 0 {
		t.Errorf("the refused number joined the profile %d times", held)
	}
}
