package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

const (
	minQuoteValidityDays = 1
	maxQuoteValidityDays = 365
)

var e164PhonePattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// sellerSendBody is the message a send carries when no representation supplies one.
const sellerSendBody = "Tu cotización está lista. Podés verla en " + domain.QuotePublicURLPlaceholder

type quoteDeliveryRepository interface {
	ListByOperation(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		key uuid.UUID) ([]domain.QuoteSend, error)
	CreateBatch(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		sends []domain.NewQuoteSend) ([]domain.QuoteSend, error)
	CompleteBatch(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		outcomes []domain.QuoteSendOutcome) error
	GetAccountIDByPublicToken(ctx context.Context, q repository.Querier,
		token string) (uuid.UUID, error)
	GetPublicByToken(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		token string) (*domain.QuoteSend, error)
}

type quoteDeliveryHistoryReader interface {
	ListByQuote(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) ([]domain.QuoteSend, error)
}

// quoteStatusHistoryReader marks where a send cycle starts: a reactivated quote re-enters QUOTED
// with the same version, and the sends before that no longer count as «used».
type quoteStatusHistoryReader interface {
	ListStatusChanges(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) ([]domain.QuoteStatusChange, error)
}

// quoteClientDeliveryReader finds the destinations a client's last sent quote reached.
type quoteClientDeliveryReader interface {
	ListClientLastQuoteSends(ctx context.Context, q repository.Querier, accountID, branchID, clientID,
		excludeQuoteID uuid.UUID) ([]domain.QuoteSend, error)
}

type quoteDeliveryQuoteRepository interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID,
		id uuid.UUID) (*domain.Quote, error)
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID, branchID,
		id uuid.UUID) (*domain.Quote, error)
	GetByVersionID(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) (*domain.QuoteVersion, error)
	CreateVersion(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewQuoteVersion) (*domain.QuoteVersion, error)
	UpdateCurrentVersion(ctx context.Context, q repository.Querier, accountID, quoteID,
		versionID uuid.UUID) (*domain.Quote, error)
	ListItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID) ([]domain.QuoteItem, error)
	CreateItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		items []domain.NewQuoteItem) ([]domain.QuoteItem, error)
	ListAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		itemIDs []uuid.UUID) (map[uuid.UUID][]domain.QuoteItemAlternative, error)
	MarkAlternativesChosenByClient(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		selections []domain.ClientAlternativeSelection) error
	CreateAlternatives(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		alternatives []domain.NewQuoteItemAlternative) error
	DeleteAlternativesByItemIDs(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		itemIDs []uuid.UUID) error
	DeleteItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		itemIDs []uuid.UUID) error
	UpdateItemQuantities(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		updates []domain.QuoteItemQuantityUpdate) error
	ReplaceItems(ctx context.Context, q repository.Querier, accountID, versionID uuid.UUID,
		replacements []domain.QuoteItemReplacement) error
	FreezeVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		versionID uuid.UUID) (*domain.QuoteVersion, error)
	SetExpiry(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID, expiresAt time.Time) error
	UpdateStatus(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID,
		from, to domain.QuoteStatus) (*domain.Quote, error)
	AppendStatusChange(ctx context.Context, q repository.Querier, accountID, quoteID uuid.UUID,
		previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus,
		userID *uuid.UUID) (*domain.QuoteStatusChange, error)
}

type quoteDeliveryChannelRepository interface {
	ListActiveByBranch(ctx context.Context, q repository.Querier, accountID,
		branchID uuid.UUID) ([]domain.Channel, error)
}

type quoteDeliveryRFQRepository interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID,
		rfqID uuid.UUID) (*domain.RFQ, error)
}

type quoteDeliveryChannelConfigReader interface {
	GetActiveConfigurationsByIDs(ctx context.Context, q repository.Querier, accountID,
		branchID uuid.UUID, channelIDs []uuid.UUID) (map[uuid.UUID][]byte, error)
}

type channelConfigOpener interface {
	Open(sealed string) (string, error)
}

type quoteDeliveryBranchRepository interface {
	GetByID(ctx context.Context, q repository.Querier, accountID,
		branchID uuid.UUID) (*domain.Branch, error)
}

type quoteEmailSender interface {
	Send(ctx context.Context, out OutboundMail) error
}

type quoteRepresentationEnsurer interface {
	Ensure(ctx context.Context, tenant domain.Tenant,
		quoteID uuid.UUID) (*domain.QuoteRepresentationResult, error)
	ResolvePublic(ctx context.Context, accountID, versionID uuid.UUID, expiresAt time.Time,
		publicURL string) (*domain.PublicQuoteRepresentation, error)
}

type quoteDeliveryRepresentationReader interface {
	GetByVersionID(ctx context.Context, q repository.Querier, accountID,
		versionID uuid.UUID) (*domain.QuoteRepresentation, error)
}

type quotePublicDB interface {
	tenantTxRunner
	CrossAccount() repository.Querier
	WithAdvisoryLock(ctx context.Context, key string, fn func() error) error
}

// QuoteDeliveryService freezes and delivers a seller-approved quote, then labels it post-commit.
type QuoteDeliveryService struct {
	clients             quoteClientConfirmer
	db                  quotePublicDB
	sends               quoteDeliveryRepository
	quotes              quoteDeliveryQuoteRepository
	rfqs                quoteDeliveryRFQRepository
	messages            quoteMessageWriter
	channels            quoteDeliveryChannelRepository
	branches            quoteDeliveryBranchRepository
	prices              branchPriceReader
	whatsapp            domain.QuoteWhatsAppSender
	email               quoteEmailSender
	evaluator           QuoteQualityEvaluator
	representations     quoteRepresentationEnsurer
	selectionSnapshots  quoteDeliveryRepresentationReader
	actions             clientActionRecorder
	sellers             quoteSellerReader
	windows             changeRequestQueue
	draftCarry          *draftCarry
	webappURL           string
	customerHostnames   customerQuoteHostnameReader
	customerLocalOrigin string
	channelOpener       channelConfigOpener
	portalContacts      quotePortalContactReader
	recentSenders       quoteRecentSenderReader
	templates           domain.QuoteWhatsAppTemplates
	now                 func() time.Time
	log                 *slog.Logger
}

// WithWhatsAppCredentials makes encrypted channel credentials available only for delivery calls.
func (s *QuoteDeliveryService) WithWhatsAppCredentials(
	opener channelConfigOpener,
) *QuoteDeliveryService {
	s.channelOpener = opener
	return s
}

// WithRepresentationService makes immutable output generation a prerequisite for delivery.
func (s *QuoteDeliveryService) WithRepresentationService(
	representations quoteRepresentationEnsurer,
) *QuoteDeliveryService {
	s.representations = representations
	return s
}

// WithSelectionSnapshots supplies the private commercial snapshot attached to a sent offer.
func (s *QuoteDeliveryService) WithSelectionSnapshots(
	representations quoteDeliveryRepresentationReader,
) *QuoteDeliveryService {
	s.selectionSnapshots = representations
	return s
}

// NewQuoteDeliveryService builds the delivery orchestrator.
func NewQuoteDeliveryService(db quotePublicDB, sends quoteDeliveryRepository,
	quotes quoteDeliveryQuoteRepository, rfqs quoteDeliveryRFQRepository, channels quoteDeliveryChannelRepository,
	branches quoteDeliveryBranchRepository, prices branchPriceReader,
	whatsapp domain.QuoteWhatsAppSender,
	email quoteEmailSender, evaluator QuoteQualityEvaluator, webappURL string,
	now func() time.Time, log *slog.Logger) *QuoteDeliveryService {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &QuoteDeliveryService{db: db, sends: sends, quotes: quotes, rfqs: rfqs,
		channels: channels, branches: branches, prices: prices, whatsapp: whatsapp,
		email: email, evaluator: evaluator, webappURL: strings.TrimRight(webappURL, "/"),
		now: now, log: log}
}

