package services

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// rfqRepository is the RFQ persistence surface the service needs.
type rfqRepository interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewRFQ) (*domain.RFQ, error)
	UpdateStatus(ctx context.Context, q repository.Querier, accountID, id uuid.UUID, status domain.RFQStatus) (*domain.RFQ, error)
	AppendStatusChange(ctx context.Context, q repository.Querier, accountID, rfqID uuid.UUID, previousStatus *domain.RFQStatus, newStatus domain.RFQStatus, userID *uuid.UUID) (*domain.RFQStatusChange, error)
}

// quoteDraftRepository is the quote persistence surface for creating draft versions.
type quoteDraftRepository interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewQuote) (*domain.Quote, error)
	UpdateCurrentVersion(ctx context.Context, q repository.Querier, accountID, quoteID, versionID uuid.UUID) (*domain.Quote, error)
	CreateVersion(ctx context.Context, q repository.Querier, accountID uuid.UUID, in domain.NewQuoteVersion) (*domain.QuoteVersion, error)
	CreateItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID, items []domain.NewQuoteItem) ([]domain.QuoteItem, error)
	AppendStatusChange(ctx context.Context, q repository.Querier, accountID, quoteID uuid.UUID, previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus, userID *uuid.UUID) (*domain.QuoteStatusChange, error)
}

// rfqChannelReader is the channel validation surface the RFQ flow needs.
type rfqChannelReader interface {
	ListActiveByType(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, channelType domain.ChannelType) ([]domain.Channel, error)
	GetActiveByID(ctx context.Context, q repository.Querier, accountID, branchID, channelID uuid.UUID) (*domain.Channel, error)
}

// RFQService owns the text RFQ pipeline up to a seller-reviewable quote draft.
type RFQService struct {
	db        tenantTxRunner
	rfqs      rfqRepository
	quotes    quoteDraftRepository
	channels  rfqChannelReader
	extractor domain.RFQExtractor
}

// NewRFQService builds an RFQService.
func NewRFQService(
	db tenantTxRunner, rfqs rfqRepository, quotes quoteDraftRepository,
	channels rfqChannelReader, extractor domain.RFQExtractor,
) *RFQService {
	return &RFQService{
		db: db, rfqs: rfqs, quotes: quotes, channels: channels, extractor: extractor,
	}
}

// CreateTextDraft turns plain RFQ text into a quote DRAFT for seller review.
func (s *RFQService) CreateTextDraft(
	ctx context.Context, tenant domain.Tenant, in domain.TextRFQDraftInput,
) (*domain.TextRFQDraft, error) {
	if err := requireBranch(tenant, "rfq draft"); err != nil {
		return nil, err
	}
	normalized, err := normalizeTextRFQDraftInput(in)
	if err != nil {
		return nil, err
	}
	if s.extractor == nil {
		return nil, fmt.Errorf("%w: rfq extractor is not configured", domain.ErrInvalidInput)
	}
	if s.channels == nil {
		return nil, fmt.Errorf("%w: channel repository is not configured", domain.ErrInvalidInput)
	}
	if _, err := s.getActiveChannel(ctx, tenant, normalized.ChannelID); err != nil {
		return nil, err
	}
	return s.createTextDraft(ctx, tenant, normalized)
}

// CreateWhatsAppMockDraft simulates an inbound WhatsApp text message in development.
func (s *RFQService) CreateWhatsAppMockDraft(
	ctx context.Context, tenant domain.Tenant, in domain.WhatsAppMockRFQInput,
) (*domain.TextRFQDraft, error) {
	if err := requireBranch(tenant, "WhatsApp mock"); err != nil {
		return nil, err
	}
	from, err := requiredText(in.From, "from")
	if err != nil {
		return nil, err
	}
	if err := requireMaxRunes(from, "from", 64); err != nil {
		return nil, err
	}
	profileName, err := optionalLimitedText(in.ProfileName, "profile_name", 160)
	if err != nil {
		return nil, err
	}
	raw, err := requiredText(in.Text, "text")
	if err != nil {
		return nil, err
	}
	if s.extractor == nil {
		return nil, fmt.Errorf("%w: rfq extractor is not configured", domain.ErrInvalidInput)
	}
	if s.channels == nil {
		return nil, fmt.Errorf("%w: channel repository is not configured", domain.ErrInvalidInput)
	}

	channel, err := s.resolveWhatsAppChannel(ctx, tenant, in.ChannelID)
	if err != nil {
		return nil, err
	}
	clientLabel := from
	if profileName != nil {
		clientLabel = fmt.Sprintf("%s (%s)", *profileName, from)
	}
	return s.createTextDraft(ctx, tenant, domain.TextRFQDraftInput{
		ChannelID: channel.ID, ClientLabel: &clientLabel, RawText: raw,
	})
}

