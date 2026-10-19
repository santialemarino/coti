//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/handler"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/services"
)

func TestInboundChannelFlow_UsesTheDestinationBranchCatalogAndPrices(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, firstBranch := e.seedAccount(t, "Inbound channel flow")
	secondBranch := e.seedBranch(t, accountID, "Second branch")
	firstChannel := e.seedWhatsAppInbox(t, accountID, firstBranch)
	secondChannel := e.seedWhatsAppInbox(t, accountID, secondBranch)
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)

	cement, membrane := uuid.New(), uuid.New()
	for _, product := range []struct {
		id   uuid.UUID
		name string
	}{
		{cement, "Cemento Portland 50kg"},
		{membrane, "Membrana liquida"},
	} {
		if _, err := e.db.CrossAccount().Exec(ctx,
			`INSERT INTO product (id, account_id, canonical_name, description) VALUES ($1, $2, $3, $3)`,
			product.id, accountID, product.name); err != nil {
			t.Fatalf("seed product: %v", err)
		}
	}
	e.stock(t, accountID, firstBranch, cement)
	e.stock(t, accountID, secondBranch, cement)
	e.stock(t, accountID, firstBranch, membrane)
	e.embedOn(t, cement, 0, 0.98)
	e.embedOn(t, membrane, 2, 0.98)
	e.openPricePeriod(t, accountID, firstBranch, cement, "100.00", nil)
	e.openPricePeriod(t, accountID, secondBranch, cement, "200.00", nil)
	e.openPricePeriod(t, accountID, firstBranch, membrane, "25.00", nil)

	unit := "unidad"
	extractor := stagedExtractor{lines: []domain.ExtractedRFQLine{
		{RequestedDescription: "2 bolsas de cemento", Quantity: decimal.NewFromInt(2),
			Unit: &unit, Source: domain.QuantitySourceExplicit, QuantityRationale: "Two bags requested"},
		{RequestedDescription: "1 membrana liquida", Quantity: decimal.NewFromInt(1),
			Unit: &unit, Source: domain.QuantitySourceExplicit, QuantityRationale: "One membrane requested"},
	}}
	pipeline := e.pipeline(t, extractor, map[string]int{"2 bolsas de cemento": 0, "1 membrana liquida": 2})
	inbound := services.NewWhatsAppInboundService(e.db,
		services.NewInboundRoutingService(e.db, repository.NewChannelRepository()),
		repository.NewInboundChannelMessageRepository(), nil, pipeline, nil, nil)
	router := gin.New()
	router.POST("/webhook", handler.NewWhatsAppWebhookHandler("test-secret", 4096, inbound).Receive)

	for _, test := range []struct {
		name          string
		channel       inboundChannel
		branch        uuid.UUID
		membraneMatch bool
		wantTotal     string
	}{
		{"first branch", firstChannel, firstBranch, true, "225.00"},
		{"second branch", secondChannel, secondBranch, false, "400.00"},
	} {
		t.Run(test.name, func(t *testing.T) {
			externalID := "wamid." + test.channel.phoneNumberID
			body := []byte(fmt.Sprintf(`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{"metadata":{"phone_number_id":%q},"messages":[{"from":"5491112345678","id":%q,"type":"text","text":{"body":"2 bolsas de cemento y 1 membrana liquida"}}]}}]}]}`,
				test.channel.phoneNumberID, externalID))
			mac := hmac.New(sha256.New, []byte("test-secret"))
			_, _ = mac.Write(body)
			signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

			for attempt := range 2 {
				req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
				req.Header.Set("X-Hub-Signature-256", signature)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				if response.Code != http.StatusOK {
					t.Fatalf("webhook attempt %d = %d, want 200", attempt+1, response.Code)
				}
			}

			var rfqID, quoteID, versionID, rfqBranch, quoteBranch, channelID uuid.UUID
			if err := e.db.CrossAccount().QueryRow(ctx,
				`SELECT rfq.id, quote.id, quote.current_version_id, rfq.branch_id, quote.branch_id, rfq.channel_id
				 FROM inbound_channel_message inbound
				 JOIN rfq ON rfq.id = inbound.rfq_id
				 JOIN quote ON quote.rfq_id = rfq.id
				 WHERE inbound.channel_id = $1 AND inbound.external_message_id = $2`,
				test.channel.id, externalID).Scan(&rfqID, &quoteID, &versionID, &rfqBranch,
				&quoteBranch, &channelID); err != nil {
				t.Fatalf("read routed draft: %v", err)
			}
			if rfqBranch != test.branch || quoteBranch != test.branch || channelID != test.channel.id {
				t.Fatalf("rfq branch/quote branch/channel = %s/%s/%s, want %s/%s/%s",
					rfqBranch, quoteBranch, channelID, test.branch, test.branch, test.channel.id)
			}

			var inboundCount, rfqCount, quoteCount int
			if err := e.db.CrossAccount().QueryRow(ctx,
				`SELECT (SELECT count(*) FROM inbound_channel_message WHERE channel_id = $1),
				        (SELECT count(*) FROM rfq WHERE channel_id = $1),
				        (SELECT count(*) FROM quote JOIN rfq ON quote.rfq_id = rfq.id WHERE rfq.channel_id = $1)`,
				test.channel.id).Scan(&inboundCount, &rfqCount, &quoteCount); err != nil {
				t.Fatalf("count retried delivery outcomes: %v", err)
			}
			if inboundCount != 1 || rfqCount != 1 || quoteCount != 1 {
				t.Fatalf("inbound messages/RFQs/quotes = %d/%d/%d, want one of each after a retry",
					inboundCount, rfqCount, quoteCount)
			}
			var cementID *uuid.UUID
			var cementStatus string
			if err := e.db.CrossAccount().QueryRow(ctx,
				`SELECT product_id, match_status FROM quote_item
				 WHERE version_id = $1 AND requested_description = $2`,
				versionID, "2 bolsas de cemento").Scan(&cementID, &cementStatus); err != nil {
				t.Fatalf("read cement match: %v", err)
			}
			if cementID == nil || *cementID != cement || cementStatus != "MATCHED" {
				t.Fatalf("cement = %v/%s, want the shared product matched", cementID, cementStatus)
			}
			var membraneID *uuid.UUID
			var membraneStatus string
			if err := e.db.CrossAccount().QueryRow(ctx,
				`SELECT product_id, match_status FROM quote_item
				 WHERE version_id = $1 AND requested_description = $2`,
				versionID, "1 membrana liquida").Scan(&membraneID, &membraneStatus); err != nil {
				t.Fatalf("read membrane match: %v", err)
			}
			if test.membraneMatch {
				if membraneID == nil || *membraneID != membrane || membraneStatus != "MATCHED" {
					t.Fatalf("membrane = %v/%s, want the available product matched", membraneID, membraneStatus)
				}
			} else if membraneID != nil || membraneStatus != "NO_MATCH" {
				t.Fatalf("membrane = %v/%s, want NO_MATCH for an unavailable product", membraneID, membraneStatus)
			}

			quotes := services.NewQuoteService(e.db, repository.NewQuoteRepository(),
				repository.NewProductPriceRepository(), domain.ARCAEnvironmentHomologation, nil)
			priced, err := quotes.AcceptMaterials(ctx, domain.Tenant{
				AccountID: accountID, BranchID: test.branch, UserID: admin.ID, Role: domain.UserRoleAdmin,
			}, quoteID)
			if err != nil {
				t.Fatalf("accept materials: %v", err)
			}
			wantTotal := decimal.RequireFromString(test.wantTotal)
			storedTotal := e.storedVersionTotal(t, versionID)
			if !priced.Version.Total.Equal(wantTotal) || !storedTotal.Equal(wantTotal) {
				t.Errorf("response/stored total = %s/%s, want %s from this branch's prices",
					priced.Version.Total, storedTotal, test.wantTotal)
			}
		})
	}
}
