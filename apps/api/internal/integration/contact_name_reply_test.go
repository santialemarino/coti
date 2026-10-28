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

func (e *env) contactName(t *testing.T, account uuid.UUID, value string) string {
	t.Helper()
	var name string
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT c.name FROM client c JOIN client_contact cc ON cc.account_id=c.account_id AND cc.client_id=c.id WHERE c.account_id=$1 AND cc.value=$2`, account, value).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

// nameReplyInbox is a WhatsApp intake with live quote conversations, contact onboarding and RFQ rows.
type nameReplyInbox struct {
	e         *env
	account   uuid.UUID
	branch    uuid.UUID
	channel   inboundChannel
	rfqs      *rowInboundRFQs
	messenger *captureContactMessenger
	service   *services.WhatsAppInboundService
}

func (e *env) nameReplyInbox(t *testing.T, name string, messaging bool) *nameReplyInbox {
	t.Helper()
	account, branch := e.seedAccount(t, name)
	box := &nameReplyInbox{e: e, account: account, branch: branch, channel: e.seedWhatsAppInbox(t, account, branch), rfqs: &rowInboundRFQs{e: e}, messenger: &captureContactMessenger{}}
	contacts := services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository())
	if messaging {
		contacts.WithMessaging(box.messenger)
	}
	conversations := services.NewConversationService(repository.NewQuoteRepository(), repository.NewQuoteSendRepository(), repository.NewQuoteMessageRepository(),
		repository.NewMessageBatchRepository(), config.ConversationConfig{WindowReset: 2 * time.Minute, WindowMax: 10 * time.Minute, ContextMessages: 10})
	box.service = services.NewWhatsAppInboundService(e.db, services.NewInboundRoutingService(e.db, repository.NewChannelRepository()),
		repository.NewInboundChannelMessageRepository(), nil, box.rfqs, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).WithConversations(conversations).WithContacts(contacts)
	return box
}

func (b *nameReplyInbox) send(t *testing.T, sender, body string) {
	t.Helper()
	label := "Perfil de WhatsApp"
	message := whatsAppReply(b.channel, uuid.NewString(), sender, body)
	message.SenderLabel = &label
	if err := b.service.Ingest(context.Background(), []domain.InboundTextMessage{message}); err != nil {
		t.Fatal(err)
	}
}

func TestContactNameReply_WhatsAppNameCompletesTheProfileAndOrdersStayOrders(t *testing.T) {
	e := newEnv(t)
	box := e.nameReplyInbox(t, "Name reply WhatsApp", true)
	ordering, naming := "+5491155550101", "+5491155550103"

	box.send(t, ordering, "2 bolsas de cemento")
	if box.rfqs.created() != 1 || len(box.messenger.messages) != 1 || e.contactName(t, box.account, ordering) != "Perfil de WhatsApp" {
		t.Fatalf("first order: rfqs=%d prompts=%d", box.rfqs.created(), len(box.messenger.messages))
	}
	prompt := box.messenger.messages[0]
	if prompt.Purpose != domain.ContactPurposeNameRequest || prompt.Kind != domain.ChannelTypeWhatsApp || strings.Contains(prompt.Body, "Perfil") {
		t.Fatalf("prompt = %+v", prompt)
	}
	box.send(t, ordering, "2 bolsas de cemento")
	if box.rfqs.created() != 2 || e.promptStatus(t, box.account, ordering) != "SENT" {
		t.Fatalf("a short order was swallowed: rfqs=%d status=%s", box.rfqs.created(), e.promptStatus(t, box.account, ordering))
	}
	// The order sent after the prompt moved the exchange on, so a name now is an ordinary message.
	box.send(t, ordering, "Juan Pérez")
	if box.rfqs.created() != 3 || e.contactName(t, box.account, ordering) == "Juan Pérez" {
		t.Fatal("a name after other activity was taken as the reply")
	}

	box.send(t, naming, "necesito arena")
	box.send(t, naming, "Soy Ana María de la Torre")
	if box.rfqs.created() != 4 || e.contactName(t, box.account, naming) != "Ana María de la Torre" || e.promptStatus(t, box.account, naming) != "COMPLETED" {
		t.Fatalf("name reply: rfqs=%d name=%q status=%s", box.rfqs.created(), e.contactName(t, box.account, naming), e.promptStatus(t, box.account, naming))
	}
	var replyRFQ, firstRFQ uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(), `SELECT (SELECT rfq_id FROM inbound_channel_message WHERE account_id=$1 AND body='Soy Ana María de la Torre'),(SELECT rfq_id FROM inbound_channel_message WHERE account_id=$1 AND body='necesito arena')`, box.account).Scan(&replyRFQ, &firstRFQ); err != nil {
		t.Fatal(err)
	}
	if replyRFQ != firstRFQ {
		t.Fatal("the name reply was not retained on the request it answers")
	}
	box.send(t, naming, "Pedro Gómez")
	if box.rfqs.created() != 5 || e.contactName(t, box.account, naming) != "Ana María de la Torre" {
		t.Fatal("a name after the prompt was answered must be an ordinary message")
	}
}

func TestContactNameReply_WhatsAppQuoteReplyStaysInTheConversation(t *testing.T) {
	e := newEnv(t)
	box := e.nameReplyInbox(t, "Name reply conversation", true)
	sender := "+5491155550104"
	box.send(t, sender, "10 ladrillos")
	// Sent before the prompt, so only the conversation, not the activity check, keeps the name out.
	quoteID := e.seedSentQuote(t, sentQuote{accountID: box.account, branchID: box.branch, channel: box.channel,
		destination: sender, status: domain.QuoteStatusSent, sentAgo: time.Hour, expiresIn: 24 * time.Hour})
	box.send(t, sender, "Juan Pérez")
	box.send(t, sender, "Confirmo")
	messages, _ := e.conversationOf(t, quoteID)
	if messages != 2 || box.rfqs.created() != 1 || e.promptStatus(t, box.account, sender) != "SENT" || e.contactName(t, box.account, sender) == "Confirmo" {
		t.Fatalf("quote reply: conversation=%d rfqs=%d status=%s name=%q", messages, box.rfqs.created(), e.promptStatus(t, box.account, sender), e.contactName(t, box.account, sender))
	}
}

func TestContactNameReply_WhatsAppNameIsRefusedAfterAQuoteOrForACatalogWord(t *testing.T) {
	e := newEnv(t)
	box := e.nameReplyInbox(t, "Name reply refusals", true)
	ctx := context.Background()
	if _, err := e.db.CrossAccount().Exec(ctx, `INSERT INTO product(account_id,canonical_name) VALUES($1,'Adhesivo Kerakoll Flex')`, box.account); err != nil {
		t.Fatal(err)
	}
	quoted, catalog := "+5491155550105", "+5491155550106"
	box.send(t, quoted, "10 ladrillos")
	// An archived quote no longer takes the conversation, but sending it was still activity.
	e.seedSentQuote(t, sentQuote{accountID: box.account, branchID: box.branch, channel: box.channel,
		destination: quoted, status: domain.QuoteStatusSent, expiresIn: 24 * time.Hour, archived: true})
	box.send(t, quoted, "Juan Pérez")
	box.send(t, catalog, "10 ladrillos")
	box.send(t, catalog, "Kerakoll Gris")
	if box.rfqs.created() != 4 || e.promptStatus(t, box.account, quoted) != "SENT" || e.promptStatus(t, box.account, catalog) != "SENT" {
		t.Fatalf("refused replies: rfqs=%d", box.rfqs.created())
	}
}

func TestContactNameReply_WhatsAppReplyNeedsADeliveredPromptInsideTheWindow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	unsent := e.nameReplyInbox(t, "Name reply unsent", false)
	unsent.send(t, "+5491155550107", "10 ladrillos")
	unsent.send(t, "+5491155550107", "Juan Pérez")
	if unsent.rfqs.created() != 2 || e.promptStatus(t, unsent.account, "+5491155550107") != "PENDING" {
		t.Fatalf("a reply to a prompt never delivered was taken as the name: rfqs=%d", unsent.rfqs.created())
	}

	late := e.nameReplyInbox(t, "Name reply window", true)
	late.send(t, "+5491155550102", "10 ladrillos")
	if _, err := e.db.CrossAccount().Exec(ctx, `UPDATE client_contact_outbox SET status_changed_at=now()-interval '25 hours' WHERE account_id=$1`, late.account); err != nil {
		t.Fatal(err)
	}
	late.send(t, "+5491155550102", "Juan Pérez")
	if late.rfqs.created() != 2 || e.contactName(t, late.account, "+5491155550102") == "Juan Pérez" {
		t.Fatalf("a reply outside the 24 h window was taken as the name: rfqs=%d", late.rfqs.created())
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

func TestContactNameReply_AnAutoReplyIsNeverTheName(t *testing.T) {
	e := newEnv(t)
	account, branch := e.seedAccount(t, "Name reply auto-reply")
	messenger := &captureContactMessenger{}
	service, route, address := e.emailIntake(t, account, branch, messenger)
	ctx := context.Background()
	order := domain.InboundEmailMessage{ExternalMessageID: uuid.NewString(), DestinationIdentifier: address, SenderID: "compras@constructora.test", Body: "20 bolsas de cemento", Original: []byte("Order")}
	id, err := service.Ingest(ctx, route, order)
	if err != nil || len(messenger.messages) != 1 {
		t.Fatalf("order: %v, prompts=%d", err, len(messenger.messages))
	}
	away := order
	away.ExternalMessageID = uuid.NewString()
	away.Body = "Fuera de la oficina"
	away.Automated = true
	actual, err := service.Ingest(ctx, route, away)
	if err != nil || actual == id || e.promptStatus(t, account, "compras@constructora.test") != "SENT" || e.profileName(t, account) == "Fuera de la oficina" {
		t.Fatalf("auto-reply was taken as the name: %v %v", actual == id, err)
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
