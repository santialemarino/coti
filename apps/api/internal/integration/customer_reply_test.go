//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func repliedWith(decision domain.CustomerDecision,
) func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
	return func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error) {
		return &domain.ChangeRequestProposal{Level: domain.ChangeRequestLevelNoMatch,
			Interpretation: "El cliente respondió sin pedir cambios.", CustomerDecision: decision,
			DecisionConfidencePercent: 90,
			Operations:                []domain.ChangeRequestOperation{{Type: domain.ChangeRequestOperationRequiresSeller}}}, nil
	}
}

// replyDesk is a sent quote with a seller who can read it over HTTP.
type replyDesk struct {
	handlerQuote
	token string
}

func (e *env) replyDesk(t *testing.T, name string) replyDesk {
	t.Helper()
	quote := e.sentHandlerQuote(t, name)
	seller := e.seedUser(t, quote.seed.tenant.AccountID, domain.UserRoleAdmin)
	return replyDesk{handlerQuote: quote, token: e.tokenFor(t, seller)}
}

// reply runs one batch of customer messages through the handler, as the conversation worker does.
func (e *env) reply(t *testing.T, desk replyDesk,
	answer func(domain.ChangeRequestInput) (*domain.ChangeRequestProposal, error), body string,
) uuid.UUID {
	t.Helper()
	batch := e.claimedBatch(t, desk.seed.tenant.AccountID, desk.quoteID, body)
	if err := e.acceptingHandler(answer, &capturedOutcomes{}).Handle(context.Background(),
		batch); err != nil {
		t.Fatalf("Handle(%q) = %v", body, err)
	}
	e.finishBatch(t, batch.ID)
	return batch.ID
}

// listed is the order as the queue and the detail both read it.
func (e *env) listed(t *testing.T, desk replyDesk) dto.RfqListItemResponse {
	t.Helper()
	branch := desk.seed.tenant.BranchID.String()
	rec := e.do(t, request{method: http.MethodGet, path: "/v1/rfqs", token: desk.token, branch: branch})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/rfqs = %d: %s", rec.Code, rec.Body)
	}
	var rows []dto.RfqListItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	rfqID := desk.seed.draft.RFQ.ID
	var row *dto.RfqListItemResponse
	for i := range rows {
		if rows[i].ID == rfqID {
			row = &rows[i]
		}
	}
	if row == nil {
		t.Fatalf("order %s missing from the list", rfqID)
	}
	rec = e.do(t, request{method: http.MethodGet, path: "/v1/rfqs/" + rfqID.String(),
		token: desk.token, branch: branch})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/rfqs/%s = %d: %s", rfqID, rec.Code, rec.Body)
	}
	var detail dto.RfqDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	listedJSON, _ := json.Marshal(row)
	detailJSON, _ := json.Marshal(detail.Rfq)
	if string(listedJSON) != string(detailJSON) {
		t.Fatalf("detail row %s, want the list row %s", detailJSON, listedJSON)
	}
	return *row
}

func (e *env) readReply(t *testing.T, desk replyDesk, seenUntil time.Time) (int, string) {
	t.Helper()
	rec := e.do(t, request{method: http.MethodPost,
		path:  "/v1/quotes/" + desk.quoteID.String() + "/customer-reply/read",
		token: desk.token, branch: desk.seed.tenant.BranchID.String(),
		body: dto.ReadCustomerReplyRequest{SeenUntil: &seenUntil}})
	if rec.Code == http.StatusNoContent {
		return rec.Code, ""
	}
	return rec.Code, errorCode(t, rec)
}

func replyReason(row dto.RfqListItemResponse) string {
	if row.CustomerReply == nil {
		return ""
	}
	return row.CustomerReply.Reason
}

// A reply that changes nothing keeps the quote SENT and reaches the queue, the newest one first.
func TestCustomerReply_TheQueueShowsTheNewestUnreadReply(t *testing.T) {
	e := newEnv(t)
	desk := e.replyDesk(t, "Reply question")

	if got := e.listed(t, desk); got.CustomerReply != nil || got.AcceptanceUnconfirmed {
		t.Fatalf("before any reply = %+v, want none", got)
	}
	e.reply(t, desk, repliedWith(domain.CustomerDecisionQuestion), "¿hacen envíos a Morón?")
	if got := replyReason(e.listed(t, desk)); got != "CUSTOMER_QUESTION" {
		t.Fatalf("reply = %q, want CUSTOMER_QUESTION", got)
	}
	e.reply(t, desk, repliedWith(domain.CustomerDecisionNone), "gracias, lo veo")
	row := e.listed(t, desk)
	if row.Status != "SENT" || replyReason(row) != "NO_REQUEST" || row.CustomerReply.ReplyAcceptance != nil {
		t.Errorf("row = %+v, want the newer NO_REQUEST on a SENT quote", row)
	}
}