// Send freezes the final version, attempts each selected channel independently, commits the
// successful result, and only then evaluates what the seller actually sent.
func (s *QuoteDeliveryService) Send(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
	in domain.QuoteDeliveryInput) (*domain.QuoteDeliveryResult, error) {
	if err := requireBranch(tenant, "a quote delivery"); err != nil {
		return nil, err
	}
	if s.clients == nil || s.rfqs == nil {
		return nil, domain.ErrNotConfigured
	}
	normalized, err := normalizeQuoteDeliveryInput(in)
	if err != nil {
		return nil, err
	}
	var result *domain.QuoteDeliveryResult
	err = s.db.WithAdvisoryLock(ctx, strings.Join([]string{"quote-delivery",
		tenant.AccountID.String(), quoteID.String()}, ":"),
		func() error {
			var sendErr error
			result, sendErr = s.sendLocked(ctx, tenant, quoteID, normalized)
			return sendErr
		})
	return result, err
}

func (s *QuoteDeliveryService) sendLocked(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID, normalized domain.QuoteDeliveryInput) (*domain.QuoteDeliveryResult, error) {
	if s.representations == nil {
		return nil, domain.ErrNotConfigured
	}
	bundle, err := s.representations.Ensure(ctx, tenant, quoteID)
	if err != nil {
		return nil, err
	}
	prepared, quote, version, err := s.prepare(ctx, tenant, quoteID, normalized, bundle.Representation.VersionID)
	if errors.Is(err, domain.ErrConflict) && domain.CodeOf(err) == domain.CodeConflict {
		prepared, quote, version, err = s.loadConcurrentReplay(ctx, tenant, quoteID, normalized)
	}
	if err != nil {
		return nil, err
	}
	if version.ID != bundle.Representation.VersionID {
		return nil, domain.WithCode(domain.CodeQuoteNotSendable, domain.ErrConflict)
	}
	base, err := s.customerQuoteBase(ctx, tenant.AccountID)
	if err != nil {
		return nil, err
	}
	for i := range prepared {
		prepared[i].PublicURL = customerQuoteURL(base, prepared[i].PublicToken)
	}
	if complete(prepared) {
		return replayResult(*quote, *version, prepared)
	}

	// Set before dispatch so the validity a message states is the expiry the quote stores.
	expiresAt := s.now().UTC().AddDate(0, 0, prepared[0].ValidityDays)
	for _, delivery := range prepared {
		if delivery.ExpiresAt != nil && hasSuccessfulSend([]domain.QuoteSend{delivery}) {
			expiresAt = *delivery.ExpiresAt
			break
		}
	}
	if quote.ExpiresAt != nil && quote.CurrentStatus == domain.QuoteStatusSent {
		expiresAt = *quote.ExpiresAt
	}
	messages, err := s.sendMessages(ctx, tenant, prepared, bundle, normalized.FollowupMessage, base, expiresAt)
	if err != nil {
		return nil, err
	}
	outcomes := s.dispatch(ctx, tenant, *quote, prepared, bundle, messages)
	completedAt := s.now().UTC()
	anySuccess := hasSuccessfulSend(prepared)
	for i := range outcomes {
		outcomes[i].ExpiresAt = &expiresAt
		if outcomes[i].Status == domain.SendTrackingStatusSent {
			outcomes[i].SentAt = &completedAt
			anySuccess = true
		}
	}

	commitCtx := context.WithoutCancel(ctx)
	err = s.db.InTenantTx(commitCtx, tenant, func(q repository.Querier) error {
		if len(outcomes) > 0 {
			if err := s.sends.CompleteBatch(commitCtx, q, tenant.AccountID, outcomes); err != nil {
				return err
			}
		}
		if !anySuccess {
			return nil
		}
		current, err := s.quotes.GetByIDForUpdate(commitCtx, q, tenant.AccountID, tenant.BranchID, quote.ID)
		if err != nil {
			return err
		}
		if err := s.clients.ConfirmForSend(commitCtx, q, tenant, *current, normalized.ClientAssociation); err != nil {
			return err
		}
		if err := s.quotes.SetExpiry(commitCtx, q, tenant.AccountID, tenant.BranchID,
			quote.ID, expiresAt); err != nil {
			return err
		}
		current, err = s.quotes.GetByID(commitCtx, q, tenant.AccountID, tenant.BranchID,
			quote.ID)
		if err != nil {
			return err
		}
		quote = current
		if current.CurrentStatus == domain.QuoteStatusQuoted {
			previous := current.CurrentStatus
			updated, updateErr := s.quotes.UpdateStatus(commitCtx, q, tenant.AccountID,
				tenant.BranchID, current.ID, previous, domain.QuoteStatusSent)
			if updateErr != nil {
				return updateErr
			}
			quote = updated
			if _, updateErr = s.quotes.AppendStatusChange(commitCtx, q, tenant.AccountID,
				current.ID, &previous, domain.QuoteStatusSent, &tenant.UserID); updateErr != nil {
				return updateErr
			}
		} else if current.CurrentStatus != domain.QuoteStatusSent {
			return domain.WithCode(domain.CodeQuoteNotSendable, domain.ErrConflict)
		}
		return s.recordSellerSends(commitCtx, q, tenant, quote.ID, prepared, outcomes, messages,
			completedAt)
	})
	if err != nil {
		return nil, err
	}

	applyOutcomes(prepared, outcomes)
	if !anySuccess {
		return nil, domain.ErrDeliveryUnavailable
	}
	quote.ExpiresAt = &expiresAt
	result := &domain.QuoteDeliveryResult{QuoteID: quote.ID, VersionID: version.ID,
		CurrentStatus: quote.CurrentStatus, ExpiresAt: &expiresAt, Deliveries: prepared}
	if normalized.FollowupMessage == nil {
		s.evaluateAfterCommit(ctx, tenant, quote.ID, version.ID)
	}
	return result, nil
}

// recordSellerSends writes each successful send into the quote's conversation as the seller's
// message. The body keeps the public-link placeholder: the token is a credential and never stored
// twice.
func (s *QuoteDeliveryService) recordSellerSends(ctx context.Context, q repository.Querier,
	tenant domain.Tenant, quoteID uuid.UUID, sends []domain.QuoteSend,
	outcomes []domain.QuoteSendOutcome, messages map[uuid.UUID]quoteSendMessage, at time.Time,
) error {
	if s.messages == nil {
		return nil
	}
	channels := make(map[uuid.UUID]uuid.UUID, len(sends))
	for _, send := range sends {
		if send.ChannelType != domain.ChannelTypeWebApp {
			channels[send.ID] = send.ChannelID
		}
	}
	for _, outcome := range outcomes {
		if outcome.Status != domain.SendTrackingStatusSent {
			continue
		}
		channelID, external := channels[outcome.ID]
		if !external {
			continue
		}
		body := sellerSendBody
		if message, found := messages[outcome.ID]; found {
			body = message.Body
		}
		if _, err := s.messages.Create(ctx, q, tenant.AccountID, tenant.BranchID,
			domain.NewQuoteMessage{ID: uuid.New(), QuoteID: quoteID, ChannelID: &channelID,
				AuthorType: domain.MessageAuthorSeller, AuthorUserID: &tenant.UserID, Body: body,
				ReceivedAt: at}); err != nil {
			return err
		}
	}
	return nil
}

