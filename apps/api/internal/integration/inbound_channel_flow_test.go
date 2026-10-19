//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestInboundChannelFlow_EmailRouteUsesDestinationBranchPricesAndPromotions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	accountID, firstBranch := e.seedAccount(t, "Email channel flow")
	secondBranch := e.seedBranch(t, accountID, "Second branch")
	admin := e.seedUser(t, accountID, domain.UserRoleAdmin)
	productID := uuid.New()
	if _, err := e.db.CrossAccount().Exec(ctx,
		`INSERT INTO product (id, account_id, canonical_name, description) VALUES ($1, $2, $3, $3)`,
		productID, accountID, "Cemento Portland 50kg"); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	e.stock(t, accountID, firstBranch, productID)
	e.stock(t, accountID, secondBranch, productID)
	e.embedOn(t, productID, 0, 0.98)
	e.openPricePeriod(t, accountID, firstBranch, productID, "100.00", nil)
	e.openPricePeriod(t, accountID, secondBranch, productID, "200.00", nil)

	unit := "unidad"
	pipeline := e.pipeline(t, stagedExtractor{lines: []domain.ExtractedRFQLine{{
		RequestedDescription: "2 bolsas de cemento", Quantity: decimal.NewFromInt(2),
		Unit: &unit, Source: domain.QuantitySourceExplicit, QuantityRationale: "Two bags requested",
	}}}, map[string]int{"2 bolsas de cemento": 0})
	routing := services.NewInboundRoutingService(e.db, repository.NewChannelRepository())
	quoteRepo := repository.NewQuoteRepository()
	quotes := services.NewQuoteService(e.db, quoteRepo, repository.NewProductPriceRepository(),
		domain.ARCAEnvironmentHomologation, nil).WithDiscountEngine(
		repository.NewQuoteDiscountRepository(), repository.NewPromotionRepository(), quoteRepo)

	tests := []struct {
		name, percentage, total, mailbox string
		branch, channelID, promotionID   uuid.UUID
	}{
		{name: "first branch", percentage: "10", total: "180.00", branch: firstBranch},
		{name: "second branch", percentage: "20", total: "320.00", branch: secondBranch},
	}
	for i := range tests {
		test := &tests[i]
		test.channelID = uuid.New()
		test.mailbox = "pedidos+" + uuid.NewString() + "@corralon.test"
		if _, err := e.db.CrossAccount().Exec(ctx,
			`INSERT INTO channel (id, account_id, branch_id, type, identifier)
			 VALUES ($1, $2, $3, 'EMAIL', $4)`, test.channelID, accountID, test.branch, test.mailbox); err != nil {
			t.Fatalf("seed email channel: %v", err)
		}
		test.promotionID = e.createPromotion(t, e.tokenFor(t, admin), map[string]any{
			"name": test.name, "branch_id": test.branch, "condition_type": "PER_ITEM",
			"action_type": "PERCENTAGE", "action_value": test.percentage,
			"conditions": []map[string]any{{"product_id": productID}},
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			destination := strings.ToUpper(test.mailbox)
			routes, err := routing.ResolveChannels(ctx, domain.ChannelTypeEmail, []string{destination})
			if err != nil {
				t.Fatalf("resolve email destination: %v", err)
			}
			route, found := routes[test.mailbox]
			if !found || route.AccountID != accountID || route.BranchID != test.branch || route.ChannelID != test.channelID {
				t.Fatalf("email route = %+v, found %v, want account/branch/channel %s/%s/%s",
					route, found, accountID, test.branch, test.channelID)
			}
			forged := route
			forged.BranchID = firstBranch
			if test.branch == firstBranch {
				forged.BranchID = secondBranch
			}
			if _, err := pipeline.CreateInboundTextDraft(ctx, forged,
				domain.InboundTextRFQDraftInput{RawText: "2 bolsas de cemento"}); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("draft with another branch's channel = %v, want not found", err)
			}
			draft, err := pipeline.CreateInboundTextDraft(ctx, route,
				domain.InboundTextRFQDraftInput{RawText: "2 bolsas de cemento"})
			if err != nil {
				t.Fatalf("create routed draft: %v", err)
			}
			if draft.Quote == nil || draft.Version == nil || draft.RFQ.BranchID != test.branch ||
				draft.RFQ.ChannelID != test.channelID || draft.Quote.BranchID != test.branch {
				t.Fatalf("draft = %+v, want RFQ and quote for the destination branch", draft)
			}
			if len(draft.Items) != 1 || draft.Items[0].ProductID == nil ||
				*draft.Items[0].ProductID != productID || draft.Items[0].MatchStatus != domain.ItemMatchStatusMatched {
				t.Fatalf("matched items = %+v, want the branch product", draft.Items)
			}
			priced, err := quotes.AcceptMaterials(ctx, domain.Tenant{
				AccountID: accountID, BranchID: test.branch, UserID: admin.ID, Role: domain.UserRoleAdmin,
			}, draft.Quote.ID)
			if err != nil {
				t.Fatalf("accept materials: %v", err)
			}
			if !priced.Version.Total.Equal(decimal.RequireFromString(test.total)) {
				t.Errorf("total = %s, want %s from the branch price and promotion", priced.Version.Total, test.total)
			}
			discounts := e.storedDiscounts(t, draft.Version.ID)
			if len(discounts) != 1 || discounts[test.name].promotionID == nil ||
				*discounts[test.name].promotionID != test.promotionID {
				t.Errorf("discounts = %+v, want only this branch's promotion", discounts)
			}
		})
	}
}

