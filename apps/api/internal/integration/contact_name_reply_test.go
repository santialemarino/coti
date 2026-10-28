//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

// rowInboundRFQs creates the RFQ row a WhatsApp order becomes, without running the pipeline.
type rowInboundRFQs struct {
	e     *env
	mu    sync.Mutex
	count int
}

func (r *rowInboundRFQs) CreateInboundTextDraftForMessage(ctx context.Context, route domain.InboundChannelRoute, in domain.InboundTextRFQDraftInput, rfqID uuid.UUID) (*domain.TextRFQDraft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
	_, err := r.e.db.CrossAccount().Exec(ctx, `INSERT INTO rfq(id,account_id,branch_id,channel_id,raw_text,status) VALUES($1,$2,$3,$4,$5,'RECEIVED')`, rfqID, route.AccountID, route.BranchID, route.ChannelID, in.RawText)
	return &domain.TextRFQDraft{}, err
}

func (r *rowInboundRFQs) created() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func (e *env) promptStatus(t *testing.T, account uuid.UUID, value string) string {
	t.Helper()
	var status string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT status FROM client_contact_outbox WHERE account_id=$1 AND value=$2`, account, value).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (e *env) profileName(t *testing.T, account uuid.UUID) string {
	t.Helper()
	var name string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT name FROM client WHERE account_id=$1 AND merged_into IS NULL`, account).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestContactNameReply_WhatsAppNameCompletesTheProfileAndOrdersStayOrders(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Name reply WhatsApp")
	channel := e.seedWhatsAppInbox(t, account, branch)
	rfqs := &rowInboundRFQs{e: e}
	messenger := &captureContactMessenger{}
	contacts := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository()).WithMessaging(messenger)
	inbound := services.NewWhatsAppInboundService(e.db, services.NewInboundRoutingService(e.db, repository.NewChannelRepository()),
		repository.NewInboundChannelMessageRepository(), nil, rfqs, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).WithContacts(contacts)
	ctx := context.Background()
	sender := "+5491155550101"
	label := "Perfil de WhatsApp"
	send := func(body string) {
		t.Helper()
		message := whatsAppReply(channel, uuid.NewString(), sender, body)
		message.SenderLabel = &label
		if err := inbound.Ingest(ctx, []domain.InboundTextMessage{message}); err != nil {
			t.Fatal(err)
		}
	}

	send("2 bolsas de cemento")
	if rfqs.created() != 1 || len(messenger.messages) != 1 || e.profileName(t, account) != label {
		t.Fatalf("first order: rfqs=%d prompts=%d name=%q", rfqs.created(), len(messenger.messages), e.profileName(t, account))
	}
	prompt := messenger.messages[0]
	if prompt.Purpose != domain.ContactPurposeNameRequest || prompt.Kind != domain.ChannelTypeWhatsApp || strings.Contains(prompt.Body, label) {
		t.Fatalf("prompt = %+v", prompt)
	}

	send("2 bolsas de cemento")
	send("necesito arena")
	if rfqs.created() != 3 || e.promptStatus(t, account, sender) != "SENT" {
		t.Fatalf("short orders were swallowed: rfqs=%d status=%s", rfqs.created(), e.promptStatus(t, account, sender))
	}

	send("Soy Juan Pérez")
	if rfqs.created() != 3 || e.profileName(t, account) != "Juan Pérez" || e.promptStatus(t, account, sender) != "COMPLETED" {
		t.Fatalf("name reply: rfqs=%d name=%q status=%s", rfqs.created(), e.profileName(t, account), e.promptStatus(t, account, sender))
	}
	var replyRFQ, firstRFQ uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT (SELECT rfq_id FROM inbound_channel_message WHERE account_id=$1 AND body='Soy Juan Pérez'),(SELECT rfq_id FROM inbound_channel_message WHERE account_id=$1 ORDER BY created_at,id LIMIT 1)`, account).Scan(&replyRFQ, &firstRFQ); err != nil {
		t.Fatal(err)
	}
	if replyRFQ != firstRFQ {
		t.Fatal("the name reply was not retained on the request it answers")
	}
	send("Pedro Gómez")
	if rfqs.created() != 4 || e.profileName(t, account) != "Juan Pérez" {
		t.Fatal("a name after the prompt was answered must be an ordinary message")
	}
}

func TestContactNameReply_WhatsAppReplyAfterTheWindowIsAnOrder(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Name reply window")
	channel := e.seedWhatsAppInbox(t, account, branch)
	rfqs := &rowInboundRFQs{e: e}
	contacts := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository()).WithMessaging(&captureContactMessenger{})
	inbound := services.NewWhatsAppInboundService(e.db, services.NewInboundRoutingService(e.db, repository.NewChannelRepository()),
		repository.NewInboundChannelMessageRepository(), nil, rfqs, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).WithContacts(contacts)
	ctx := context.Background()
	sender := "+5491155550102"
	if err := inbound.Ingest(ctx, []domain.InboundTextMessage{whatsAppReply(channel, uuid.NewString(), sender, "10 ladrillos")}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE client_contact_outbox SET status_changed_at=now()-interval '25 hours' WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if err := inbound.Ingest(ctx, []domain.InboundTextMessage{whatsAppReply(channel, uuid.NewString(), sender, "Juan Pérez")}); err != nil {
		t.Fatal(err)
	}
	if rfqs.created() != 2 || e.profileName(t, account) == "Juan Pérez" {
		t.Fatalf("a reply outside the 24 h window was taken as the name: rfqs=%d", rfqs.created())
	}
}

func (e *env) emailIntake(t *testing.T, account, branch uuid.UUID, messenger domain.ContactMessenger) (*services.EmailInboundService, domain.InboundChannelRoute, string) {
	t.Helper()
	channel := uuid.New()
	address := uuid.NewString() + "@example.test"
	if _, err := e.db.CrossAccount().Exec(context.Background(), `INSERT INTO channel(id,account_id,branch_id,type,identifier) VALUES($1,$2,$3,'EMAIL',$4)`, channel, account, branch, address); err != nil {
		t.Fatal(err)
	}
	baseURL, _ := url.Parse("http://localhost:8080")
	objects, err := storage.NewLocalStorage(t.TempDir(), baseURL, storage.NewURLSigner([]byte("contact-name-reply-test-secret-at-least-32-bytes"), nil))
	if err != nil {
		t.Fatal(err)
	}
	contacts := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository()).WithMessaging(messenger)
	service := services.NewEmailInboundService(e.db, repository.NewInboundChannelMessageRepository(), repository.NewRFQRepository(), repository.NewRFQAttachmentRepository(), repository.NewChannelRepository(), objects, 1024).WithContacts(contacts)
	return service, domain.InboundChannelRoute{AccountID: account, BranchID: branch, ChannelID: channel, ChannelType: domain.ChannelTypeEmail}, address
}

func TestContactNameReply_EmailReplyWithQuoteAndSignatureSavesTheName(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Name reply email")
	messenger := &captureContactMessenger{}
	service, route, address := e.emailIntake(t, account, branch, messenger)
	ctx := context.Background()
	original := domain.InboundEmailMessage{ExternalMessageID: uuid.NewString(), DestinationIdentifier: address, SenderID: "juan.perez@example.test", SenderLabel: "Juan", Subject: "Pedido", Body: "20 bolsas de cemento", Original: []byte("Original"), InternetMessageID: "<order-1@mail.example.test>"}
	id, err := service.Ingest(ctx, route, original)
	if err != nil {
		t.Fatal(err)
	}
	if len(messenger.messages) != 1 || messenger.messages[0].InReplyTo != original.InternetMessageID || messenger.messages[0].Purpose != domain.ContactPurposeNameRequest {
		t.Fatalf("prompt = %+v", messenger.messages)
	}
	reply := original
	reply.ExternalMessageID = uuid.NewString()
	reply.Subject = "Re: Tu pedido a Name reply email"
	reply.Body = "Juan Pérez\n\nSaludos,\nJuan\n-- \nJuan Pérez | Constructora JP\nTel 11 5555 5555\n\nEl lun, 27 oct 2026 a las 10:00, Name reply email <ventas@example.test>\nescribió:\n> Gracias por escribirnos. Para registrar tus pedidos a tu nombre, respondé solo con tu nombre y apellido.\n"
	reply.Original = []byte("Reply")
	reply.InternetMessageID = "<reply-1@mail.example.test>"
	actual, err := service.Ingest(ctx, route, reply)
	if err != nil || actual != id {
		t.Fatalf("reply routed to %s (%v), want %s", actual, err, id)
	}
	var rfqs int
	if err := e.db.CrossAccount().QueryRow(ctx, `SELECT count(*) FROM rfq WHERE account_id=$1`, account).Scan(&rfqs); err != nil {
		t.Fatal(err)
	}
	if rfqs != 1 || e.profileName(t, account) != "Juan Pérez" || len(messenger.messages) != 1 {
		t.Fatalf("email name reply: rfqs=%d name=%q prompts=%d", rfqs, e.profileName(t, account), len(messenger.messages))
	}
}

func TestContactNameReply_AutomatedMailNeverGetsTheQuestion(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Name reply backscatter")
	messenger := &captureContactMessenger{}
	service, route, address := e.emailIntake(t, account, branch, messenger)
	for _, message := range []domain.InboundEmailMessage{
		{SenderID: "no-reply@proveedor.test", Body: "Su factura está disponible"},
		{SenderID: "MAILER-DAEMON@mx.example.test", Body: "Delivery failed"},
		{SenderID: "news@corralon-amigo.test", Body: "Ofertas de la semana", Automated: true},
	} {
		message.ExternalMessageID = uuid.NewString()
		message.DestinationIdentifier = address
		message.Original = []byte("Automated")
		if _, err := service.Ingest(context.Background(), route, message); err != nil {
			t.Fatal(err)
		}
	}
	if len(messenger.messages) != 0 {
		t.Fatalf("automated mail got %d name prompts", len(messenger.messages))
	}
}

func TestContactNameReply_StalePromptInDeliveryIsClaimedAgain(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Name reply lease")
	ctx := context.Background()
	messenger := &captureContactMessenger{}
	service := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository()).WithMessaging(messenger).WithPromptLease(5 * time.Minute)
	var channel uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO channel(account_id,branch_id,type,identifier) VALUES($1,$2,'EMAIL',$3) RETURNING id`, account, branch, uuid.NewString()+"@example.test").Scan(&channel); err != nil {
		t.Fatal(err)
	}
	route := domain.InboundChannelRoute{AccountID: account, BranchID: branch, ChannelID: channel, ChannelType: domain.ChannelTypeEmail}
	value := "stalled@example.test"
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO client_contact_outbox(account_id,branch_id,channel_id,kind,value,purpose,status,status_changed_at) VALUES($1,$2,$3,'EMAIL',$4,'REQUEST_NAME','SENDING',now()-interval '1 minute')`, account, branch, channel, value); err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteInbound(ctx, route, domain.ChannelTypeEmail, value, domain.ContactReplyContext{}); err != nil {
		t.Fatal(err)
	}
	if len(messenger.messages) != 0 {
		t.Fatal("a delivery inside its lease was claimed twice")
	}
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE client_contact_outbox SET status_changed_at=now()-interval '10 minutes' WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if err := service.CompleteInbound(ctx, route, domain.ChannelTypeEmail, value, domain.ContactReplyContext{}); err != nil {
		t.Fatal(err)
	}
	if len(messenger.messages) != 1 || e.promptStatus(t, account, value) != "SENT" {
		t.Fatalf("stale delivery: prompts=%d status=%s", len(messenger.messages), e.promptStatus(t, account, value))
	}
}

func TestContactNameReply_EmailPromptAnswersAsTheSupplierMailbox(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Corralón Norte")
	ctx := context.Background()
	mailbox := uuid.NewString() + "@norte.test"
	var channel uuid.UUID
	if err := e.db.CrossAccount().QueryRow(ctx, `INSERT INTO channel(account_id,branch_id,type,identifier) VALUES($1,$2,'EMAIL',$3) RETURNING id`, account, branch, mailbox).Scan(&channel); err != nil {
		t.Fatal(err)
	}
	mailer := &captureMailer{}
	delivery := services.NewClientContactDeliveryService(e.db, repository.NewClientContactRepository(), mailer, nil, nil, config.CustomerAuthConfig{})
	if _, err := delivery.SendContact(ctx, domain.ContactMessage{AccountID: account, BranchID: branch, ChannelID: channel, Purpose: domain.ContactPurposeNameRequest, Kind: domain.ChannelTypeEmail, To: "cliente@example.test", Body: "fixed", InReplyTo: "<order@mail.test>"}); err != nil {
		t.Fatal(err)
	}
	if _, err := delivery.SendContact(ctx, domain.ContactMessage{AccountID: account, Purpose: domain.ContactPurposeVerification, Kind: domain.ChannelTypeEmail, To: "cliente@example.test", Body: "link"}); err != nil {
		t.Fatal(err)
	}
	if len(mailer.sent) != 2 {
		t.Fatalf("sent %d", len(mailer.sent))
	}
	prompt, proof := mailer.sent[0], mailer.sent[1]
	if prompt.Subject != "Tu pedido a Corralón Norte" || prompt.FromName != "Corralón Norte" || prompt.ReplyTo != mailbox || prompt.ReplyToName != "Corralón Norte — Corralón Norte Centro" || prompt.InReplyTo != "<order@mail.test>" || strings.Contains(prompt.Subject, "Coti") {
		t.Fatalf("prompt mail = %+v", prompt)
	}
	if proof.Subject == prompt.Subject || proof.ReplyTo != "" || proof.FromName != "" {
		t.Fatalf("verification mail = %+v", proof)
	}
	if _, err := delivery.SendContact(ctx, domain.ContactMessage{AccountID: account, Kind: domain.ChannelTypeEmail, To: "cliente@example.test", Body: "x"}); err == nil {
		t.Fatal("a contact mail without a purpose was sent")
	}
}