func (s *QuoteDeliveryService) prepare(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID, in domain.QuoteDeliveryInput, representedVersionID uuid.UUID) ([]domain.QuoteSend, *domain.Quote,
	*domain.QuoteVersion, error) {
	var prepared []domain.QuoteSend
	var quote *domain.Quote
	var version *domain.QuoteVersion
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		quote, err = s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if err := requireUnarchived(*quote); err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusQuoted &&
			quote.CurrentStatus != domain.QuoteStatusSent {
			return domain.WithCode(domain.CodeQuoteNotSendable, domain.ErrConflict)
		}
		version, err = s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if in.FollowupMessage != nil && quote.CurrentStatus != domain.QuoteStatusSent {
			return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
		}
		if version.ID != representedVersionID || !version.IsImmutable {
			return domain.WithCode(domain.CodeQuoteNotSendable, domain.ErrConflict)
		}

		prepared, err = s.sends.ListByOperation(ctx, q, tenant.AccountID, tenant.BranchID,
			quoteID, in.IdempotencyKey)
		if err != nil {
			return err
		}
		branch, err := s.branches.GetByID(ctx, q, tenant.AccountID, tenant.BranchID)
		if err != nil {
			return err
		}
		validityDays := branch.DefaultExpiryDays
		if in.ExpiryDays != nil {
			validityDays = *in.ExpiryDays
		}
		if err := validateValidityDays(validityDays); err != nil {
			return err
		}
		if len(prepared) > 0 {
			return validateReplay(prepared, version.ID, in, validityDays)
		}

		mode := in.Mode
		if mode == "" {
			mode = domain.QuoteDeliveryModeResend
			if quote.CurrentStatus == domain.QuoteStatusQuoted {
				mode = domain.QuoteDeliveryModeFirst
			}
		}
		if mode == domain.QuoteDeliveryModeFirst && quote.CurrentStatus != domain.QuoteStatusQuoted {
			return domain.WithCode(domain.CodeQuoteNotSendable, domain.ErrConflict)
		}
		if mode != domain.QuoteDeliveryModeFirst && quote.CurrentStatus != domain.QuoteStatusSent {
			return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
		}
		if mode == domain.QuoteDeliveryModeOtherMedium {
			used, err := s.cycleSends(ctx, q, tenant, quoteID, version.ID)
			if err != nil {
				return err
			}
			for _, send := range used {
				if (send.ChannelType == domain.ChannelTypeWhatsApp && len(deliveryPhones(in)) > 0) ||
					(send.ChannelType == domain.ChannelTypeEmail && len(deliveryEmails(in)) > 0) {
					return domain.WithCode(domain.CodeChannelAlreadyUsed, domain.ErrConflict)
				}
			}
		}
		if s.clients == nil {
			return domain.ErrNotConfigured
		}
		if err := s.clients.ValidateForSend(ctx, q, tenant, *quote, in.ClientAssociation); err != nil {
			return err
		}
		rfq, err := s.rfqs.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quote.RFQID)
		if err != nil {
			return err
		}
		selected, err := s.selectedChannels(ctx, q, tenant, in, rfq.ChannelID)
		if err != nil {
			return err
		}
		if err := s.requireWhatsAppReach(ctx, q, tenant, selected, deliveryPhones(in),
			in.FollowupMessage != nil); err != nil {
			return err
		}
		news := make([]domain.NewQuoteSend, 0, len(selected))
		for _, selectedChannel := range selected {
			destinations := []string{"customer-portal"}
			if selectedChannel.Type == domain.ChannelTypeWhatsApp {
				destinations = deliveryPhones(in)
			}
			if selectedChannel.Type == domain.ChannelTypeEmail {
				destinations = deliveryEmails(in)
			}
			for _, destination := range destinations {
				token, err := newPublicToken()
				if err != nil {
					return err
				}
				purpose := domain.QuoteDeliveryPurposeQuote
				if in.FollowupMessage != nil {
					purpose = domain.QuoteDeliveryPurposeFollowup
				}
				news = append(news, domain.NewQuoteSend{
					ID: uuid.New(), VersionID: version.ID, ChannelID: selectedChannel.ID,
					IdempotencyKey: in.IdempotencyKey, Destination: destination, PublicToken: token,
					Format: domain.SendFormatWebAppLink, ValidityDays: validityDays,
					ClientAssociationHash: sendClientFingerprint(in.ClientAssociation),
					Purpose:               purpose, Message: in.FollowupMessage,
				})
			}
		}

		prepared, err = s.sends.CreateBatch(ctx, q, tenant.AccountID, news)
		return err
	})
	return prepared, quote, version, err
}

