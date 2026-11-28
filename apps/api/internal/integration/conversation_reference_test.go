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
	changeOnQuote27 = "Sobre la cotización COT-000027 que me enviaron, necesito un cambio: en lugar de 3 " +
		"bolsas de Cemento Avellaneda de 50 kg, quiero 5 bolsas. Mantené los 2 baldes de Hidrófugo de " +
		"20 kg. Por favor actualizá esa misma cotización y enviámela nuevamente. Gracias."
	separateOrder = "Hola, necesito cotizar un pedido nuevo: 5 bolsas de Cemento Avellaneda de 50 kg y 2 " +
		"baldes de Hidrófugo de 20 kg. Gracias."
)

// A message naming one of the sender's live quotes joins that quote, not the newest send.
func TestConversationRouting_AQuoteReferenceWinsOverTheNewestSend(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralon Referencias")
	channel := e.seedWhatsAppInbox(t, accountID, branchID)
	quote := func(sentAgo time.Duration) uuid.UUID {
		return e.seedSentQuote(t, sentQuote{accountID: accountID, branchID: branchID,
			channel: channel, destination: "+5491155550101", status: domain.QuoteStatusSent,
			sentAgo: sentAgo, expiresIn: 24 * time.Hour})
	}
	older, newest := quote(2*time.Hour), quote(time.Hour)
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`UPDATE quote SET number = 27 WHERE id = $1`, older); err != nil {
		t.Fatalf("number the older quote: %v", err)
	}
	service := e.inboundService(t, &recordingInboundRFQs{})

	for _, reply := range []domain.InboundTextMessage{
		whatsAppReply(channel, "wamid.cot27", conversationSender, changeOnQuote27),
		whatsAppReply(channel, "wamid.la99", conversationSender, "sobre la 99, ¿llega el lunes?"),
	} {
		if err := service.Ingest(context.Background(), []domain.InboundTextMessage{reply}); err != nil {
			t.Fatalf("Ingest(%s) = %v", reply.ExternalMessageID, err)
		}
	}
	for quoteID, want := range map[uuid.UUID]int{older: 1, newest: 1} {
		if messages, _ := e.conversationOf(t, quoteID); messages != want {
			t.Errorf("quote %s messages = %d, want %d", quoteID, messages, want)
		}
	}
	var referenced, unknown int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT (SELECT other_live_quotes FROM quote_message WHERE quote_id = $1),
		        (SELECT other_live_quotes FROM quote_message WHERE quote_id = $2)`, older, newest).
		Scan(&referenced, &unknown); err != nil {
		t.Fatalf("read hints: %v", err)
	}
	if referenced != 0 || unknown != 1 {
		t.Errorf("other_live_quotes = %d on the named quote and %d on the newest, want 0 and 1",
			referenced, unknown)
	}
}

func newOrderReply(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	return &domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelNoMatch,
		Interpretation:   "El cliente pide cotizar un pedido aparte.",
		CustomerDecision: domain.CustomerDecisionNewOrder, DecisionConfidencePercent: 90,
		Operations: []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationRequiresSeller}}}, nil
}

// A batch the model reads as a separate order goes to intake and leaves the quote it joined as it was.
func TestConversationHandler_ANewOrderGoesToIntake(t *testing.T) {
	e := newEnv(t)
	quote := e.sentHandlerQuote(t, "New order reply")
	batch := e.claimedBatch(t, quote.seed.tenant.AccountID, quote.quoteID, separateOrder)
	var channelID uuid.UUID
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`UPDATE quote_message SET channel_id = (SELECT channel_id FROM quote_send WHERE version_id = $2
		                                        ORDER BY sent_at DESC LIMIT 1)
		 WHERE batch_id = $1 RETURNING channel_id`, batch.ID, quote.sentID).Scan(&channelID); err != nil {
		t.Fatalf("put the message on the send's channel: %v", err)
	}
	intake := &recordingInboundRFQs{}

	if err := e.batchChangeHandler(&scriptedProposer{answer: newOrderReply}, nil).WithNewOrders(intake).
		Handle(context.Background(), batch); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	if intake.count() != 1 {
		t.Fatalf("orders taken = %d, want 1", intake.count())
	}
	state := e.quoteState(t, quote.quoteID)
	if state.status != domain.QuoteStatusSent || state.currentID != quote.sentID || state.versions != 1 {
		t.Errorf("quote = %+v, want it SENT on the sent version", state)
	}
	var reason, recorded string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT d.ai_proposal::jsonb ->> 'no_match_reason', d.ai_proposal::jsonb ->> 'new_order_rfq_id'
		 FROM handler_decision d WHERE d.message_batch_id = $1`, batch.ID).Scan(&reason, &recorded); err != nil {
		t.Fatalf("read the decision: %v", err)
	}
	if reason != string(domain.HandlerNoMatchNewOrder) || recorded != intake.rfqIDs[0].String() {
		t.Errorf("decision = %s naming %s, want NEW_ORDER naming the order taken (%s)", reason, recorded,
			intake.rfqIDs[0])
	}
}
