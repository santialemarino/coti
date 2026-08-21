//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
)

// seedBranch adds a second branch to an account. Its teardown is the account's, which deletes
// every branch it has.
func (e *env) seedBranch(t *testing.T, accountID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	branchID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO branch (id, account_id, name) VALUES ($1, $2, $3)`,
		branchID, accountID, name); err != nil {
		t.Fatalf("seed branch: %v", err)
	}
	return branchID
}

// seedRFQ creates one RFQ to hang attachments off, with its own teardown.
func (e *env) seedRFQ(t *testing.T, accountID, branchID uuid.UUID) uuid.UUID {
	t.Helper()
	channelID := e.seedIntakeChannel(t, accountID, branchID)
	rfqID := uuid.New()

	if _, err := e.db.CrossAccount().Exec(context.Background(),
		`INSERT INTO rfq (id, account_id, branch_id, channel_id, raw_text, status)
		 VALUES ($1, $2, $3, $4, $5, 'RECEIVED')`,
		rfqID, accountID, branchID, channelID, "pedido con adjuntos"); err != nil {
		t.Fatalf("seed rfq: %v", err)
	}
	t.Cleanup(func() {
		e.mustCleanup(t, `DELETE FROM rfq_attachment WHERE rfq_id = $1`, rfqID)
		e.mustCleanup(t, `DELETE FROM rfq WHERE id = $1`, rfqID)
	})
	return rfqID
}

// uploadAttachment posts one file to an RFQ, declaring contentType on the part itself, which is
// what the service reads.
func (e *env) uploadAttachment(
	t *testing.T, token, branch string, rfqID uuid.UUID, filename, contentType string, content []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	headers := make(textproto.MIMEHeader)
	headers.Set("Content-Disposition",
		`form-data; name="file"; filename="`+filename+`"`)
	headers.Set("Content-Type", contentType)
	part, err := writer.CreatePart(headers)
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost,
		"/v1/rfqs/"+rfqID.String()+"/attachments", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	if branch != "" {
		req.Header.Set("X-Branch-Id", branch)
	}

	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

type attachmentBody struct {
	ID    string `json:"id"`
	RFQID string `json:"rfq_id"`
	Type  string `json:"type"`
	URL   string `json:"url"`
}

func decodeAttachment(t *testing.T, rec *httptest.ResponseRecorder) attachmentBody {
	t.Helper()
	var body attachmentBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode attachment %s: %v", rec.Body, err)
	}
	return body
}

// fetchLink follows a signed link through the router, which is what makes the reference a
// working download rather than a string.
func (e *env) fetchLink(t *testing.T, rawURL string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, rawURL, nil)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// AC1: the file uploads, the reference persists, and that reference resolves to a link that
// downloads the file.
func TestRFQAttachments_UploadPersistsAReferenceThatResolvesToADownload(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Adjuntos")
	user := e.seedUser(t, accountID, "ADMIN")
	token := e.tokenFor(t, user)
	rfqID := e.seedRFQ(t, accountID, branchID)
	const content = "%PDF-1.7 presupuesto"

	rec := e.uploadAttachment(t, token, branchID.String(), rfqID, "plano.pdf", "application/pdf",
		[]byte(content))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201; body: %s", rec.Code, rec.Body)
	}
	uploaded := decodeAttachment(t, rec)
	if uploaded.Type != "PDF" {
		t.Errorf("type = %q, want PDF", uploaded.Type)
	}

	// The row is what persists; the link is derived from it on every read.
	var storedKey string
	var status string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT file_url, processing_status FROM rfq_attachment WHERE id = $1`,
		uploaded.ID).Scan(&storedKey, &status); err != nil {
		t.Fatalf("read stored attachment: %v", err)
	}
	if status != "PENDING" {
		t.Errorf("processing_status = %q, want PENDING", status)
	}

	listed := e.do(t, request{method: http.MethodGet,
		path: "/v1/rfqs/" + rfqID.String() + "/attachments", token: token, branch: branchID.String()})
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", listed.Code, listed.Body)
	}
	var list struct {
		Attachments []attachmentBody `json:"attachments"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list %s: %v", listed.Body, err)
	}
	if len(list.Attachments) != 1 || list.Attachments[0].ID != uploaded.ID {
		t.Fatalf("list = %#v, want the one uploaded attachment", list.Attachments)
	}

	downloaded := e.fetchLink(t, list.Attachments[0].URL)
	if downloaded.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200; body: %s", downloaded.Code, downloaded.Body)
	}
	if got := downloaded.Body.String(); got != content {
		t.Errorf("downloaded body = %q, want %q", got, content)
	}
	if got := downloaded.Header().Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", got)
	}
}

// AC4: the object key carries the account, and one account's file is not reachable from another.
func TestRFQAttachments_AnotherAccountReachesNeitherTheRowNorTheFile(t *testing.T) {
	e := newEnv(t)
	ownerAccount, ownerBranch := e.seedAccount(t, "Corralón Dueño")
	owner := e.seedUser(t, ownerAccount, "ADMIN")
	ownerToken := e.tokenFor(t, owner)
	rfqID := e.seedRFQ(t, ownerAccount, ownerBranch)

	rec := e.uploadAttachment(t, ownerToken, ownerBranch.String(), rfqID, "plano.pdf",
		"application/pdf", []byte("%PDF-1.7 privado"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201; body: %s", rec.Code, rec.Body)
	}
	uploaded := decodeAttachment(t, rec)

	var storedKey string
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT file_url FROM rfq_attachment WHERE id = $1`, uploaded.ID).Scan(&storedKey); err != nil {
		t.Fatalf("read stored key: %v", err)
	}
	// The account leads the key, so isolation is visible in the path itself.
	if want := "accounts/" + ownerAccount.String() + "/"; !strings.HasPrefix(storedKey, want) {
		t.Errorf("key = %q, want it to start with %q", storedKey, want)
	}

	// A second account, its own RFQ, asking for the first account's attachment.
	intruderAccount, intruderBranch := e.seedAccount(t, "Corralón Intruso")
	intruder := e.seedUser(t, intruderAccount, "ADMIN")
	intruderToken := e.tokenFor(t, intruder)

	listed := e.do(t, request{method: http.MethodGet,
		path: "/v1/rfqs/" + rfqID.String() + "/attachments", token: intruderToken,
		branch: intruderBranch.String()})
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", listed.Code, listed.Body)
	}
	var list struct {
		Attachments []attachmentBody `json:"attachments"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list %s: %v", listed.Body, err)
	}
	if len(list.Attachments) != 0 {
		t.Fatalf("another account listed %d attachments, want none", len(list.Attachments))
	}

	// And it cannot upload into the first account's RFQ either: the foreign key would accept
	// that row, so the insert proves the RFQ is the caller's before it writes.
	intruding := e.uploadAttachment(t, intruderToken, intruderBranch.String(), rfqID, "otro.pdf",
		"application/pdf", []byte("%PDF-1.7 ajeno"))
	if intruding.Code != http.StatusNotFound {
		t.Fatalf("cross-account upload status = %d, want 404; body: %s",
			intruding.Code, intruding.Body)
	}
	var rows int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM rfq_attachment WHERE rfq_id = $1`, rfqID).Scan(&rows); err != nil {
		t.Fatalf("count attachments: %v", err)
	}
	if rows != 1 {
		t.Errorf("rfq has %d attachments, want only the owner's 1", rows)
	}
}