// Preset returns what the send dialog preselects for mode and the destinations the current send
// cycle already reached. clientID is the client chosen in the dialog, when there is one.
func (s *QuoteDeliveryService) Preset(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
	mode domain.QuoteDeliveryMode, clientID *uuid.UUID) (*domain.QuoteDeliveryPreset, error) {
	if err := requireBranch(tenant, "a delivery preset"); err != nil {
		return nil, err
	}
	if !mode.Valid() {
		return nil, fmt.Errorf("%w: unknown delivery mode", domain.ErrInvalidInput)
	}
	history, ok := s.sends.(quoteDeliveryHistoryReader)
	if !ok {
		return nil, domain.ErrNotConfigured
	}
	var preset domain.QuoteDeliveryPreset
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		version, err := s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		sends, err := history.ListByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		start, err := s.cycleStart(ctx, q, tenant, quoteID)
		if err != nil {
			return err
		}
		sameClient := clientID == nil || quote.ClientID == nil || *clientID == *quote.ClientID
		preset = deliveryPreset(mode, sends, version.ID, start, sameClient)
		client := clientID
		if client == nil {
			client = quote.ClientID
		}
		reader, ok := s.sends.(quoteClientDeliveryReader)
		if preset.Source != domain.QuoteDeliveryPresetNone || mode != domain.QuoteDeliveryModeFirst ||
			client == nil || !ok {
			return nil
		}
		last, err := reader.ListClientLastQuoteSends(ctx, q, tenant.AccountID, tenant.BranchID, *client, quoteID)
		if err != nil {
			return err
		}
		phones, emails := deliveryDestinations(last)
		if len(phones)+len(emails) > 0 {
			preset.Source, preset.Phones, preset.Emails = domain.QuoteDeliveryPresetClientLast, phones, emails
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &preset, nil
}

// cycleSends returns the successful quote sends of versionID since the quote last entered QUOTED.
func (s *QuoteDeliveryService) cycleSends(ctx context.Context, q repository.Querier, tenant domain.Tenant,
	quoteID, versionID uuid.UUID) ([]domain.QuoteSend, error) {
	history, ok := s.sends.(quoteDeliveryHistoryReader)
	if !ok {
		return nil, domain.ErrNotConfigured
	}
	sends, err := history.ListByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
	if err != nil {
		return nil, err
	}
	start, err := s.cycleStart(ctx, q, tenant, quoteID)
	if err != nil {
		return nil, err
	}
	return currentCycle(sends, versionID, start), nil
}

// cycleStart is when the quote last entered QUOTED; zero when the log is not available.
func (s *QuoteDeliveryService) cycleStart(ctx context.Context, q repository.Querier, tenant domain.Tenant,
	quoteID uuid.UUID) (time.Time, error) {
	reader, ok := s.quotes.(quoteStatusHistoryReader)
	if !ok {
		return time.Time{}, nil
	}
	changes, err := reader.ListStatusChanges(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
	if err != nil {
		return time.Time{}, err
	}
	var start time.Time
	for _, change := range changes {
		if change.NewStatus == domain.QuoteStatusQuoted && change.ChangedAt.After(start) {
			start = change.ChangedAt
		}
	}
	return start, nil
}

// ensureCurrentSendCycle prevents an unanswered historical link from closing a reactivated quote.
func (s *QuoteDeliveryService) ensureCurrentSendCycle(ctx context.Context, q repository.Querier,
	accountID uuid.UUID, quote domain.Quote, send domain.QuoteSend) error {
	start, err := s.cycleStart(ctx, q, domain.Tenant{AccountID: accountID, BranchID: quote.BranchID}, quote.ID)
	if err != nil {
		return err
	}
	published := send.CreatedAt
	if send.SentAt != nil {
		published = *send.SentAt
	}
	if published.Before(start) {
		return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
	}
	return nil
}

// deliveryPreset applies the preset rules: a resend and a reminder reuse what this cycle reached;
// a first send reuses the version's own earlier cycle, else the last sent version, but only while
// the dialog keeps the quote's client; «otro medio» starts from nothing.
func deliveryPreset(mode domain.QuoteDeliveryMode, sends []domain.QuoteSend, versionID uuid.UUID,
	cycleStart time.Time, sameClient bool) domain.QuoteDeliveryPreset {
	preset := domain.QuoteDeliveryPreset{Source: domain.QuoteDeliveryPresetNone}
	preset.UsedPhones, preset.UsedEmails = deliveryDestinations(currentCycle(sends, versionID, cycleStart))
	switch mode {
	case domain.QuoteDeliveryModeResend, domain.QuoteDeliveryModeFollowup:
		if len(preset.UsedPhones)+len(preset.UsedEmails) > 0 {
			preset.Source = domain.QuoteDeliveryPresetVersion
			preset.Phones, preset.Emails = preset.UsedPhones, preset.UsedEmails
		}
	case domain.QuoteDeliveryModeFirst:
		if !sameClient {
			return preset
		}
		successful := successfulQuoteSends(sends)
		var own, previous []domain.QuoteSend
		var previousVersion uuid.UUID
		for _, send := range successful {
			switch {
			case send.VersionID == versionID:
				own = append(own, send)
			case previousVersion == uuid.Nil || send.VersionID == previousVersion:
				previousVersion = send.VersionID
				previous = append(previous, send)
			}
		}
		if len(own) > 0 {
			preset.Source = domain.QuoteDeliveryPresetVersion
			preset.Phones, preset.Emails = deliveryDestinations(own)
		} else if len(previous) > 0 {
			preset.Source = domain.QuoteDeliveryPresetPreviousVersion
			preset.Phones, preset.Emails = deliveryDestinations(previous)
		}
	}
	return preset
}

// currentCycle keeps the successful quote sends of versionID made at or after start.
func currentCycle(sends []domain.QuoteSend, versionID uuid.UUID, start time.Time) []domain.QuoteSend {
	cycle := make([]domain.QuoteSend, 0, len(sends))
	for _, send := range successfulQuoteSends(sends) {
		at := send.CreatedAt
		if send.SentAt != nil {
			at = *send.SentAt
		}
		if send.VersionID == versionID && !at.Before(start) {
			cycle = append(cycle, send)
		}
	}
	return cycle
}

// successfulQuoteSends drops reminders and failed or pending attempts, keeping the input order.
func successfulQuoteSends(sends []domain.QuoteSend) []domain.QuoteSend {
	kept := make([]domain.QuoteSend, 0, len(sends))
	for _, send := range sends {
		if send.Purpose != domain.QuoteDeliveryPurposeFollowup && hasSuccessfulSend([]domain.QuoteSend{send}) {
			kept = append(kept, send)
		}
	}
	return kept
}

// deliveryDestinations lists the WhatsApp numbers and email addresses sends reached, each once, in
// the sends' order; portal publications have no destination of their own.
func deliveryDestinations(sends []domain.QuoteSend) (phones, emails []string) {
	phones, emails = []string{}, []string{}
	seen := map[string]bool{}
	for _, send := range sends {
		key := string(send.ChannelType) + ":" + send.Destination
		if send.Destination == "" || seen[key] {
			continue
		}
		seen[key] = true
		switch send.ChannelType {
		case domain.ChannelTypeWhatsApp:
			phones = append(phones, send.Destination)
		case domain.ChannelTypeEmail:
			emails = append(emails, send.Destination)
		}
	}
	return phones, emails
}

func (s *QuoteDeliveryService) selectedChannels(ctx context.Context, q repository.Querier,
	tenant domain.Tenant, in domain.QuoteDeliveryInput, originChannelID uuid.UUID) ([]domain.Channel, error) {
	active, err := s.channels.ListActiveByBranch(ctx, q, tenant.AccountID, tenant.BranchID)
	if err != nil {
		return nil, err
	}
	types := make([]domain.ChannelType, 0, 2)
	if len(deliveryPhones(in)) > 0 {
		types = append(types, domain.ChannelTypeWhatsApp)
	}
	if len(deliveryEmails(in)) > 0 {
		types = append(types, domain.ChannelTypeEmail)
	}
	if in.FollowupMessage == nil {
		types = append(types, domain.ChannelTypeWebApp)
	}
	selected := make([]domain.Channel, 0, len(types))
	for _, channelType := range types {
		var channels []domain.Channel
		for _, channel := range active {
			if channel.Type == channelType {
				channels = append(channels, channel)
			}
		}
		if len(channels) > 1 {
			for _, channel := range channels {
				if channel.ID == originChannelID {
					channels = []domain.Channel{channel}
					break
				}
			}
		}
		if len(channels) != 1 {
			return nil, domain.WithCode(domain.CodeDeliveryChannel,
				fmt.Errorf("%w: branch needs exactly one active %s channel", domain.ErrInvalidInput,
					channelType))
		}
		selected = append(selected, channels[0])
	}
	return selected, nil
}

func (s *QuoteDeliveryService) loadConcurrentReplay(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID, in domain.QuoteDeliveryInput) ([]domain.QuoteSend, *domain.Quote,
	*domain.QuoteVersion, error) {
	var sends []domain.QuoteSend
	var quote *domain.Quote
	var version *domain.QuoteVersion
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		quote, err = s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		version, err = s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID,
			quoteID)
		if err != nil {
			return err
		}
		sends, err = s.sends.ListByOperation(ctx, q, tenant.AccountID, tenant.BranchID, quoteID,
			in.IdempotencyKey)
		if err != nil {
			return err
		}
		if len(sends) == 0 {
			return domain.ErrConflict
		}
		branch, err := s.branches.GetByID(ctx, q, tenant.AccountID, tenant.BranchID)
		if err != nil {
			return err
		}
		validityDays := branch.DefaultExpiryDays
		if in.ExpiryDays != nil {
			validityDays = *in.ExpiryDays
		}
		if err := validateValidityDays(validityDays); err != nil {
			return err
		}
		return validateReplay(sends, version.ID, in, validityDays)
	})
	return sends, quote, version, err
}

