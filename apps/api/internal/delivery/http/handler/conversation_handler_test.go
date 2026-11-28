package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type fakeConversationReview struct {
	tenant    domain.Tenant
	quoteID   uuid.UUID
	seenUntil time.Time
	err       error
}

func (f *fakeConversationReview) DiscardAIChanges(context.Context, domain.Tenant,
	uuid.UUID) (*domain.QuoteVersion, error) {
	return nil, nil
}

func (f *fakeConversationReview) RetryBatch(context.Context, domain.Tenant, uuid.UUID,
	uuid.UUID) (*domain.MessageBatch, error) {
	return nil, nil
}

func (f *fakeConversationReview) ReadCustomerReply(_ context.Context, tenant domain.Tenant,
	quoteID uuid.UUID, seenUntil time.Time) error {
	f.tenant, f.quoteID, f.seenUntil = tenant, quoteID, seenUntil
	return f.err
}

const seenUntil = "2026-11-27T21:04:05.123456Z"

func readCustomerReply(t *testing.T, fake *fakeConversationReview, quoteID string,
) *httptest.ResponseRecorder {
	return readCustomerReplyWith(t, fake, quoteID, `{"seen_until":"`+seenUntil+`"}`)
}

func readCustomerReplyWith(t *testing.T, fake *fakeConversationReview, quoteID, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "quoteId", Value: quoteID}}
	middleware.SetTenant(c, domain.Tenant{AccountID: uuid.New(), BranchID: uuid.New()})
	NewConversationHandler(fake).ReadCustomerReply(c)
	// Gin writes a bodiless status when the request ends, which a bare handler call never reaches.
	c.Writer.WriteHeaderNow()
	return recorder
}

func TestConversationHandler_ReadCustomerReply_AnswersNoContent(t *testing.T) {
	fake := &fakeConversationReview{}
	quoteID := uuid.New()

	recorder := readCustomerReply(t, fake, quoteID.String())

	want, _ := time.Parse(time.RFC3339Nano, seenUntil)
	if recorder.Code != http.StatusNoContent || fake.quoteID != quoteID ||
		fake.tenant.AccountID == uuid.Nil || !fake.seenUntil.Equal(want) {
		t.Fatalf("status = %d, quote = %s, tenant = %+v, seen until %s; want 204 for the path's "+
			"quote up to the reply seen", recorder.Code, fake.quoteID, fake.tenant, fake.seenUntil)
	}
}

func TestConversationHandler_ReadCustomerReply_NeedsTheReplySeen(t *testing.T) {
	fake := &fakeConversationReview{}

	recorder := readCustomerReplyWith(t, fake, uuid.NewString(), `{}`)

	if recorder.Code != http.StatusBadRequest || fake.quoteID != uuid.Nil {
		t.Fatalf("status = %d, service called with %s; want 400 before the service", recorder.Code,
			fake.quoteID)
	}
}

func TestConversationHandler_ReadCustomerReply_NamesAQuoteNoLongerSent(t *testing.T) {
	fake := &fakeConversationReview{err: domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)}

	recorder := readCustomerReply(t, fake, uuid.NewString())

	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || body.Code != string(domain.CodeQuoteNotSent) {
		t.Fatalf("status = %d, code = %q; want 409 QUOTE_NOT_SENT", recorder.Code, body.Code)
	}
}

func TestConversationHandler_ReadCustomerReply_RefusesAMalformedID(t *testing.T) {
	fake := &fakeConversationReview{}

	recorder := readCustomerReply(t, fake, "not-a-uuid")

	if recorder.Code != http.StatusBadRequest || fake.quoteID != uuid.Nil {
		t.Fatalf("status = %d, service called with %s; want 400 before the service", recorder.Code,
			fake.quoteID)
	}
}
