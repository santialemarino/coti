//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
	"github.com/santialemarino/coti/apps/api/internal/storage"
)

type stagedEmailExtractor struct {
	stagedExtractor
	t *testing.T
}

func (s stagedEmailExtractor) ExtractFromContent(ctx context.Context, blocks []domain.Content,
	_ []domain.RFQInterpretationExample) (*domain.RFQExtraction, error) {
	s.t.Helper()
	if len(blocks) != 1 || blocks[0].Kind != domain.ContentKindText ||
		!strings.Contains(blocks[0].Text, "2 bolsas de cemento") {
		s.t.Fatalf("extraction input = %+v, want the stored email body", blocks)
	}
	return s.Extract(ctx, blocks[0].Text)
}

func TestEmailQuoteFlow_UsesTheReceivingBranchCatalogPricesAndPromotions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, firstBranch := e.seedAccount(t, "Email quote flow")
	secondBranch := e.seedBranch(t, accountID, "Second branch")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	productID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO product (id, account_id, canonical_name, description) VALUES ($1, $2, $3, $3)`,
		productID, accountID, "Cemento Portland 50kg"); err != nil {
		t.Fatal(err)
	}
	e.stock(t, accountID, firstBranch, productID)
	e.stock(t, accountID, secondBranch, productID)
	e.embedOn(t, productID, 0, 0.98)
	e.openPricePeriod(t, accountID, firstBranch, productID, "100.00", nil)
	e.openPricePeriod(t, accountID, secondBranch, productID, "200.00", nil)

	baseURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	objects, err := storage.NewLocalStorage(t.TempDir(), baseURL,
		storage.NewURLSigner([]byte("email-flow-test-secret-at-least-32-bytes"), nil))
	if err != nil {
		t.Fatal(err)
	}
	attachments := repository.NewRFQAttachmentRepository()
	intake := services.NewEmailInboundService(e.db, repository.NewInboundChannelMessageRepository(),
		repository.NewRFQRepository(), attachments, repository.NewChannelRepository(), objects, 1024)
	unit := "unidad"
	extractor := stagedEmailExtractor{t: t, stagedExtractor: stagedExtractor{lines: []domain.ExtractedRFQLine{{
		RequestedDescription: "2 bolsas de cemento", Quantity: decimal.NewFromInt(2), Unit: &unit,
		Source: domain.QuantitySourceExplicit, QuantityRationale: "Two bags requested",
	}}}}
	pipeline := e.pipeline(t, extractor, map[string]int{"2 bolsas de cemento": 0})
	reader := services.NewRFQAttachmentService(e.db, attachments, objects, config.StorageConfig{}, nil).
		WithStoredReading(nil, 20000, 1000)
	job := services.NewAttachmentExtractionJob(attachments, reader, pipeline,
		config.AttachmentConfig{ExtractionRFQBatchSize: 5, ExtractionReclaimAfter: time.Minute},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	quoteRepo := repository.NewQuoteRepository()
	quotes := services.NewQuoteService(e.db, quoteRepo, repository.NewProductPriceRepository(),
		domain.ARCAEnvironmentHomologation, nil).WithDiscountEngine(
		repository.NewQuoteDiscountRepository(), repository.NewPromotionRepository(), quoteRepo)

	tests := []struct {
		name, percentage, total string
		branch                  uuid.UUID
	}{
		{"First branch", "10", "180.00", firstBranch},
		{"Second branch", "20", "320.00", secondBranch},
	}
	for _, test := range tests {
		channelID := uuid.New()
		address := uuid.NewString() + "@example.test"
		if _, err := e.db.CrossAccount().Exec(ctx,
			`INSERT INTO channel (id, account_id, branch_id, type, identifier)
			 VALUES ($1, $2, $3, 'EMAIL', $4)`, channelID, accountID, test.branch, address); err != nil {
			t.Fatal(err)
		}
		promotionID := e.createPromotion(t, e.tokenFor(t, admin), map[string]any{
			"name": test.name, "branch_id": test.branch, "condition_type": "PER_ITEM",
			"action_type": "PERCENTAGE", "action_value": test.percentage,
			"conditions": []map[string]any{{"product_id": productID}},
		})
		route := domain.InboundChannelRoute{AccountID: accountID, BranchID: test.branch,
			ChannelID: channelID, ChannelType: domain.ChannelTypeEmail}
		message := domain.InboundEmailMessage{ExternalMessageID: "email-" + channelID.String(),
			DestinationIdentifier: address, SenderID: "client@example.test", Body: "2 bolsas de cemento",
			Original:           []byte("From: client@example.test\r\n\r\n2 bolsas de cemento"),
			ProviderReceivedAt: time.Now()}
		rfqID, err := intake.Ingest(ctx, route, message)
		if err != nil {
			t.Fatalf("%s: ingest email: %v", test.name, err)
		}
		retry, err := intake.Ingest(ctx, route, message)
		if err != nil || retry != rfqID {
			t.Fatalf("%s: retry = %s, %v, want %s", test.name, retry, err, rfqID)
		}
		if _, err := job.Run(ctx, e.db.CrossAccount()); err != nil {
			t.Fatalf("%s: extract email: %v", test.name, err)
		}
		var quoteID, versionID, rfqBranch, quoteBranch, matchedProduct uuid.UUID
		var matchStatus string
		if err := e.db.CrossAccount().QueryRow(ctx,
			`SELECT quote.id, quote.current_version_id, rfq.branch_id, quote.branch_id,
			        item.product_id, item.match_status
			 FROM rfq JOIN quote ON quote.rfq_id = rfq.id
			 JOIN quote_item item ON item.version_id = quote.current_version_id
			 WHERE rfq.id = $1`, rfqID).
			Scan(&quoteID, &versionID, &rfqBranch, &quoteBranch, &matchedProduct, &matchStatus); err != nil {
			t.Fatalf("%s: read generated quote: %v", test.name, err)
		}
		if rfqBranch != test.branch || quoteBranch != test.branch || matchedProduct != productID || matchStatus != "MATCHED" {
			t.Fatalf("%s: branch/quote branch/product/status = %s/%s/%s/%s", test.name,
				rfqBranch, quoteBranch, matchedProduct, matchStatus)
		}
		priced, err := quotes.AcceptMaterials(ctx, domain.Tenant{
			AccountID: accountID, BranchID: test.branch, UserID: admin.ID, Role: domain.UserRoleAdmin,
		}, quoteID)
		if err != nil {
			t.Fatalf("%s: accept materials: %v", test.name, err)
		}
		if !priced.Version.Total.Equal(decimal.RequireFromString(test.total)) {
			t.Errorf("%s: total = %s, want %s", test.name, priced.Version.Total, test.total)
		}
		discounts := e.storedDiscounts(t, versionID)
		if len(discounts) != 1 || discounts[test.name].promotionID == nil ||
			*discounts[test.name].promotionID != promotionID {
			t.Errorf("%s: discounts = %+v, want the receiving branch promotion", test.name, discounts)
		}
		var inboundCount, quoteCount int
		if err := e.db.CrossAccount().QueryRow(ctx,
			`SELECT (SELECT count(*) FROM inbound_channel_message WHERE channel_id = $1),
			        (SELECT count(*) FROM quote WHERE rfq_id = $2)`, channelID, rfqID).
			Scan(&inboundCount, &quoteCount); err != nil || inboundCount != 1 || quoteCount != 1 {
			t.Fatalf("%s: messages/quotes = %d/%d, error %v, want one of each", test.name,
				inboundCount, quoteCount, err)
		}
	}
}