func (s *QuoteDeliveryService) dispatch(ctx context.Context, tenant domain.Tenant,
	quote domain.Quote, sends []domain.QuoteSend,
	bundle *domain.QuoteRepresentationResult, messages map[uuid.UUID]quoteSendMessage,
) []domain.QuoteSendOutcome {
	credentials, credentialsErr := s.whatsAppCredentials(ctx, tenant, sends)
	if credentialsErr != nil {
		s.log.WarnContext(ctx, "whatsapp channel credentials are unavailable",
			slog.String("quote_id", quote.ID.String()), slog.Any("error", credentialsErr))
	}
	outcomes := make([]domain.QuoteSendOutcome, 0, len(sends))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range sends {
		if sends[i].TrackingStatus != domain.SendTrackingStatusPending {
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			send := sends[index]
			message, found := messages[send.ID]
			if !found {
				message = quoteSendMessage{Body: sellerSendBody, Link: send.PublicURL}
			}
			messageBody := strings.ReplaceAll(message.Body, domain.QuotePublicURLPlaceholder, message.Link)
			refusal := message.Refusal
			outcome := domain.QuoteSendOutcome{ID: send.ID,
				Status: domain.SendTrackingStatusFailed}
			var err error
			switch send.ChannelType {
			case domain.ChannelTypeWebApp:
			// Publication is committed with the send outcome, without an external provider.
			case domain.ChannelTypeWhatsApp:
				var receipt *domain.DeliveryReceipt
				message := domain.QuoteWhatsAppMessage{
					DeliveryID: send.ID, To: send.Destination,
					Body:      messageBody,
					PublicURL: message.Link,
					Template:  message.Template,
				}
				if refusal != nil {
					err = refusal
				} else if credentialsErr != nil {
					err = credentialsErr
				} else if credential, found := credentials[send.ChannelID]; found {
					message.Credentials = credential
					receipt, err = s.whatsapp.SendQuote(ctx, message)
				} else {
					receipt, err = s.whatsapp.SendQuote(ctx, message)
				}
				if err == nil && receipt != nil && receipt.ProviderReference != "" {
					outcome.ProviderReference = &receipt.ProviderReference
				}
			case domain.ChannelTypeEmail:
				clientID := quote.ClientID
				content := quoteEmailContent{Subject: "Tu cotización está lista",
					Heading:     "Tu cotización está lista",
					Paragraphs:  []string{"Revisá el detalle y la vigencia en la web."},
					ActionLabel: "Ver cotización"}
				if message.Email != nil {
					content = *message.Email
				}
				// The client's answer goes to the branch mailbox, not to the platform's sender.
				replyTo, replyToName, fromName := "", "", ""
				if send.ChannelIdentifier != nil {
					replyTo = *send.ChannelIdentifier
				}
				if bundle != nil {
					payload := bundle.Representation.Payload
					// The customer reads the corralón as the sender, never the platform.
					fromName = payload.Supplier.Name
					replyToName = payload.Supplier.Name + " — " + payload.Branch.Name
				}
				if send.Purpose == domain.QuoteDeliveryPurposeFollowup && send.Message != nil {
					content = quoteEmailContent{Subject: "Seguimiento de cotización",
						Heading: "Seguimiento de cotización", Paragraphs: []string{*send.Message},
						ActionLabel: "Ver cotización"}
				}
				err = s.email.Send(ctx, OutboundMail{AccountID: tenant.AccountID,
					UserID: &tenant.UserID, ClientID: clientID, QuoteID: &quote.ID,
					Event: domain.NotificationEventQuoteSent, To: send.Destination,
					FromName: fromName, ReplyTo: replyTo, ReplyToName: replyToName,
					Subject: content.Subject, Heading: content.Heading,
					Paragraphs: content.Paragraphs, Items: content.Items, Closing: content.Closing,
					ActionLabel: content.ActionLabel, ActionURL: message.Link,
					Attachments: message.Attachments})
			default:
				err = fmt.Errorf("unsupported delivery channel %s", send.ChannelType)
			}
			if err == nil {
				outcome.Status = domain.SendTrackingStatusSent
			} else {
				s.log.WarnContext(ctx, "quote delivery channel failed",
					slog.String("quote_id", quote.ID.String()), slog.String("send_id", send.ID.String()),
					slog.String("channel", string(send.ChannelType)), slog.Any("error", err))
			}
			mu.Lock()
			outcomes = append(outcomes, outcome)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	return outcomes
}

func (s *QuoteDeliveryService) whatsAppCredentials(ctx context.Context, tenant domain.Tenant,
	sends []domain.QuoteSend,
) (map[uuid.UUID]domain.WhatsAppDeliveryCredentials, error) {
	reader, canRead := s.channels.(quoteDeliveryChannelConfigReader)
	if s.channelOpener == nil || !canRead {
		return map[uuid.UUID]domain.WhatsAppDeliveryCredentials{}, nil
	}
	channelIDs := make([]uuid.UUID, 0, len(sends))
	for _, send := range sends {
		if send.ChannelType == domain.ChannelTypeWhatsApp {
			channelIDs = append(channelIDs, send.ChannelID)
		}
	}
	if len(channelIDs) == 0 {
		return map[uuid.UUID]domain.WhatsAppDeliveryCredentials{}, nil
	}

	var rawConfigs map[uuid.UUID][]byte
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var readErr error
		rawConfigs, readErr = reader.GetActiveConfigurationsByIDs(ctx, q, tenant.AccountID,
			tenant.BranchID, channelIDs)
		return readErr
	}); err != nil {
		return nil, err
	}

	credentials := make(map[uuid.UUID]domain.WhatsAppDeliveryCredentials, len(channelIDs))
	for _, channelID := range channelIDs {
		raw, found := rawConfigs[channelID]
		if !found {
			return nil, fmt.Errorf("%w: active WhatsApp channel has no configuration",
				domain.ErrNotConfigured)
		}
		parsed, err := domain.ParseChannelConfig(domain.ChannelTypeWhatsApp, raw)
		if err != nil {
			return nil, fmt.Errorf("%w: stored WhatsApp channel configuration is invalid",
				domain.ErrNotConfigured)
		}
		if parsed == nil {
			return nil, fmt.Errorf("%w: active WhatsApp channel has no configuration",
				domain.ErrNotConfigured)
		}
		if err := parsed.MapSecrets(s.channelOpener.Open); err != nil {
			return nil, fmt.Errorf("%w: stored WhatsApp channel credentials cannot be opened",
				domain.ErrNotConfigured)
		}
		config, ok := parsed.(*domain.WhatsAppChannelConfig)
		if !ok {
			return nil, fmt.Errorf("%w: stored channel is not WhatsApp", domain.ErrNotConfigured)
		}
		credentials[channelID] = domain.WhatsAppDeliveryCredentials{
			PhoneNumberID: config.PhoneNumberID,
			AccessToken:   config.AccessToken,
		}
	}
	return credentials, nil
}

// ResolvePublic returns expiry alone for expired tokens and the immutable bundle for active ones.
func (s *QuoteDeliveryService) ResolvePublic(ctx context.Context,
	token string) (*domain.PublicQuoteRepresentation, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrNotFound
	}
	accountID, err := s.sends.GetAccountIDByPublicToken(ctx, s.db.CrossAccount(), token)
	if err != nil {
		return nil, err
	}
	var send *domain.QuoteSend
	var quote *domain.Quote
	var cycleErr error
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID},
		func(q repository.Querier) error {
			var getErr error
			if send, getErr = s.sends.GetPublicByToken(ctx, q, accountID, token); getErr != nil {
				return getErr
			}
			if send.ExpiresAt == nil || !s.now().Before(*send.ExpiresAt) {
				return nil
			}
			quote, getErr = s.quotes.GetByVersionID(ctx, q, accountID, send.VersionID)
			if getErr != nil {
				return getErr
			}
			if quote.CurrentVersionID != nil && *quote.CurrentVersionID == send.VersionID {
				cycleErr = s.ensureCurrentSendCycle(ctx, q, accountID, *quote, *send)
			}
			if errors.Is(cycleErr, domain.ErrConflict) {
				return nil
			}
			return cycleErr
		})
	if err != nil {
		return nil, err
	}
	if send.ExpiresAt == nil {
		return nil, domain.ErrNotFound
	}
	if !s.now().Before(*send.ExpiresAt) {
		return &domain.PublicQuoteRepresentation{Status: "EXPIRED", ExpiresAt: *send.ExpiresAt}, nil
	}
	customerStatus := s.customerStatus(ctx, accountID, send.ID)
	// Answered deliveries retain their history even when the quote or send cycle is closed.
	if (quote.ArchivedAt != nil || cycleErr != nil) && customerStatus == nil {
		return &domain.PublicQuoteRepresentation{Status: domain.PublicQuoteStatusUnavailable,
			ExpiresAt: *send.ExpiresAt}, nil
	}
	if s.representations == nil {
		return nil, domain.ErrNotConfigured
	}
	base, err := s.customerQuoteBase(ctx, accountID)
	if err != nil {
		return nil, err
	}
	versionID, accepted := s.acceptedVersion(ctx, accountID, *quote, *send, customerStatus)
	result, err := s.representations.ResolvePublic(ctx, accountID, versionID, *send.ExpiresAt,
		customerQuoteURL(base, send.PublicToken))
	if err != nil {
		return nil, err
	}
	result.CustomerStatus = customerStatus
	result.AcceptedVersion = accepted
	result.InReview = quote.AcceptanceUnconfirmedAt != nil
	return result, nil
}

// acceptedRepresentationEnsurer renders a version the customer's own choices froze.
type acceptedRepresentationEnsurer interface {
	EnsureAccepted(ctx context.Context, tenant domain.Tenant, quoteID,
		versionID uuid.UUID) (*domain.QuoteRepresentationResult, error)
}