// An acceptance the rules hold back stays a signal on the reply.
func TestCustomerReply_ASignalCarriesItsAcceptance(t *testing.T) {
	e := newEnv(t)
	desk := e.replyDesk(t, "Reply signal")

	e.reply(t, desk, acceptedReply(40), "dale")
	row := e.listed(t, desk)
	if row.Status != "SENT" || replyReason(row) != "CUSTOMER_ACCEPTS" ||
		row.CustomerReply.ReplyAcceptance == nil || *row.CustomerReply.ReplyAcceptance != "SIGNAL" {
		t.Errorf("row = %+v, want a SIGNAL acceptance on a SENT quote", row)
	}
	e.reply(t, desk, repliedWith(domain.CustomerDecisionReject), "no, gracias")
	if got := replyReason(e.listed(t, desk)); got != "CUSTOMER_REJECTS" {
		t.Errorf("reply = %q, want CUSTOMER_REJECTS", got)
	}
}

// «Marcar como leído» settles what is pending; the customer's next reply brings the order back.
func TestCustomerReply_ReadingDismissesItUntilTheCustomerWritesAgain(t *testing.T) {
	e := newEnv(t)
	desk := e.replyDesk(t, "Reply read")
	e.reply(t, desk, repliedWith(domain.CustomerDecisionQuestion), "¿tienen stock?")
	e.reply(t, desk, repliedWith(domain.CustomerDecisionNone), "ok")

	if code, errCode := e.readReply(t, desk, e.listed(t, desk).CustomerReply.CreatedAt); code != http.StatusNoContent {
		t.Fatalf("read = %d %s, want 204", code, errCode)
	}
	if got := e.listed(t, desk); got.CustomerReply != nil {
		t.Fatalf("after reading = %+v, want no reply", got.CustomerReply)
	}
	var settled int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM handler_decision WHERE quote_version_id = $1
		   AND seller_decision = 'MANUAL_OVERRIDE' AND user_id IS NOT NULL AND decided_at IS NOT NULL`,
		desk.sentID).Scan(&settled); err != nil || settled != 2 {
		t.Errorf("settled = %d, %v, want both replies settled by the seller", settled, err)
	}
	if code, errCode := e.readReply(t, desk, time.Now()); code != http.StatusNoContent {
		t.Errorf("a second read = %d %s, want 204 with nothing to settle", code, errCode)
	}

	e.reply(t, desk, repliedWith(domain.CustomerDecisionQuestion), "¿y el flete?")
	if got := replyReason(e.listed(t, desk)); got != "CUSTOMER_QUESTION" {
		t.Errorf("after writing again = %q, want CUSTOMER_QUESTION", got)
	}
}

// An acceptance waiting for the seller is flagged; undoing it leaves its reply behind.
func TestCustomerReply_AnUndoneAcceptanceLeavesItsReplyBehind(t *testing.T) {
	e := newEnv(t)
	desk := e.replyDesk(t, "Reply undo")
	e.reply(t, desk, acceptedReply(75), "dale")

	row := e.listed(t, desk)
	if row.Status != "ACCEPTED" || !row.AcceptanceUnconfirmed || row.CustomerReply != nil {
		t.Fatalf("row = %+v, want ACCEPTED pending confirmation, with no reply to read", row)
	}
	if code, errCode := e.readReply(t, desk, time.Now()); code != http.StatusConflict || errCode != "QUOTE_NOT_SENT" {
		t.Errorf("read on an accepted quote = %d %s, want 409 QUOTE_NOT_SENT", code, errCode)
	}
	if _, err := e.acceptanceQuoteService().UndoAcceptance(context.Background(), desk.seed.tenant,
		desk.quoteID); err != nil {
		t.Fatalf("UndoAcceptance() = %v", err)
	}
	row = e.listed(t, desk)
	if row.Status != "SENT" || row.AcceptanceUnconfirmed || row.CustomerReply != nil {
		t.Errorf("after the undo = %+v, want SENT with the old reply behind it", row)
	}
}

// A reply recorded after the seller's screen loaded stays unread when they mark the older one read.
func TestCustomerReply_ReadingLeavesALaterReplyUnread(t *testing.T) {
	e := newEnv(t)
	desk := e.replyDesk(t, "Reply read race")
	e.reply(t, desk, repliedWith(domain.CustomerDecisionQuestion), "¿tienen stock?")
	seen := e.listed(t, desk).CustomerReply.CreatedAt
	e.reply(t, desk, acceptedReply(40), "dale, lo quiero")

	if code, errCode := e.readReply(t, desk, seen); code != http.StatusNoContent {
		t.Fatalf("read = %d %s, want 204", code, errCode)
	}
	if got := replyReason(e.listed(t, desk)); got != "CUSTOMER_ACCEPTS" {
		t.Errorf("after reading the question = %q, want the later acceptance still unread", got)
	}
}
