//go:build integration

package integration

import (
	"context"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

func TestEmailInbound_PreservesOriginalAndRoutesIdempotently(t *testing.T) {
	e := newEnv(t)
	accountID, branchID := e.seedAccount(t, "Email intake")
	otherBranch := e.seedBranch(t, accountID, "Other branch")
	channelID, otherChannelID := uuid.New(), uuid.New()
	address := uuid.NewString() + "@example.test"
	otherAddress := uuid.NewString() + "@example.test"
	for _, seed := range []struct {
		branchID uuid.UUID
		channel  uuid.UUID
		address  string
	}{
		{branchID, channelID, address},
		{otherBranch, otherChannelID, otherAddress},
	} {
		if _, err := e.db.CrossAccount().Exec(context.Background(),
			`INSERT INTO channel (id, account_id, branch_id, type, identifier)
			 VALUES ($1, $2, $3, 'EMAIL', $4)`, seed.channel, accountID, seed.branchID, seed.address); err != nil {
			t.Fatal(err)
		}
	}
	baseURL, _ := url.Parse("http://localhost:8080")
	objects, err := storage.NewLocalStorage(t.TempDir(), baseURL,
		storage.NewURLSigner([]byte("email-inbound-test-secret-at-least-32-bytes"), nil))
	if err != nil {
		t.Fatal(err)
	}
	service := services.NewEmailInboundService(e.db, repository.NewInboundChannelMessageRepository(),
		repository.NewRFQRepository(), repository.NewRFQAttachmentRepository(),
		repository.NewChannelRepository(), objects, 1024).WithContacts(services.NewClientContactService(e.db,repository.NewClientRepository(),repository.NewClientContactRepository()))
	route := domain.InboundChannelRoute{AccountID: accountID, BranchID: branchID,
		ChannelID: channelID, ChannelType: domain.ChannelTypeEmail}
	original := []byte("From: client@example.com\r\n\r\noriginal full email")
	message := domain.InboundEmailMessage{ExternalMessageID: "gmail-message-123",
		DestinationIdentifier: address, SenderID: "client@example.com", SenderLabel: "Client", Subject: "Pedido",
		Body: "20 bolsas de cemento", Original: original,
		Attachments: []domain.InboundEmailAttachment{
			{Filename: "pedido.pdf", ContentType: "application/pdf", Content: []byte("pdf bytes")},
			{Filename: "other.bin", ContentType: "application/octet-stream", Content: []byte("other bytes")},
		}}
	first, err := service.Ingest(context.Background(), route, message)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	retry, err := service.Ingest(context.Background(), route, message)
	if err != nil || retry != first {
		t.Fatalf("retry = %s, %v; want %s", retry, err, first)
	}
	otherRoute := domain.InboundChannelRoute{AccountID: accountID, BranchID: otherBranch,
		ChannelID: otherChannelID, ChannelType: domain.ChannelTypeEmail}
	otherMessage := message
	otherMessage.DestinationIdentifier = otherAddress
	otherRFQ, err := service.Ingest(context.Background(), otherRoute, otherMessage)
	if err != nil || otherRFQ == first {
		t.Fatalf("other branch RFQ = %s, %v", otherRFQ, err)
	}
	wrongRoute := route
	wrongRoute.BranchID = otherBranch
	if _, err := service.Ingest(context.Background(), wrongRoute, message); err == nil {
		t.Fatal("a channel from another branch must be refused")
	}
	for _, target := range []struct {
		name  string
		query string
		id    uuid.UUID
	}{
		{"branch", "UPDATE branch SET is_active = $2 WHERE id = $1", branchID},
		{"account", "UPDATE account SET is_active = $2 WHERE id = $1", accountID},
	} {
		if _, err := e.db.CrossAccount().Exec(context.Background(), target.query, target.id, false); err != nil {
			t.Fatal(err)
		}
		closedMessage := message
		closedMessage.ExternalMessageID = uuid.NewString()
		if _, err := service.Ingest(context.Background(), route, closedMessage); err == nil {
			t.Fatalf("a closed %s must reject intake", target.name)
		}
		if _, err := e.db.CrossAccount().Exec(context.Background(), target.query, target.id, true); err != nil {
			t.Fatal(err)
		}
	}

	var rfqCount, messageCount int
	if err := e.db.CrossAccount().QueryRow(context.Background(),
		`SELECT (SELECT count(*) FROM rfq WHERE account_id = $1),
		        (SELECT count(*) FROM inbound_channel_message WHERE account_id = $1)`, accountID).
		Scan(&rfqCount, &messageCount); err != nil || rfqCount != 2 || messageCount != 2 {
		t.Fatalf("rfqs/messages = %d/%d, %v; want 2/2", rfqCount, messageCount, err)
	}
	var files []domain.RFQAttachment
	if err := e.db.InTenantTx(context.Background(), route.Tenant(), func(q repository.Querier) error {
		var err error
		files, err = repository.NewRFQAttachmentRepository().ListByRFQ(context.Background(), q,
			accountID, branchID, first)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("attachments = %d, want original, body, PDF and unsupported file", len(files))
	}
	byName := make(map[string]domain.RFQAttachment)
	for _, file := range files {
		if file.OriginalFilename != nil {
			byName[*file.OriginalFilename] = file
		}
	}
	if byName["pedido.pdf"].ProcessingStatus != domain.AttachmentProcessingPending ||
		byName["other.bin"].ProcessingStatus != domain.AttachmentProcessingFailed ||
		byName["email.txt"].ProcessingStatus != domain.AttachmentProcessingPending {
		t.Fatalf("file states = %+v", byName)
	}
	stored := byName["original.eml"]
	if stored.StorageKey == nil || !strings.HasPrefix(*stored.StorageKey, "accounts/"+accountID.String()+"/rfqs/") {
		t.Fatalf("original key = %v", stored.StorageKey)
	}
	object, err := objects.Download(context.Background(), *stored.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	defer object.Body.Close()
	bytes, err := io.ReadAll(object.Body)
	if err != nil || string(bytes) != string(original) {
		t.Fatalf("original bytes = %q, %v", bytes, err)
	}
}