// acceptedVersion picks what the customer sees once they accepted through this send: the version
// their alternatives produced, when one did. Anything that cannot render it keeps the sent version.
func (s *QuoteDeliveryService) acceptedVersion(ctx context.Context, accountID uuid.UUID,
	quote domain.Quote, send domain.QuoteSend, customerStatus *domain.ClientActionType,
) (uuid.UUID, bool) {
	if customerStatus == nil || *customerStatus != domain.ClientActionAccept ||
		quote.CurrentStatus != domain.QuoteStatusAccepted || quote.CurrentVersionID == nil ||
		*quote.CurrentVersionID == send.VersionID {
		return send.VersionID, false
	}
	ensurer, ok := s.representations.(acceptedRepresentationEnsurer)
	if !ok {
		return send.VersionID, false
	}
	// The answer and the version its alternatives froze are written in one transaction, so they
	// share now(): a version made any other way (a reactivation, a later send) never matches.
	var producedByThisAnswer bool
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q repository.Querier) error {
		action, err := s.actions.GetBySend(ctx, q, accountID, send.ID)
		if err != nil {
			return err
		}
		current, err := s.quotes.GetCurrentVersion(ctx, q, accountID, quote.BranchID, quote.ID)
		if err != nil {
			return err
		}
		producedByThisAnswer = current.ID == *quote.CurrentVersionID && current.CreatedAt.Equal(action.CreatedAt)
		return nil
	})
	if err != nil || !producedByThisAnswer {
		return send.VersionID, false
	}
	tenant := domain.Tenant{AccountID: accountID, BranchID: quote.BranchID}
	if _, err := ensurer.EnsureAccepted(ctx, tenant, quote.ID, *quote.CurrentVersionID); err != nil {
		s.log.WarnContext(ctx, "accepted version unavailable; showing the sent one",
			slog.String("quote_id", quote.ID.String()), slog.Any("error", err))
		return send.VersionID, false
	}
	return *quote.CurrentVersionID, true
}

// PreviewPublic calculates the frozen commercial outcome of customer alternative choices.
func (s *QuoteDeliveryService) PreviewPublic(ctx context.Context, token string,
	selections []domain.ClientAlternativeSelection,
) (*domain.PublicQuoteSelectionPreview, error) {
	if s.selectionSnapshots == nil || s.draftCarry == nil || s.draftCarry.sweep == nil {
		return nil, domain.ErrNotConfigured
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrNotFound
	}
	accountID, err := s.sends.GetAccountIDByPublicToken(ctx, s.db.CrossAccount(), token)
	if err != nil {
		return nil, err
	}
	var preview *domain.PublicQuoteSelectionPreview
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID},
		func(q repository.Querier) error {
			send, getErr := s.sends.GetPublicByToken(ctx, q, accountID, token)
			if getErr != nil {
				return getErr
			}
			if send.ExpiresAt == nil {
				return domain.ErrNotFound
			}
			if !s.now().Before(*send.ExpiresAt) {
				return domain.WithCode(domain.CodeQuoteSendExpired, domain.ErrConflict)
			}
			quote, getErr := s.quotes.GetByVersionID(ctx, q, accountID, send.VersionID)
			if getErr != nil {
				return getErr
			}
			if quote.ArchivedAt != nil {
				return domain.WithCode(domain.CodeQuoteArchived, domain.ErrConflict)
			}
			if err := s.ensureCurrentSendCycle(ctx, q, accountID, *quote, *send); err != nil {
				return err
			}
			if quote.CurrentVersionID == nil || *quote.CurrentVersionID != send.VersionID ||
				quote.CurrentStatus != domain.QuoteStatusSent {
				return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
			}
			representation, getErr := s.selectionSnapshots.GetByVersionID(ctx, q, accountID,
				send.VersionID)
			if getErr != nil {
				return getErr
			}
			if representation.SelectionPricing == nil {
				return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
			}
			items, getErr := s.quotes.ListItems(ctx, q, accountID, send.VersionID)
			if getErr != nil {
				return getErr
			}
			alternatives, getErr := s.quotes.ListAlternativesByItemIDs(ctx, q, accountID,
				quoteItemIDs(items))
			if getErr != nil {
				return getErr
			}
			selected, selectionErr := selectedAlternatives(items, alternatives,
				representation.Payload, selections)
			if selectionErr != nil {
				return selectionErr
			}
			discounts, getErr := s.draftCarry.sweep.discounts.ListByVersionID(ctx, q, accountID,
				send.VersionID)
			if getErr != nil {
				return getErr
			}
			links, getErr := s.draftCarry.sweep.discounts.ListItemIDsByDiscountIDs(ctx, q, accountID,
				send.VersionID, discountIDs(discounts))
			if getErr != nil {
				return getErr
			}
			selectedItems := withSelectedAlternatives(items, selected)
			result, evaluationErr := previewSelectionPricing(quote.BranchID,
				selectedItems, discounts, links,
				*representation.SelectionPricing)
			if evaluationErr != nil {
				return evaluationErr
			}
			payload := previewSelectionPayload(representation.Payload,
				selectedItems, selected, discounts,
				*representation.SelectionPricing, result)
			preview = &domain.PublicQuoteSelectionPreview{Payload: payload,
				Subtotal: sumItemSubtotals(selectedItems).StringFixed(domain.MoneyScale)}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return preview, nil
}

// RespondPublic records the customer's deliberate answer to a published quote and closes the loop
// when that answer addresses the version the quote still shows: ACCEPT moves SENT to ACCEPTED,
// REJECT to REJECTED, and REQUEST_CHANGE to CHANGE_REQUESTED. A token that already answered
// replays its first answer instead of recording another one, so a flaky retry never doubles the
// response.
func (s *QuoteDeliveryService) RespondPublic(ctx context.Context, token string,
	in domain.ClientActionInput) (*domain.PublicQuoteActionResult, error) {
	if s.actions == nil {
		return nil, domain.ErrNotConfigured
	}
	if err := normalizeClientAction(&in); err != nil {
		return nil, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrNotFound
	}
	accountID, err := s.sends.GetAccountIDByPublicToken(ctx, s.db.CrossAccount(), token)
	if err != nil {
		return nil, err
	}
	var result *domain.PublicQuoteActionResult
	err = s.db.WithAdvisoryLock(ctx, strings.Join([]string{"quote-action",
		accountID.String(), token}, ":"), func() error {
		var respondErr error
		result, respondErr = s.respondLocked(ctx, accountID, token, in)
		return respondErr
	})
	return result, err
}