func TestInboundChannelFlow_ClosedAccountsAndBranchesDoNotCreateOrders(t *testing.T) {
	for _, test := range []struct {
		name, close, reopen string
	}{
		{"closed account", `UPDATE account SET is_active = FALSE WHERE id = $1`,
			`UPDATE account SET is_active = TRUE WHERE id = $1`},
		{"closed branch", `UPDATE branch SET is_active = FALSE WHERE id = $1`,
			`UPDATE branch SET is_active = TRUE WHERE id = $1`},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := newEnv(t)
			ctx := context.Background()
			accountID, branchID := e.seedAccount(t, test.name)
			channel := e.seedWhatsAppInbox(t, accountID, branchID)
			id := branchID
			if test.name == "closed account" {
				id = accountID
			}
			if _, err := e.db.CrossAccount().Exec(ctx, test.close, id); err != nil {
				t.Fatalf("close destination: %v", err)
			}
			unit := "unidad"
			pipeline := e.pipeline(t, stagedExtractor{lines: []domain.ExtractedRFQLine{{
				RequestedDescription: "2 bolsas de cemento", Quantity: decimal.NewFromInt(2),
				Unit: &unit, Source: domain.QuantitySourceExplicit, QuantityRationale: "Two bags requested",
			}}}, map[string]int{"2 bolsas de cemento": 0})
			inbound := services.NewWhatsAppInboundService(e.db,
				services.NewInboundRoutingService(e.db, repository.NewChannelRepository()),
				repository.NewInboundChannelMessageRepository(), nil, pipeline, nil, nil)
			router := gin.New()
			router.POST("/webhook", handler.NewWhatsAppWebhookHandler("test-secret", 4096, inbound).Receive)
			body := []byte(fmt.Sprintf(`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{"metadata":{"phone_number_id":%q},"messages":[{"from":"5491112345678","id":"wamid.closed","type":"text","text":{"body":"2 bolsas de cemento"}}]}}]}]}`, channel.phoneNumberID))
			mac := hmac.New(sha256.New, []byte("test-secret"))
			_, _ = mac.Write(body)
			request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
			request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("webhook response = %d, want 500 so the provider can retry", response.Code)
			}
			var messages, rfqs int
			if err := e.db.CrossAccount().QueryRow(ctx,
				`SELECT (SELECT count(*) FROM inbound_channel_message WHERE account_id = $1),
				        (SELECT count(*) FROM rfq WHERE account_id = $1)`, accountID).Scan(&messages, &rfqs); err != nil {
				t.Fatalf("count intake: %v", err)
			}
			if messages != 0 || rfqs != 0 {
				t.Fatalf("messages/RFQs = %d/%d, want no intake for a closed destination", messages, rfqs)
			}
			if _, err := e.db.CrossAccount().Exec(ctx, test.reopen, id); err != nil {
				t.Fatalf("reopen destination: %v", err)
			}
			for range 2 {
				request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
				request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("webhook retry after reopening = %d, want 200", response.Code)
				}
			}
			if err := e.db.CrossAccount().QueryRow(ctx,
				`SELECT (SELECT count(*) FROM inbound_channel_message WHERE account_id = $1),
				        (SELECT count(*) FROM rfq WHERE account_id = $1)`, accountID).Scan(&messages, &rfqs); err != nil {
				t.Fatalf("count retried intake: %v", err)
			}
			if messages != 1 || rfqs != 1 {
				t.Fatalf("retried messages/RFQs = %d/%d, want one of each after reopening", messages, rfqs)
			}
		})
	}
}

