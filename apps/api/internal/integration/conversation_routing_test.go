//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

const conversationSender = "5491155550101"

// recordingInboundRFQs stands in for the RFQ pipeline, which needs a model: a message that starts
// an order is only counted.
type recordingInboundRFQs struct {
	mu     sync.Mutex
	rfqIDs []uuid.UUID
}

func (r *recordingInboundRFQs) CreateInboundTextDraftForMessage(_ context.Context,
	_ domain.InboundChannelRoute, _ domain.InboundTextRFQDraftInput, rfqID uuid.UUID,
) (*domain.TextRFQDraft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rfqIDs = append(r.rfqIDs, rfqID)
	return &domain.TextRFQDraft{}, nil
}

func (r *recordingInboundRFQs) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rfqIDs)
}

type inboundChannel struct {
	id            uuid.UUID
	phoneNumberID string
}

// sentQuote is a quote and the send that reached destination over a channel.
type sentQuote struct {
	accountID, branchID uuid.UUID
	channel             inboundChannel
	destination         string
	status              domain.QuoteStatus
	sentAgo             time.Duration
	expiresIn           time.Duration
	archived            bool
}

func (e *env) inboundService(t *testing.T, rfqs *recordingInboundRFQs) *services.WhatsAppInboundService {
	t.Helper()
	conversations := services.NewConversationService(repository.NewQuoteRepository(),
		repository.NewQuoteSendRepository(), repository.NewQuoteMessageRepository(),
		repository.NewMessageBatchRepository(), config.ConversationConfig{
			WindowReset: 2 * time.Minute, WindowMax: 10 * time.Minute, ContextMessages: 10})
	return services.NewWhatsAppInboundService(e.db,
		services.NewInboundRoutingService(e.db, repository.NewChannelRepository()),
		repository.NewInboundChannelMessageRepository(), nil, rfqs, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil))).WithConversations(conversations).WithContacts(services.NewClientContactService(e.db, repository.NewClientRepository(), repository.NewClientContactRepository()))
}

// seedWhatsAppInbox opens a WhatsApp channel whose provider number id routes webhooks; the
// account's teardown removes it.
func (e *env) seedWhatsAppInbox(t *testing.T, accountID, branchID uuid.UUID) inboundChannel {
	t.Helper()
	channel := inboundChannel{id: uuid.New(), phoneNumberID: uuid.NewString()}
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO channel (id, account_id, branch_id, type, identifier, is_active, config)
		 VALUES ($1, $2, $3, 'WHATSAPP', $4::text, TRUE, jsonb_build_object('phone_number_id', $4::text))`,
		channel.id, accountID, branchID, channel.phoneNumberID); err != nil {
		t.Fatalf("seed WhatsApp inbox: %v", err)
	}
	return channel
}

// seedSentQuote writes an order, its quote, a frozen version and the send; the account's teardown
// removes them.
func (e *env) seedSentQuote(t *testing.T, in sentQuote) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	rfqID, quoteID, versionID := uuid.New(), uuid.New(), uuid.New()
	var archivedAt *time.Time
	if in.archived {
		now := time.Now()
		archivedAt = &now
	}
	for _, seed := range []struct {
		what string
		sql  string
		args []any
	}{
		{"rfq", `INSERT INTO rfq (id, account_id, branch_id, channel_id, raw_text, status)
		         VALUES ($1, $2, $3, $4, 'pedido', 'GENERATED')`,
			[]any{rfqID, in.accountID, in.branchID, in.channel.id}},
		{"quote", `INSERT INTO quote (id, account_id, number, branch_id, rfq_id, current_status, archived_at)
		           VALUES ($1::uuid, $2, (('x'||substr(replace($1::uuid::text,'-',''),1,15))::bit(60)::bigint),
		                   $3, $4, $5, $6)`,
			[]any{quoteID, in.accountID, in.branchID, rfqID, in.status, archivedAt}},
		{"version", `INSERT INTO quote_version (id, account_id, quote_id, version_number, is_immutable, frozen_at)
		             VALUES ($1, $2, $3, 1, TRUE, now())`,
			[]any{versionID, in.accountID, quoteID}},
		{"current version", `UPDATE quote SET current_version_id = $2 WHERE id = $1`,
			[]any{quoteID, versionID}},
		{"send", `INSERT INTO quote_send (account_id, version_id, channel_id, destination, format,
		                                  sent_at, expires_at, tracking_status)
		          VALUES ($1, $2, $3, $4, 'WEBAPP_LINK', now() - make_interval(secs => $5),
		                  now() + make_interval(secs => $6), 'SENT')`,
			[]any{in.accountID, versionID, in.channel.id, in.destination, in.sentAgo.Seconds(),
				in.expiresIn.Seconds()}},
	} {
		if _, err := e.db.CrossAccount().Exec(ctx, seed.sql, seed.args...); err != nil {
			t.Fatalf("seed %s: %v", seed.what, err)
		}
	}
	return quoteID
}

func whatsAppReply(channel inboundChannel, externalID, sender, body string) domain.InboundTextMessage {
	return domain.InboundTextMessage{ExternalMessageID: externalID,
		DestinationIdentifier: channel.phoneNumberID, SenderID: sender, Body: body,
		Payload: []byte(`{"object":"whatsapp_business_account"}`)}
}

// conversationOf counts the client messages on a quote and the inbound events pointing at them.
func (e *env) conversationOf(t *testing.T, quoteID uuid.UUID) (messages, reserved int) {
	t.Helper()
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(DISTINCT message.id), count(inbound.id)
		 FROM quote_message message
		 LEFT JOIN inbound_channel_message inbound ON inbound.quote_message_id = message.id
		 WHERE message.quote_id = $1 AND message.author_type = 'CLIENT'`,
		quoteID).Scan(&messages, &reserved); err != nil {
		t.Fatalf("read conversation: %v", err)
	}
	return messages, reserved
}