func previewSelectionPayload(payload domain.QuoteRepresentationPayload,
	items []domain.QuoteItem, selected map[uuid.UUID]domain.QuoteItemAlternative,
	discounts []domain.QuoteDiscount, snapshot domain.QuoteSelectionPricingSnapshot,
	result sweepResult,
) domain.QuoteRepresentationPayload {
	byID := make(map[uuid.UUID]domain.QuoteItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	payload.Items = append([]domain.QuoteRepresentationItem(nil), payload.Items...)
	for index := range payload.Items {
		item, found := byID[payload.Items[index].ID]
		if !found {
			continue
		}
		payload.Items[index].UnitPrice = item.UnitPriceSnapshot.Decimal.StringFixed(domain.MoneyScale)
		payload.Items[index].Subtotal = item.Subtotal.Decimal.StringFixed(domain.MoneyScale)
		if alternative, chosen := selected[item.ID]; chosen {
			payload.Items[index].ProductCode = alternative.Code
			if alternative.CanonicalName != nil {
				payload.Items[index].ProductName = *alternative.CanonicalName
			}
			payload.Items[index].Unit = alternative.Unit
		}
	}
	payload.Discounts = previewSelectionDiscounts(discounts, snapshot, result)
	payload.Total = result.total.StringFixed(domain.MoneyScale)
	return payload
}

func previewSelectionPricing(branchID uuid.UUID, items []domain.QuoteItem,
	discounts []domain.QuoteDiscount, links map[uuid.UUID][]uuid.UUID,
	snapshot domain.QuoteSelectionPricingSnapshot,
) (sweepResult, error) {
	carried := make([]domain.QuoteDiscount, 0, len(discounts))
	for _, discount := range discounts {
		if discount.Origin != domain.DiscountOriginManualSeller && !discount.SuppressedBySeller {
			continue
		}
		if discount.Origin == domain.DiscountOriginAIAdaptation {
			discount.Origin = domain.DiscountOriginAutomatic
		}
		carried = append(carried, discount)
	}
	groups := make(map[uuid.UUID]domain.ProductGroup, len(snapshot.ProductGroups))
	for _, product := range snapshot.ProductGroups {
		groups[product.ProductID] = product.Group
	}
	return (sweepState{discounts: carried, links: links, promotions: snapshot.Promotions,
		groups: groups, at: snapshot.CapturedAt}).evaluate(branchID, items)
}

func previewSelectionDiscounts(discounts []domain.QuoteDiscount,
	snapshot domain.QuoteSelectionPricingSnapshot, result sweepResult,
) []domain.QuoteRepresentationDiscount {
	followed := make(map[uuid.UUID]string, len(result.followed))
	for _, amount := range result.followed {
		followed[amount.discountID] = amount.amount.StringFixed(domain.MoneyScale)
	}
	promotions := make(map[uuid.UUID]string, len(snapshot.Promotions))
	for _, promotion := range snapshot.Promotions {
		promotions[promotion.ID] = promotion.Name
	}
	preview := make([]domain.QuoteRepresentationDiscount, 0, len(discounts)+len(result.earned))
	for _, discount := range discounts {
		if discount.Origin != domain.DiscountOriginManualSeller || discount.SuppressedBySeller {
			continue
		}
		description := ""
		if discount.Description != nil {
			description = *discount.Description
		} else if discount.PromotionName != nil {
			description = *discount.PromotionName
		}
		amount := discount.Amount.StringFixed(domain.MoneyScale)
		if updated, moved := followed[discount.ID]; moved {
			amount = updated
		}
		preview = append(preview, domain.QuoteRepresentationDiscount{
			Description: description, Amount: amount,
		})
	}
	for _, application := range result.earned {
		preview = append(preview, domain.QuoteRepresentationDiscount{
			Description: promotions[application.PromotionID],
			Amount:      application.Amount.StringFixed(domain.MoneyScale),
		})
	}
	return preview
}

func (s *QuoteDeliveryService) respondLocked(ctx context.Context, accountID uuid.UUID, token string,
	in domain.ClientActionInput,
) (*domain.PublicQuoteActionResult, error) {
	var result *domain.PublicQuoteActionResult
	var moved *domain.ClientQuoteOutcome
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID},
		func(q repository.Querier) error {
			send, err := s.sends.GetPublicByToken(ctx, q, accountID, token)
			if err != nil {
				return err
			}
			if send.ExpiresAt == nil {
				return domain.ErrNotFound
			}
			if !s.now().Before(*send.ExpiresAt) {
				return domain.ErrConflict
			}
			quote, err := s.quotes.GetByVersionID(ctx, q, accountID, send.VersionID)
			if err != nil {
				return err
			}
			quote, err = s.quotes.GetByIDForUpdate(ctx, q, accountID, quote.BranchID, quote.ID)
			if err != nil {
				return err
			}
			existing, err := s.actions.GetBySend(ctx, q, accountID, send.ID)
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				return err
			}
			if existing != nil {
				result = &domain.PublicQuoteActionResult{CustomerStatus: existing.Type,
					CreatedAt: existing.CreatedAt, QuoteStatus: quote.CurrentStatus}
				return nil
			}
			if quote.ArchivedAt != nil {
				return domain.WithCode(domain.CodeQuoteArchived, domain.ErrConflict)
			}
			if err := s.ensureCurrentSendCycle(ctx, q, accountID, *quote, *send); err != nil {
				return err
			}
			if quote.CurrentVersionID == nil || *quote.CurrentVersionID != send.VersionID ||
				quote.CurrentStatus != domain.QuoteStatusSent {
				return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
			}
			target, ok := statusForClientAction(in.Type)
			if !ok {
				return domain.ErrInvalidInput
			}
			var acceptedVersion *domain.QuoteVersion
			if in.Type == domain.ClientActionAccept {
				current, currentErr := s.quotes.GetCurrentVersion(ctx, q, accountID, quote.BranchID, quote.ID)
				if currentErr != nil {
					return currentErr
				}
				acceptedVersion = current
				if len(in.Selections) > 0 {
					if s.selectionSnapshots == nil || s.draftCarry == nil {
						return domain.ErrNotConfigured
					}
					representation, representationErr := s.selectionSnapshots.GetByVersionID(ctx, q,
						accountID, send.VersionID)
					if representationErr != nil {
						return representationErr
					}
					if representation.SelectionPricing == nil {
						return domain.WithCode(domain.CodeQuoteNotSent, domain.ErrConflict)
					}
					acceptedVersion, currentErr = cloneQuoteVersionForClientSelection(ctx, q, s.quotes, s.quotes,
						accountID, quote.BranchID, quote.ID, *current, in.Selections, s.draftCarry,
						representation.Payload, *representation.SelectionPricing)
					if currentErr != nil {
						return currentErr
					}
					if currentErr = s.quotes.MarkAlternativesChosenByClient(ctx, q, accountID,
						send.VersionID, in.Selections); currentErr != nil {
						return currentErr
					}
				}
			}
			created, err := s.actions.Create(ctx, q, accountID, domain.NewClientAction{
				ID: uuid.New(), QuoteSendID: &send.ID, VersionID: send.VersionID, Type: in.Type,
				Comment: in.Message})
			if err != nil {
				return err
			}
			if in.Type == domain.ClientActionRequestChange {
				if err := s.createChangeRequest(ctx, q, accountID, *quote, *send, *created,
					*in.Message); err != nil {
					return err
				}
			}
			updated, updateErr := s.quotes.UpdateStatus(ctx, q, accountID, quote.BranchID,
				quote.ID, quote.CurrentStatus, target)
			if updateErr != nil {
				return updateErr
			}
			previous := quote.CurrentStatus
			if _, appendErr := s.quotes.AppendStatusChange(ctx, q, accountID, quote.ID,
				&previous, target, nil); appendErr != nil {
				return appendErr
			}
			result = &domain.PublicQuoteActionResult{CustomerStatus: created.Type,
				CreatedAt: created.CreatedAt, QuoteStatus: target}
			if acceptedVersion != nil {
				total := acceptedVersion.Total.StringFixed(domain.MoneyScale)
				result.Total = &total
			}
			if in.Type == domain.ClientActionAccept || in.Type == domain.ClientActionReject {
				moved = &domain.ClientQuoteOutcome{QuoteID: updated.ID,
					Reference: string(domain.QuoteReference(updated.Number)), Status: target,
					Action: in.Type, SellerID: updated.SellerID}
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	if moved != nil {
		s.notifySellerOfOutcome(ctx, accountID, *moved)
	}
	return result, nil
}

// customerStatus reads the answer this exact send already received, if any; a read failure is
// logged and left unanswered rather than failing the whole public page.
func (s *QuoteDeliveryService) customerStatus(ctx context.Context, accountID, sendID uuid.UUID,
) *domain.ClientActionType {
	if s.actions == nil {
		return nil
	}
	var action *domain.ClientAction
	err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID},
		func(q repository.Querier) error {
			var getErr error
			action, getErr = s.actions.GetBySend(ctx, q, accountID, sendID)
			return getErr
		})
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		s.log.WarnContext(ctx, "customer response read failed",
			slog.String("send_id", sendID.String()), slog.Any("error", err))
		return nil
	}
	return &action.Type
}

func (s *QuoteDeliveryService) evaluateAfterCommit(ctx context.Context, tenant domain.Tenant,
	quoteID, versionID uuid.UUID) {
	if s.evaluator == nil {
		return
	}
	if _, err := s.evaluator.EvaluateFinalQuote(context.WithoutCancel(ctx), tenant, quoteID,
		versionID); err != nil {
		s.log.ErrorContext(ctx, "post-send quote evaluation failed",
			slog.String("quote_id", quoteID.String()), slog.String("version_id", versionID.String()),
			slog.Any("error", err))
	}
}

