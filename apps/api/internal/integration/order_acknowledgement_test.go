//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const (
	acknowledgement         = "Recibimos tu mensaje, un vendedor te responde a la brevedad."
	acknowledgementWithName = acknowledgement + " Para registrar tus pedidos a tu nombre, respondé solo con tu nombre y apellido."
)

func (b *nameReplyInbox) deliver(t *testing.T, externalID, sender, body string) {
	t.Helper()
	message := whatsAppReply(b.channel, externalID, sender, body)
	if err := b.service.Ingest(context.Background(), []domain.InboundTextMessage{message}); err != nil {
		t.Fatal(err)
	}
}

func TestOrderAcknowledgement_EachWhatsAppOrderIsAcknowledgedOnce(t *testing.T) {
	e := newEnv(t)
	box := e.nameReplyInbox(t, "Acknowledgement WhatsApp", true)
	sender := "+5491155550301"

	// A new sender's first order gets one message: the acknowledgement and the name question.
	first := uuid.NewString()
	box.deliver(t, first, sender, "20 bolsas de cemento")
	if len(box.messenger.messages) != 1 || box.messenger.messages[0].Body != acknowledgementWithName ||
		box.messenger.messages[0].Purpose != domain.ContactPurposeNameRequest {
		t.Fatalf("first order sent %+v, want the acknowledgement merged with the name question", box.messenger.messages)
	}
	// Meta delivers the same message again: nothing more goes out.
	box.deliver(t, first, sender, "20 bolsas de cemento")
	if len(box.messenger.messages) != 1 {
		t.Fatalf("a retried delivery sent %d messages, want the first one alone", len(box.messenger.messages))
	}
	// The next order of a known sender is acknowledged alone.
	box.deliver(t, uuid.NewString(), sender, "10 ladrillos")
	if len(box.messenger.messages) != 2 || box.messenger.messages[1].Body != acknowledgement ||
		box.messenger.messages[1].Purpose != domain.ContactPurposeAcknowledgement {
		t.Fatalf("second order sent %+v, want the acknowledgement alone", box.messenger.messages[1:])
	}
	var acknowledged int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM rfq WHERE account_id=$1 AND acknowledged_at IS NOT NULL`, box.account).Scan(&acknowledged); err != nil {
		t.Fatal(err)
	}
	if acknowledged != 2 || box.rfqs.created() != 2 {
		t.Fatalf("acknowledged %d of %d orders, want both", acknowledged, box.rfqs.created())
	}
}

func TestOrderAcknowledgement_AQuoteReplyIsNotANewOrder(t *testing.T) {
	e := newEnv(t)
	box := e.nameReplyInbox(t, "Acknowledgement conversation", true)
	sender := "+5491155550302"
	box.deliver(t, uuid.NewString(), sender, "10 ladrillos")
	e.seedSentQuote(t, sentQuote{accountID: box.account, branchID: box.branch, channel: box.channel,
		destination: sender, status: domain.QuoteStatusSent, sentAgo: time.Hour, expiresIn: 24 * time.Hour})
	sent := len(box.messenger.messages)

	box.deliver(t, uuid.NewString(), sender, "Confirmo")
	if len(box.messenger.messages) != sent {
		t.Fatalf("a reply to a quote sent %+v, want nothing", box.messenger.messages[sent:])
	}
}

func TestOrderAcknowledgement_EmailOrdersAreAcknowledgedInTheThread(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Acknowledgement email")
	messenger := &captureContactMessenger{}
	service, route, address := e.emailIntake(t, account, branch, messenger)
	ctx := context.Background()

	order := domain.InboundEmailMessage{ExternalMessageID: uuid.NewString(), DestinationIdentifier: address,
		SenderID: "compras@obra.test", Subject: "Pedido", Body: "20 bolsas de cemento",
		Original: []byte("Order"), InternetMessageID: "<order-ack@mail.example.test>"}
	if _, err := service.Ingest(ctx, route, order); err != nil {
		t.Fatal(err)
	}
	if len(messenger.messages) != 1 || messenger.messages[0].Body != acknowledgementWithName ||
		messenger.messages[0].InReplyTo != order.InternetMessageID {
		t.Fatalf("email order sent %+v, want the merged acknowledgement in the thread", messenger.messages)
	}
	next := order
	next.ExternalMessageID, next.InternetMessageID = uuid.NewString(), "<order-ack-2@mail.example.test>"
	next.Body = "10 ladrillos"
	if _, err := service.Ingest(ctx, route, next); err != nil {
		t.Fatal(err)
	}
	bounce := domain.InboundEmailMessage{ExternalMessageID: uuid.NewString(), DestinationIdentifier: address,
		SenderID: "no-reply@proveedor.test", Body: "Su factura está disponible", Original: []byte("Automated")}
	if _, err := service.Ingest(ctx, route, bounce); err != nil {
		t.Fatal(err)
	}
	if len(messenger.messages) != 2 || messenger.messages[1].Body != acknowledgement ||
		messenger.messages[1].Purpose != domain.ContactPurposeAcknowledgement {
		t.Fatalf("messages %+v, want one acknowledgement per order and none for automated mail", messenger.messages)
	}
}