func (e *env) openWindowsOf(t *testing.T, quoteID uuid.UUID) []domain.MessageBatch {
	t.Helper()
	rows, err := e.db.CrossAccount().Query(context.Background(),
		`SELECT id, opened_at, closes_at FROM message_batch WHERE quote_id = $1 AND status = 'OPEN'`,
		quoteID)
	if err != nil {
		t.Fatalf("read windows: %v", err)
	}
	defer rows.Close()
	var batches []domain.MessageBatch
	for rows.Next() {
		var batch domain.MessageBatch
		if err := rows.Scan(&batch.ID, &batch.OpenedAt, &batch.ClosesAt); err != nil {
			t.Fatalf("scan window: %v", err)
		}
		batches = append(batches, batch)
	}
	return batches
}

func TestConversationRouting_InboundRepliesJoinTheLiveQuote(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Conversa")
	channel := e.seedWhatsAppInbox(t, accountID, branchID)
	live := func(destination string, status domain.QuoteStatus) sentQuote {
		return sentQuote{accountID: accountID, branchID: branchID, channel: channel,
			destination: destination, status: status, sentAgo: time.Hour, expiresIn: 24 * time.Hour}
	}

	for _, test := range []struct {
		name   string
		seed   func(t *testing.T) sentQuote
		sender string
		joins  bool
	}{
		{name: "sent quote", sender: conversationSender, joins: true,
			seed: func(*testing.T) sentQuote { return live("+5491155550101", domain.QuoteStatusSent) }},
		{name: "change requested quote", sender: conversationSender, joins: true,
			seed: func(*testing.T) sentQuote {
				return live("+5491155550101", domain.QuoteStatusChangeRequested)
			}},
		{name: "sent without the 9, sender with it", sender: "5491155550102", joins: true,
			seed: func(*testing.T) sentQuote { return live("+541155550102", domain.QuoteStatusSent) }},
		{name: "sent with the 9, sender without it", sender: "541155550103", joins: true,
			seed: func(*testing.T) sentQuote { return live("+5491155550103", domain.QuoteStatusSent) }},
		{name: "expired send", sender: conversationSender,
			seed: func(*testing.T) sentQuote {
				quote := live("+5491155550101", domain.QuoteStatusSent)
				quote.expiresIn = -time.Minute
				return quote
			}},
		{name: "archived quote", sender: conversationSender,
			seed: func(*testing.T) sentQuote {
				quote := live("+5491155550101", domain.QuoteStatusSent)
				quote.archived = true
				return quote
			}},
		{name: "accepted quote", sender: conversationSender,
			seed: func(*testing.T) sentQuote { return live("+5491155550101", domain.QuoteStatusAccepted) }},
		{name: "a sender no quote can have reached", sender: "54911-abc",
			seed: func(*testing.T) sentQuote { return live("+5491155550101", domain.QuoteStatusSent) }},
		{name: "another sender", sender: "5491155559999",
			seed: func(*testing.T) sentQuote { return live("+5491155550101", domain.QuoteStatusSent) }},
		{name: "sent from another channel of the branch", sender: conversationSender,
			seed: func(t *testing.T) sentQuote {
				quote := live("+5491155550101", domain.QuoteStatusSent)
				quote.channel = e.seedWhatsAppInbox(t, accountID, branchID)
				return quote
			}},
		{name: "sent from another branch", sender: conversationSender,
			seed: func(t *testing.T) sentQuote {
				quote := live("+5491155550101", domain.QuoteStatusSent)
				quote.branchID = e.seedBranch(t, accountID, "Sucursal Norte "+uuid.NewString())
				quote.channel = e.seedWhatsAppInbox(t, accountID, quote.branchID)
				return quote
			}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Earlier cases' quotes stay behind, archived, so only this case's quote is live.
			if _, err := e.db.CrossAccount().Exec(context.Background(),
				`UPDATE quote SET archived_at = now() WHERE account_id = $1`, accountID); err != nil {
				t.Fatalf("archive earlier quotes: %v", err)
			}
			quoteID := e.seedSentQuote(t, test.seed(t))
			rfqs := &recordingInboundRFQs{}

			err := e.inboundService(t, rfqs).Ingest(context.Background(), []domain.InboundTextMessage{
				whatsAppReply(channel, "wamid."+uuid.NewString(), test.sender, "sumale 5 bolsas")})
			if err != nil {
				t.Fatalf("Ingest() = %v, want no error", err)
			}
			messages, reserved := e.conversationOf(t, quoteID)
			if test.joins {
				if messages != 1 || reserved != 1 || rfqs.count() != 0 || len(e.openWindowsOf(t, quoteID)) != 1 {
					t.Errorf("messages/reserved/rfqs = %d/%d/%d, want the reply in the quote's open window",
						messages, reserved, rfqs.count())
				}
				return
			}
			if messages != 0 || rfqs.count() != 1 {
				t.Errorf("messages/rfqs = %d/%d, want a new order instead", messages, rfqs.count())
			}
		})
	}
}