func TestInboundChannelFlow_MixedDeliveryRetriesEveryDestinationAfterReopening(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	firstAccount, firstBranch := e.seedAccount(t, "First inbound account")
	secondAccount, secondBranch := e.seedAccount(t, "Second inbound account")
	first := e.seedWhatsAppInbox(t, firstAccount, firstBranch)
	second := e.seedWhatsAppInbox(t, secondAccount, secondBranch)
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE branch SET is_active = FALSE WHERE id = $1`, secondBranch); err != nil {
		t.Fatalf("close destination: %v", err)
	}
	unit := "unidad"
	pipeline := e.pipeline(t, stagedExtractor{lines: []domain.ExtractedRFQLine{{
		RequestedDescription: "2 bolsas de cemento", Quantity: decimal.NewFromInt(2),
		Unit: &unit, Source: domain.QuantitySourceExplicit, QuantityRationale: "Two bags requested",
	}}}, map[string]int{"2 bolsas de cemento": 0})
	inbound := services.NewWhatsAppInboundService(e.db,
		services.NewInboundRoutingService(e.db, repository.NewChannelRepository()),
		repository.NewInboundChannelMessageRepository(), nil, pipeline, nil, nil)
	router := gin.New()
	router.POST("/webhook", handler.NewWhatsAppWebhookHandler("test-secret", 4096, inbound).Receive)
	body := []byte(fmt.Sprintf(`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{"metadata":{"phone_number_id":%q},"messages":[{"from":"5491112345678","id":"wamid.first","type":"text","text":{"body":"2 bolsas de cemento"}}]}},{"field":"messages","value":{"metadata":{"phone_number_id":%q},"messages":[{"from":"5491112345678","id":"wamid.second","type":"text","text":{"body":"2 bolsas de cemento"}}]}}]}]}`, first.phoneNumberID, second.phoneNumberID))
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write(body)
	deliver := func() int {
		request := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response.Code
	}
	if got := deliver(); got != http.StatusInternalServerError {
		t.Fatalf("mixed delivery = %d, want 500 so the provider can retry", got)
	}
	var count int
	for _, target := range []struct {
		account uuid.UUID
		want    int
	}{{firstAccount, 1}, {secondAccount, 0}} {
		var rfqs int
		if err := e.db.CrossAccount().QueryRow(ctx,
			`SELECT (SELECT count(*) FROM inbound_channel_message WHERE account_id = $1),
			        (SELECT count(*) FROM rfq WHERE account_id = $1)`, target.account).Scan(&count, &rfqs); err != nil {
			t.Fatal(err)
		}
		if count != target.want || rfqs != target.want {
			t.Fatalf("messages/RFQs for account %s = %d/%d, want %d of each", target.account,
				count, rfqs, target.want)
		}
	}
	if _, err := e.db.CrossAccount().Exec(ctx,
		`UPDATE branch SET is_active = TRUE WHERE id = $1`, secondBranch); err != nil {
		t.Fatalf("reopen destination: %v", err)
	}
	for range 2 {
		if got := deliver(); got != http.StatusOK {
			t.Fatalf("mixed delivery after reopening = %d, want 200", got)
		}
	}
	for _, target := range []struct{ account, branch, channel uuid.UUID }{
		{firstAccount, firstBranch, first.id}, {secondAccount, secondBranch, second.id},
	} {
		if err := e.db.CrossAccount().QueryRow(ctx,
			`SELECT count(*) FROM rfq WHERE account_id = $1 AND branch_id = $2 AND channel_id = $3`,
			target.account, target.branch, target.channel).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("RFQs for account/branch/channel %s/%s/%s = %d, want 1", target.account,
				target.branch, target.channel, count)
		}
	}
}