func (s *RFQService) createTextDraft(
	ctx context.Context, tenant domain.Tenant, in domain.TextRFQDraftInput,
) (*domain.TextRFQDraft, error) {
	lines, err := s.extractor.Extract(ctx, in.RawText)
	if err != nil {
		return nil, err
	}
	items, err := newQuoteItemsFromRFQLines(lines)
	if err != nil {
		return nil, err
	}

	var draft domain.TextRFQDraft
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, channelErr := s.channels.GetActiveByID(ctx, q, tenant.AccountID, tenant.BranchID,
			in.ChannelID); channelErr != nil {
			return channelErr
		}
		rfq, createRFQErr := s.rfqs.Create(ctx, q, tenant.AccountID, domain.NewRFQ{
			BranchID:    tenant.BranchID,
			ClientID:    in.ClientID,
			ChannelID:   in.ChannelID,
			RawText:     &in.RawText,
			Status:      domain.RFQStatusReceived,
			WorkType:    in.WorkType,
			ClientLabel: in.ClientLabel,
		})
		if createRFQErr != nil {
			return createRFQErr
		}

		sellerID := tenant.UserID
		quote, createQuoteErr := s.quotes.Create(ctx, q, tenant.AccountID, domain.NewQuote{
			BranchID:      tenant.BranchID,
			ClientID:      in.ClientID,
			RFQID:         rfq.ID,
			SellerID:      &sellerID,
			CurrentStatus: domain.QuoteStatusDraft,
		})
		if createQuoteErr != nil {
			return createQuoteErr
		}

		version, createVersionErr := s.quotes.CreateVersion(ctx, q, tenant.AccountID,
			domain.NewQuoteVersion{
				QuoteID:       quote.ID,
				AuthorID:      &sellerID,
				VersionNumber: 1,
				Total:         decimal.Zero,
				IsImmutable:   false,
			})
		if createVersionErr != nil {
			return createVersionErr
		}

		createdItems, createItemsErr := s.quotes.CreateItems(ctx, q, tenant.AccountID,
			version.ID, items)
		if createItemsErr != nil {
			return createItemsErr
		}

		quote, updateQuoteErr := s.quotes.UpdateCurrentVersion(ctx, q, tenant.AccountID,
			quote.ID, version.ID)
		if updateQuoteErr != nil {
			return updateQuoteErr
		}

		previousRFQStatus := rfq.Status
		if _, appendRFQErr := s.rfqs.AppendStatusChange(ctx, q, tenant.AccountID, rfq.ID,
			&previousRFQStatus, domain.RFQStatusGenerated, &sellerID); appendRFQErr != nil {
			return appendRFQErr
		}
		if _, appendQuoteErr := s.quotes.AppendStatusChange(ctx, q, tenant.AccountID, quote.ID,
			nil, domain.QuoteStatusDraft, &sellerID); appendQuoteErr != nil {
			return appendQuoteErr
		}

		rfq, updateRFQErr := s.rfqs.UpdateStatus(ctx, q, tenant.AccountID, rfq.ID,
			domain.RFQStatusGenerated)
		if updateRFQErr != nil {
			return updateRFQErr
		}

		draft = domain.TextRFQDraft{
			RFQ:     *rfq,
			Quote:   *quote,
			Version: *version,
			Items:   createdItems,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &draft, nil
}

func (s *RFQService) getActiveChannel(
	ctx context.Context, tenant domain.Tenant, channelID uuid.UUID,
) (*domain.Channel, error) {
	var channel *domain.Channel
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var getErr error
		channel, getErr = s.channels.GetActiveByID(ctx, q, tenant.AccountID, tenant.BranchID,
			channelID)
		return getErr
	})
	if err != nil {
		return nil, err
	}
	return channel, nil
}