func TestConversationRouting_NewestLiveSendWinsAndHintsTheOthers(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Dos Obras")
	channel := e.seedWhatsAppInbox(t, accountID, branchID)
	quote := func(sentAgo time.Duration) uuid.UUID {
		return e.seedSentQuote(t, sentQuote{accountID: accountID, branchID: branchID,
			channel: channel, destination: "+5491155550101", status: domain.QuoteStatusSent,
			sentAgo: sentAgo, expiresIn: 24 * time.Hour})
	}
	older := quote(3 * time.Hour)
	newest := quote(time.Hour)
	middle := quote(2 * time.Hour)

	if err := e.inboundService(t, &recordingInboundRFQs{}).Ingest(context.Background(),
		[]domain.InboundTextMessage{whatsAppReply(channel, "wamid.newest", conversationSender, "dale")}); err != nil {
		t.Fatalf("Ingest() = %v, want no error", err)
	}
	for quoteID, want := range map[uuid.UUID]int{older: 0, middle: 0, newest: 1} {
		if messages, _ := e.conversationOf(t, quoteID); messages != want {
			t.Errorf("quote %s messages = %d, want %d", quoteID, messages, want)
		}
	}
	var hint int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT other_live_quotes FROM quote_message WHERE quote_id = $1`, newest).Scan(&hint); err != nil {
		t.Fatalf("read hint: %v", err)
	}
	if hint != 2 {
		t.Errorf("other_live_quotes = %d, want 2", hint)
	}
}

func TestConversationRouting_WindowOpensExtendsAndIgnoresTheSeller(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Ventana")
	channel := e.seedWhatsAppInbox(t, accountID, branchID)
	quoteID := e.seedSentQuote(t, sentQuote{accountID: accountID, branchID: branchID,
		channel: channel, destination: "+5491155550101", status: domain.QuoteStatusSent,
		sentAgo: time.Hour, expiresIn: 24 * time.Hour})
	service := e.inboundService(t, &recordingInboundRFQs{})
	ingest := func(externalID string) {
		t.Helper()
		if err := service.Ingest(context.Background(), []domain.InboundTextMessage{
			whatsAppReply(channel, externalID, conversationSender, "otra cosa")}); err != nil {
			t.Fatalf("Ingest(%s) = %v, want no error", externalID, err)
		}
	}

	ingest("wamid.window-1")
	opened := e.openWindowsOf(t, quoteID)
	if len(opened) != 1 || opened[0].ClosesAt.Sub(opened[0].OpenedAt) != 2*time.Minute {
		t.Fatalf("windows = %+v, want one closing two minutes after it opened", opened)
	}
	ingest("wamid.window-2")
	extended := e.openWindowsOf(t, quoteID)
	if len(extended) != 1 || extended[0].ID != opened[0].ID ||
		!extended[0].ClosesAt.After(opened[0].ClosesAt) {
		t.Fatalf("windows = %+v, want the same window closing later than %s", extended, opened[0].ClosesAt)
	}

	conversations := services.NewConversationService(repository.NewQuoteRepository(),
		repository.NewQuoteSendRepository(), repository.NewQuoteMessageRepository(),
		repository.NewMessageBatchRepository(), config.ConversationConfig{
			WindowReset: time.Hour, WindowMax: 2 * time.Hour})
	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}
	for _, author := range []domain.MessageAuthorType{domain.MessageAuthorSeller, domain.MessageAuthorSystem} {
		if err := e.db.InTenantTx(context.Background(), tenant, func(q repository.Querier) error {
			_, err := conversations.AppendMessage(context.Background(), q, tenant,
				domain.NewQuoteMessage{QuoteID: quoteID, AuthorType: author, Body: "te paso precio"})
			return err
		}); err != nil {
			t.Fatalf("AppendMessage(%s) = %v, want no error", author, err)
		}
	}
	after := e.openWindowsOf(t, quoteID)
	if len(after) != 1 || !after[0].ClosesAt.Equal(extended[0].ClosesAt) {
		t.Errorf("windows = %+v, want the seller and the system leaving %s alone", after, extended[0].ClosesAt)
	}
}

func TestConversationRouting_ARetriedWebhookJoinsOnce(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Reintento")
	channel := e.seedWhatsAppInbox(t, accountID, branchID)
	quoteID := e.seedSentQuote(t, sentQuote{accountID: accountID, branchID: branchID,
		channel: channel, destination: "+5491155550101", status: domain.QuoteStatusSent,
		sentAgo: time.Hour, expiresIn: 24 * time.Hour})
	rfqs := &recordingInboundRFQs{}
	service := e.inboundService(t, rfqs)

	for range 3 {
		if err := service.Ingest(context.Background(), []domain.InboundTextMessage{
			whatsAppReply(channel, "wamid.retried", conversationSender, "sumale arena")}); err != nil {
			t.Fatalf("Ingest() = %v, want no error", err)
		}
	}
	if messages, reserved := e.conversationOf(t, quoteID); messages != 1 || reserved != 1 || rfqs.count() != 0 {
		t.Errorf("messages/reserved/rfqs = %d/%d/%d, want one joined message", messages, reserved, rfqs.count())
	}
}

func TestConversationRouting_ConcurrentRepliesShareOneWindow(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Simultaneo")
	channel := e.seedWhatsAppInbox(t, accountID, branchID)
	quoteID := e.seedSentQuote(t, sentQuote{accountID: accountID, branchID: branchID,
		channel: channel, destination: "+5491155550101", status: domain.QuoteStatusSent,
		sentAgo: time.Hour, expiresIn: 24 * time.Hour})
	service := e.inboundService(t, &recordingInboundRFQs{})

	const replies = 8
	var wg sync.WaitGroup
	errs := make(chan error, replies)
	for i := range replies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- service.Ingest(context.Background(), []domain.InboundTextMessage{
				whatsAppReply(channel, "wamid.burst-"+uuid.NewString(), conversationSender,
					"mensaje "+string(rune('a'+i)))})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Ingest() = %v, want no error", err)
		}
	}
	if messages, _ := e.conversationOf(t, quoteID); messages != replies {
		t.Errorf("messages = %d, want %d", messages, replies)
	}
	var batches int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM message_batch WHERE quote_id = $1`, quoteID).Scan(&batches); err != nil {
		t.Fatalf("count windows: %v", err)
	}
	if batches != 1 {
		t.Errorf("windows = %d, want one shared by every reply", batches)
	}
}