func normalizeQuoteDeliveryInput(in domain.QuoteDeliveryInput) (domain.QuoteDeliveryInput, error) {
	in.Phone = strings.TrimSpace(in.Phone)
	if len(in.Phones) > 20 || len(in.Emails) > 20 {
		return in, domain.ErrInvalidInput
	}
	in.Phones = uniqueDestinations(in.Phones, false)
	in.Emails = uniqueDestinations(in.Emails, true)
	for _, phone := range in.Phones {
		if !e164PhonePattern.MatchString(phone) {
			return in, domain.ErrInvalidInput
		}
	}
	for _, email := range in.Emails {
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email {
			return in, domain.ErrInvalidInput
		}
	}
	if in.Mode == "" {
		switch {
		case in.FollowupMessage != nil:
			in.Mode = domain.QuoteDeliveryModeFollowup
		case in.OtherChannelsOnly:
			in.Mode = domain.QuoteDeliveryModeOtherMedium
		}
	}
	if in.Mode != "" && !in.Mode.Valid() {
		return in, fmt.Errorf("%w: unknown delivery mode", domain.ErrInvalidInput)
	}
	if (in.Mode == domain.QuoteDeliveryModeFollowup) != (in.FollowupMessage != nil) ||
		in.OtherChannelsOnly && in.Mode != domain.QuoteDeliveryModeOtherMedium {
		return in, fmt.Errorf("%w: the delivery mode does not match its fields", domain.ErrInvalidInput)
	}
	in.OtherChannelsOnly = in.Mode == domain.QuoteDeliveryModeOtherMedium
	if in.FollowupMessage != nil {
		message := strings.TrimSpace(*in.FollowupMessage)
		if message == "" || utf8.RuneCountInString(message) > 2000 {
			return in, domain.ErrInvalidInput
		}
		in.FollowupMessage = &message
	}
	if in.IdempotencyKey == uuid.Nil || (len(deliveryPhones(in)) == 0 && len(deliveryEmails(in)) == 0) {
		return in, fmt.Errorf("%w: idempotency key and a delivery destination are required", domain.ErrInvalidInput)
	}
	if in.Phone != "" && !e164PhonePattern.MatchString(in.Phone) {
		return in, fmt.Errorf("%w: recipient phone must be E.164", domain.ErrInvalidInput)
	}
	if in.Email != nil {
		trimmed := strings.TrimSpace(*in.Email)
		parsed, err := mail.ParseAddress(trimmed)
		if err != nil || parsed.Address != trimmed {
			return in, fmt.Errorf("%w: email delivery address is invalid", domain.ErrInvalidInput)
		}
		in.Email = &trimmed
	}
	if in.ExpiryDays != nil && (*in.ExpiryDays < minQuoteValidityDays ||
		*in.ExpiryDays > maxQuoteValidityDays) {
		return in, fmt.Errorf("%w: expiry_days must be between %d and %d",
			domain.ErrInvalidInput, minQuoteValidityDays, maxQuoteValidityDays)
	}
	if err := normalizeSendClient(in.ClientAssociation); err != nil {
		return in, err
	}
	return in, nil
}

func validateValidityDays(days int) error {
	if days < minQuoteValidityDays || days > maxQuoteValidityDays {
		return fmt.Errorf("%w: expiry days must be between %d and %d", domain.ErrInvalidInput,
			minQuoteValidityDays, maxQuoteValidityDays)
	}
	return nil
}

const maxClientActionComment = 512

func normalizeClientAction(in *domain.ClientActionInput) error {
	switch in.Type {
	case domain.ClientActionAccept, domain.ClientActionReject, domain.ClientActionRequestChange:
	default:
		return fmt.Errorf("%w: a customer response must be ACCEPT, REQUEST_CHANGE or REJECT",
			domain.ErrInvalidInput)
	}
	if in.Message != nil {
		trimmed := strings.TrimSpace(*in.Message)
		if trimmed == "" {
			in.Message = nil
		} else {
			in.Message = &trimmed
		}
	}
	if in.Type == domain.ClientActionRequestChange &&
		(in.Message == nil || utf8.RuneCountInString(*in.Message) > maxClientActionComment) {
		return fmt.Errorf("%w: requesting a change needs a message of 1 to %d characters",
			domain.ErrInvalidInput, maxClientActionComment)
	}
	if in.Type != domain.ClientActionRequestChange && in.Message != nil &&
		utf8.RuneCountInString(*in.Message) > maxClientActionComment {
		return fmt.Errorf("%w: comment must be at most %d characters",
			domain.ErrInvalidInput, maxClientActionComment)
	}
	if in.Type != domain.ClientActionAccept && len(in.Selections) > 0 {
		return fmt.Errorf("%w: alternatives can only be selected when accepting", domain.ErrInvalidInput)
	}
	return nil
}

func statusForClientAction(action domain.ClientActionType) (domain.QuoteStatus, bool) {
	switch action {
	case domain.ClientActionAccept:
		return domain.QuoteStatusAccepted, true
	case domain.ClientActionReject:
		return domain.QuoteStatusRejected, true
	case domain.ClientActionRequestChange:
		return domain.QuoteStatusChangeRequested, true
	default:
		return "", false
	}
}

func validateReplay(sends []domain.QuoteSend, versionID uuid.UUID,
	in domain.QuoteDeliveryInput, validityDays int) error {
	want := map[string]bool{}
	if in.FollowupMessage == nil {
		want[string(domain.ChannelTypeWebApp)+":customer-portal"] = true
	}
	for _, phone := range deliveryPhones(in) {
		want[string(domain.ChannelTypeWhatsApp)+":"+phone] = true
	}
	for _, email := range deliveryEmails(in) {
		want[string(domain.ChannelTypeEmail)+":"+email] = true
	}
	if len(sends) != len(want) {
		return domain.WithCode(domain.CodeIdempotencyMismatch, domain.ErrConflict)
	}
	for _, send := range sends {
		key := string(send.ChannelType) + ":" + send.Destination
		sameMessage := (send.Message == nil && in.FollowupMessage == nil) || (send.Message != nil && in.FollowupMessage != nil && *send.Message == *in.FollowupMessage)
		if !want[key] || !sameMessage || send.VersionID != versionID || send.ValidityDays != validityDays || send.ClientAssociationHash != sendClientFingerprint(in.ClientAssociation) {
			return domain.WithCode(domain.CodeIdempotencyMismatch, domain.ErrConflict)
		}
		delete(want, key)
	}
	return nil
}

func complete(sends []domain.QuoteSend) bool {
	for _, send := range sends {
		if send.TrackingStatus == domain.SendTrackingStatusPending {
			return false
		}
	}
	return true
}

func hasSuccessfulSend(sends []domain.QuoteSend) bool {
	for _, send := range sends {
		if send.TrackingStatus == domain.SendTrackingStatusSent ||
			send.TrackingStatus == domain.SendTrackingStatusDelivered ||
			send.TrackingStatus == domain.SendTrackingStatusViewed {
			return true
		}
	}
	return false
}

func replayResult(quote domain.Quote, version domain.QuoteVersion,
	sends []domain.QuoteSend) (*domain.QuoteDeliveryResult, error) {
	for _, send := range sends {
		if send.TrackingStatus == domain.SendTrackingStatusSent ||
			send.TrackingStatus == domain.SendTrackingStatusDelivered ||
			send.TrackingStatus == domain.SendTrackingStatusViewed {
			return &domain.QuoteDeliveryResult{QuoteID: quote.ID, VersionID: version.ID,
				CurrentStatus: quote.CurrentStatus, ExpiresAt: quote.ExpiresAt,
				Deliveries: sends, Replay: true}, nil
		}
	}
	return nil, domain.ErrDeliveryUnavailable
}

func applyOutcomes(sends []domain.QuoteSend, outcomes []domain.QuoteSendOutcome) {
	byID := make(map[uuid.UUID]domain.QuoteSendOutcome, len(outcomes))
	for _, outcome := range outcomes {
		byID[outcome.ID] = outcome
	}
	for i := range sends {
		outcome, ok := byID[sends[i].ID]
		if !ok {
			continue
		}
		sends[i].TrackingStatus = outcome.Status
		sends[i].ProviderReference = outcome.ProviderReference
		sends[i].SentAt = outcome.SentAt
		sends[i].ExpiresAt = outcome.ExpiresAt
	}
}

func newPublicToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate quote public token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func deliveryPhones(in domain.QuoteDeliveryInput) []string {
	values := append([]string{}, in.Phones...)
	if in.Phone != "" {
		values = append(values, in.Phone)
	}
	return uniqueDestinations(values, false)
}
func deliveryEmails(in domain.QuoteDeliveryInput) []string {
	values := append([]string{}, in.Emails...)
	if in.Email != nil {
		values = append(values, *in.Email)
	}
	return uniqueDestinations(values, true)
}
func uniqueDestinations(values []string, email bool) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if email {
			value = strings.ToLower(value)
		}
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