func (s *RFQService) resolveWhatsAppChannel(
	ctx context.Context, tenant domain.Tenant, channelID *uuid.UUID,
) (*domain.Channel, error) {
	if channelID != nil {
		if *channelID == uuid.Nil {
			return nil, fmt.Errorf("%w: channel_id must be a valid UUID", domain.ErrInvalidInput)
		}
		channel, err := s.getActiveChannel(ctx, tenant, *channelID)
		if err != nil {
			return nil, err
		}
		if channel.Type != domain.ChannelTypeWhatsApp {
			return nil, fmt.Errorf("%w: channel_id must identify a WhatsApp channel",
				domain.ErrInvalidInput)
		}
		return channel, nil
	}

	var channels []domain.Channel
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		channels, listErr = s.channels.ListActiveByType(ctx, q, tenant.AccountID,
			tenant.BranchID, domain.ChannelTypeWhatsApp)
		return listErr
	})
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("%w: no active WhatsApp channel for the selected branch",
			domain.ErrNotFound)
	}
	if len(channels) > 1 {
		return nil, fmt.Errorf("%w: channel_id is required when the branch has multiple WhatsApp channels",
			domain.ErrInvalidInput)
	}
	return &channels[0], nil
}

func normalizeTextRFQDraftInput(
	in domain.TextRFQDraftInput,
) (domain.TextRFQDraftInput, error) {
	if in.ChannelID == uuid.Nil {
		return domain.TextRFQDraftInput{}, fmt.Errorf("%w: channel_id is required",
			domain.ErrInvalidInput)
	}
	raw, err := requiredText(in.RawText, "raw_text")
	if err != nil {
		return domain.TextRFQDraftInput{}, err
	}
	clientLabel, err := optionalLimitedText(in.ClientLabel, "client_label", 255)
	if err != nil {
		return domain.TextRFQDraftInput{}, err
	}
	workType, err := optionalLimitedText(in.WorkType, "work_type", 255)
	if err != nil {
		return domain.TextRFQDraftInput{}, err
	}
	in.RawText = raw
	in.ClientLabel = clientLabel
	in.WorkType = workType
	return in, nil
}

func newQuoteItemsFromRFQLines(lines []domain.ExtractedRFQLine) ([]domain.NewQuoteItem, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: extractor returned no line items", domain.ErrInvalidInput)
	}

	items := make([]domain.NewQuoteItem, 0, len(lines))
	for i, line := range lines {
		field := fmt.Sprintf("items[%d]", i)
		description, err := requiredText(line.RequestedDescription, field+".requested_description")
		if err != nil {
			return nil, err
		}
		if err := requireMaxRunes(description, field+".requested_description", 512); err != nil {
			return nil, err
		}
		if !line.Quantity.IsPositive() {
			return nil, fmt.Errorf("%w: %s.quantity must be positive", domain.ErrInvalidInput, field)
		}
		if err := validateAmount(line.Quantity, field+".quantity"); err != nil {
			return nil, err
		}
		unit, err := optionalLimitedText(line.Unit, field+".unit", 64)
		if err != nil {
			return nil, err
		}
		rationale, err := optionalLimitedText(line.QuantityRationale, field+".quantity_rationale", 512)
		if err != nil {
			return nil, err
		}

		items = append(items, domain.NewQuoteItem{
			RequestedDescription: description,
			Quantity:             line.Quantity,
			Unit:                 unit,
			MatchStatus:          domain.ItemMatchStatusNoMatch,
			QuantityRationale:    rationale,
		})
	}
	return items, nil
}

func optionalLimitedText(raw *string, field string, max int) (*string, error) {
	trimmed := optionalText(raw)
	if trimmed == nil {
		return nil, nil
	}
	if err := requireMaxRunes(*trimmed, field, max); err != nil {
		return nil, err
	}
	return trimmed, nil
}

func requireMaxRunes(value, field string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%w: %s cannot exceed %d characters", domain.ErrInvalidInput, field, max)
	}
	return nil
}