// The branch is the boundary row level security does not guard, so it is the one a missing
// predicate leaks through without any cross-account test noticing.
func TestRFQAttachments_AnotherBranchOfTheSameAccountReachesNothing(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Dos Sucursales")
	otherBranch := e.seedBranch(t, accountID, "Sucursal Norte")
	user := e.seedUser(t, accountID, "ADMIN")
	rfqID := e.seedRFQ(t, accountID, branchID)

	ownerToken := e.tokenFor(t, user)
	rec := e.uploadAttachment(t, ownerToken, branchID.String(), rfqID, "plano.pdf",
		"application/pdf", []byte("%PDF-1.7 sucursal"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201; body: %s", rec.Code, rec.Body)
	}

	// Same account, same user, a different branch selected.
	otherToken := e.tokenFor(t, user)
	listed := e.do(t, request{method: http.MethodGet,
		path: "/v1/rfqs/" + rfqID.String() + "/attachments", token: otherToken,
		branch: otherBranch.String()})
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", listed.Code, listed.Body)
	}
	var list struct {
		Attachments []attachmentBody `json:"attachments"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list %s: %v", listed.Body, err)
	}
	if len(list.Attachments) != 0 {
		t.Fatalf("another branch listed %d attachments, want none", len(list.Attachments))
	}

	intruding := e.uploadAttachment(t, otherToken, otherBranch.String(), rfqID, "otro.pdf",
		"application/pdf", []byte("%PDF-1.7 otra sucursal"))
	if intruding.Code != http.StatusNotFound {
		t.Fatalf("cross-branch upload status = %d, want 404; body: %s",
			intruding.Code, intruding.Body)
	}
}

// AC3: a file of a type that is not accepted is refused before anything is stored.
func TestRFQAttachments_UnsupportedTypeIsRefusedBeforeAnythingIsStored(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Corralón Tipos")
	user := e.seedUser(t, accountID, "ADMIN")
	token := e.tokenFor(t, user)
	rfqID := e.seedRFQ(t, accountID, branchID)

	rec := e.uploadAttachment(t, token, branchID.String(), rfqID, "index.html", "text/html",
		[]byte("<script>alert(1)</script>"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body)
	}
	if got := errorCode(t, rec); got != "UNSUPPORTED_FILE_TYPE" {
		t.Errorf("code = %q, want UNSUPPORTED_FILE_TYPE", got)
	}

	var rows int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM rfq_attachment WHERE rfq_id = $1`, rfqID).Scan(&rows); err != nil {
		t.Fatalf("count attachments: %v", err)
	}
	if rows != 0 {
		t.Errorf("a refused upload persisted %d rows, want none", rows)
	}
}

// AC3, the other half: a file over the configured size is refused, and nothing is stored.
func TestRFQAttachments_OversizedFileIsRefusedBeforeAnythingIsStored(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.Storage.MaxFileSize = 64 })
	accountID, branchID := e.seedAccount(t, "Corralón Tamaño")
	user := e.seedUser(t, accountID, "ADMIN")
	token := e.tokenFor(t, user)
	rfqID := e.seedRFQ(t, accountID, branchID)

	rec := e.uploadAttachment(t, token, branchID.String(), rfqID, "grande.pdf", "application/pdf",
		bytes.Repeat([]byte("x"), 4096))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body: %s", rec.Code, rec.Body)
	}
	if got := errorCode(t, rec); got != "FILE_TOO_LARGE" {
		t.Errorf("code = %q, want FILE_TOO_LARGE", got)
	}

	var rows int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT count(*) FROM rfq_attachment WHERE rfq_id = $1`, rfqID).Scan(&rows); err != nil {
		t.Fatalf("count attachments: %v", err)
	}
	if rows != 0 {
		t.Errorf("a refused upload persisted %d rows, want none", rows)
	}
}