// Another account sent a live quote to the same phone; a message on this account's number never
// reaches it.
func TestConversationRouting_NeverCrossesAccounts(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Propio")
	channel := e.seedWhatsAppInbox(t, accountID, branchID)
	strangerID, strangerBranch := e.seedAccount(t, "Corralon Ajeno")
	strangerChannel := e.seedWhatsAppInbox(t, strangerID, strangerBranch)
	strangerQuote := e.seedSentQuote(t, sentQuote{accountID: strangerID, branchID: strangerBranch,
		channel: strangerChannel, destination: "+5491155550101", status: domain.QuoteStatusSent,
		sentAgo: time.Hour, expiresIn: 24 * time.Hour})
	rfqs := &recordingInboundRFQs{}

	if err := e.inboundService(t, rfqs).Ingest(context.Background(), []domain.InboundTextMessage{
		whatsAppReply(channel, "wamid.own", conversationSender, "hola")}); err != nil {
		t.Fatalf("Ingest() = %v, want no error", err)
	}
	if messages, _ := e.conversationOf(t, strangerQuote); messages != 0 || rfqs.count() != 1 {
		t.Errorf("stranger messages/own rfqs = %d/%d, want 0/1", messages, rfqs.count())
	}

	// The candidate read on the owner pool, where RLS is off, still keeps to the named account.
	tx, err := e.db.AdminTx(context.Background())
	if err != nil {
		t.Fatalf("AdminTx() = %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	candidates, err := repository.NewQuoteSendRepository().ListLiveConversations(context.Background(),
		tx, accountID, strangerBranch, strangerChannel.id, []string{"+5491155550101"}, time.Now())
	if err != nil || len(candidates) != 0 {
		t.Errorf("ListLiveConversations(other account) = %v, %v; want none", candidates, err)
	}
}

// A send on this branch's channel for a quote another branch owns is not a conversation here. The
// quote lock is also branch-scoped, so the candidate read is asserted directly as well.
func TestConversationRouting_KeepsToTheChannelsBranch(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Dos Sucursales")
	channel := e.seedWhatsAppInbox(t, accountID, branchID)
	otherBranch := e.seedBranch(t, accountID, "Sucursal Sur")
	quoteID := e.seedSentQuote(t, sentQuote{accountID: accountID, branchID: otherBranch,
		channel: channel, destination: "+5491155550101", status: domain.QuoteStatusSent,
		sentAgo: time.Hour, expiresIn: 24 * time.Hour})
	rfqs := &recordingInboundRFQs{}

	if err := e.inboundService(t, rfqs).Ingest(context.Background(), []domain.InboundTextMessage{
		whatsAppReply(channel, "wamid.branch", conversationSender, "hola")}); err != nil {
		t.Fatalf("Ingest() = %v, want no error", err)
	}
	if messages, _ := e.conversationOf(t, quoteID); messages != 0 || rfqs.count() != 1 {
		t.Errorf("messages/rfqs = %d/%d, want a new order instead", messages, rfqs.count())
	}

	tenant := domain.Tenant{AccountID: accountID, BranchID: branchID}
	var candidates []domain.ConversationCandidate
	if err := e.db.InTenantTx(context.Background(), tenant, func(q repository.Querier) error {
		var err error
		candidates, err = repository.NewQuoteSendRepository().ListLiveConversations(
			context.Background(), q, accountID, branchID, channel.id, []string{"+5491155550101"},
			time.Now())
		return err
	}); err != nil || len(candidates) != 0 {
		t.Errorf("ListLiveConversations(channel's branch) = %v, %v; want none", candidates, err)
	}
}
