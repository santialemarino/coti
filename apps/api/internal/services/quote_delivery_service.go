package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

const (
	minQuoteValidityDays = 1
	maxQuoteValidityDays = 365
)

var e164PhonePattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

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

type quoteDeliveryQuoteRepository interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID,
		id uuid.UUID) (*domain.Quote, error)
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID, branchID,
		id uuid.UUID) (*domain.Quote, error)
	GetCurrentVersion(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) (*domain.QuoteVersion, error)
	FreezeVersion(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		versionID uuid.UUID) (*domain.QuoteVersion, error)
	SetClient(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		clientID uuid.UUID) error
	SetExpiry(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID, expiresAt time.Time) error
	UpdateStatus(ctx context.Context, q repository.Querier, accountID, branchID, quoteID uuid.UUID,
		from, to domain.QuoteStatus) (*domain.Quote, error)
	AppendStatusChange(ctx context.Context, q repository.Querier, accountID, quoteID uuid.UUID,
		previousStatus *domain.QuoteStatus, newStatus domain.QuoteStatus,
		userID *uuid.UUID) (*domain.QuoteStatusChange, error)
}

type quoteDeliveryRFQRepository interface {
	SetClient(ctx context.Context, q repository.Querier, accountID, branchID, rfqID,
		clientID uuid.UUID) error
}

type quoteDeliveryClientRepository interface {
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewClient) (*domain.Client, error)
	UpdateContact(ctx context.Context, q repository.Querier, accountID, clientID uuid.UUID,
		in domain.ClientContact) (*domain.Client, error)
}

type quoteDeliveryChannelRepository interface {
	ListActiveByBranch(ctx context.Context, q repository.Querier, accountID,
		branchID uuid.UUID) ([]domain.Channel, error)
}

type quoteDeliveryBranchRepository interface {
	GetByID(ctx context.Context, q repository.Querier, accountID,
		branchID uuid.UUID) (*domain.Branch, error)
}

type quoteEmailSender interface {
	Send(ctx context.Context, out OutboundMail) error
}

type quotePublicDB interface {
	tenantTxRunner
	CrossAccount() repository.Querier
	WithAdvisoryLock(ctx context.Context, key string, fn func() error) error
}

// QuoteDeliveryService freezes and delivers a seller-approved quote, then labels it post-commit.
type QuoteDeliveryService struct {
	db        quotePublicDB
	sends     quoteDeliveryRepository
	quotes    quoteDeliveryQuoteRepository
	rfqs      quoteDeliveryRFQRepository
	clients   quoteDeliveryClientRepository
	channels  quoteDeliveryChannelRepository
	branches  quoteDeliveryBranchRepository
	whatsapp  domain.QuoteWhatsAppSender
	email     quoteEmailSender
	evaluator QuoteQualityEvaluator
	webappURL string
	now       func() time.Time
	log       *slog.Logger
}

// NewQuoteDeliveryService builds the delivery orchestrator.
func NewQuoteDeliveryService(db quotePublicDB, sends quoteDeliveryRepository,
	quotes quoteDeliveryQuoteRepository, rfqs quoteDeliveryRFQRepository,
	clients quoteDeliveryClientRepository, channels quoteDeliveryChannelRepository,
	branches quoteDeliveryBranchRepository, whatsapp domain.QuoteWhatsAppSender,
	email quoteEmailSender, evaluator QuoteQualityEvaluator, webappURL string,
	now func() time.Time, log *slog.Logger) *QuoteDeliveryService {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &QuoteDeliveryService{db: db, sends: sends, quotes: quotes, rfqs: rfqs,
		clients: clients, channels: channels, branches: branches, whatsapp: whatsapp,
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
	normalized, err := normalizeQuoteDeliveryInput(in)
	if err != nil {
		return nil, err
	}
	var result *domain.QuoteDeliveryResult
	err = s.db.WithAdvisoryLock(ctx, strings.Join([]string{"quote-delivery",
		tenant.AccountID.String(), quoteID.String(), normalized.IdempotencyKey.String()}, ":"),
		func() error {
			var sendErr error
			result, sendErr = s.sendLocked(ctx, tenant, quoteID, normalized)
			return sendErr
		})
	return result, err
}

func (s *QuoteDeliveryService) sendLocked(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID, normalized domain.QuoteDeliveryInput) (*domain.QuoteDeliveryResult, error) {
	prepared, quote, version, err := s.prepare(ctx, tenant, quoteID, normalized)
	if errors.Is(err, domain.ErrConflict) && domain.CodeOf(err) == domain.CodeConflict {
		prepared, quote, version, err = s.loadConcurrentReplay(ctx, tenant, quoteID, normalized)
	}
	if err != nil {
		return nil, err
	}
	s.decorateURLs(prepared)
	if complete(prepared) {
		return replayResult(*quote, *version, prepared)
	}

	outcomes := s.dispatch(ctx, tenant, *quote, prepared)
	completedAt := s.now().UTC()
	expiresAt := completedAt.AddDate(0, 0, prepared[0].ValidityDays)
	for _, delivery := range prepared {
		if delivery.ExpiresAt != nil && hasSuccessfulSend([]domain.QuoteSend{delivery}) {
			expiresAt = *delivery.ExpiresAt
			break
		}
	}
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
		if err := s.quotes.SetExpiry(commitCtx, q, tenant.AccountID, tenant.BranchID,
			quote.ID, expiresAt); err != nil {
			return err
		}
		current, err := s.quotes.GetByID(commitCtx, q, tenant.AccountID, tenant.BranchID,
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
		return nil
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
	s.evaluateAfterCommit(ctx, tenant, quote.ID, version.ID)
	return result, nil
}

func (s *QuoteDeliveryService) prepare(ctx context.Context, tenant domain.Tenant,
	quoteID uuid.UUID, in domain.QuoteDeliveryInput) ([]domain.QuoteSend, *domain.Quote,
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
		if quote.ArchivedAt != nil || (quote.CurrentStatus != domain.QuoteStatusQuoted &&
			quote.CurrentStatus != domain.QuoteStatusSent) {
			return domain.WithCode(domain.CodeQuoteNotSendable, domain.ErrConflict)
		}
		version, err = s.quotes.GetCurrentVersion(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
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

		clientID, err := s.upsertClient(ctx, q, tenant, *quote, in)
		if err != nil {
			return err
		}
		if err := s.quotes.SetClient(ctx, q, tenant.AccountID, tenant.BranchID, quote.ID,
			clientID); err != nil {
			return err
		}
		if err := s.rfqs.SetClient(ctx, q, tenant.AccountID, tenant.BranchID, quote.RFQID,
			clientID); err != nil {
			return err
		}
		quote.ClientID = &clientID

		if !version.IsImmutable {
			version, err = s.quotes.FreezeVersion(ctx, q, tenant.AccountID, tenant.BranchID,
				quote.ID, version.ID)
			if err != nil {
				return err
			}
		}
		selected, err := s.selectedChannels(ctx, q, tenant, in)
		if err != nil {
			return err
		}
		news := make([]domain.NewQuoteSend, 0, len(selected))
		for _, selectedChannel := range selected {
			destination := in.Phone
			if selectedChannel.Type == domain.ChannelTypeEmail {
				destination = *in.Email
			}
			token, err := newPublicToken()
			if err != nil {
				return err
			}
			news = append(news, domain.NewQuoteSend{ID: uuid.New(), VersionID: version.ID,
				ChannelID: selectedChannel.ID, IdempotencyKey: in.IdempotencyKey,
				Destination: destination, PublicToken: token,
				Format: domain.SendFormatWebAppLink, ValidityDays: validityDays})
		}
		prepared, err = s.sends.CreateBatch(ctx, q, tenant.AccountID, news)
		return err
	})
	return prepared, quote, version, err
}

func (s *QuoteDeliveryService) upsertClient(ctx context.Context, q repository.Querier,
	tenant domain.Tenant, quote domain.Quote, in domain.QuoteDeliveryInput) (uuid.UUID, error) {
	if quote.ClientID == nil {
		created, err := s.clients.Create(ctx, q, tenant.AccountID,
			domain.NewClient{Phone: in.Phone, Email: in.Email})
		if err != nil {
			return uuid.Nil, err
		}
		return created.ID, nil
	}
	updated, err := s.clients.UpdateContact(ctx, q, tenant.AccountID, *quote.ClientID,
		domain.ClientContact{Phone: in.Phone, Email: in.Email})
	if err != nil {
		return uuid.Nil, err
	}
	return updated.ID, nil
}

func (s *QuoteDeliveryService) selectedChannels(ctx context.Context, q repository.Querier,
	tenant domain.Tenant, in domain.QuoteDeliveryInput) ([]domain.Channel, error) {
	active, err := s.channels.ListActiveByBranch(ctx, q, tenant.AccountID, tenant.BranchID)
	if err != nil {
		return nil, err
	}
	types := []domain.ChannelType{domain.ChannelTypeWhatsApp}
	if in.Email != nil {
		types = append(types, domain.ChannelTypeEmail)
	}
	selected := make([]domain.Channel, 0, len(types))
	for _, channelType := range types {
		var channels []domain.Channel
		for _, channel := range active {
			if channel.Type == channelType {
				channels = append(channels, channel)
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
	quote domain.Quote, sends []domain.QuoteSend) []domain.QuoteSendOutcome {
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
			publicURL := s.publicURL(send.PublicToken)
			outcome := domain.QuoteSendOutcome{ID: send.ID,
				Status: domain.SendTrackingStatusFailed}
			var err error
			switch send.ChannelType {
			case domain.ChannelTypeWhatsApp:
				var receipt *domain.DeliveryReceipt
				receipt, err = s.whatsapp.SendQuote(ctx, domain.QuoteWhatsAppMessage{
					DeliveryID: send.ID, To: send.Destination,
					Body:      "Tu cotización está lista. Podés verla en " + publicURL,
					PublicURL: publicURL})
				if err == nil && receipt != nil && receipt.ProviderReference != "" {
					outcome.ProviderReference = &receipt.ProviderReference
				}
			case domain.ChannelTypeEmail:
				clientID := quote.ClientID
				err = s.email.Send(ctx, OutboundMail{AccountID: tenant.AccountID,
					UserID: &tenant.UserID, ClientID: clientID, QuoteID: &quote.ID,
					Event: domain.NotificationEventQuoteSent, To: send.Destination,
					Subject: "Tu cotización está lista", Heading: "Tu cotización está lista",
					Paragraphs:  []string{"Revisá el detalle y la vigencia en la web."},
					ActionLabel: "Ver cotización", ActionURL: publicURL})
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

// ResolvePublic returns only token validity; quote contents belong to the future webapp ticket.
func (s *QuoteDeliveryService) ResolvePublic(ctx context.Context,
	token string) (*domain.PublicQuoteSend, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrNotFound
	}
	accountID, err := s.sends.GetAccountIDByPublicToken(ctx, s.db.CrossAccount(), token)
	if err != nil {
		return nil, err
	}
	var send *domain.QuoteSend
	err = s.db.InTenantTx(ctx, domain.Tenant{AccountID: accountID},
		func(q repository.Querier) error {
			var getErr error
			send, getErr = s.sends.GetPublicByToken(ctx, q, accountID, token)
			return getErr
		})
	if err != nil {
		return nil, err
	}
	if send.ExpiresAt == nil {
		return nil, domain.ErrNotFound
	}
	status := "ACTIVE"
	if !s.now().Before(*send.ExpiresAt) {
		status = "EXPIRED"
	}
	return &domain.PublicQuoteSend{Status: status, ExpiresAt: *send.ExpiresAt}, nil
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
	if in.IdempotencyKey == uuid.Nil || !e164PhonePattern.MatchString(in.Phone) {
		return in, fmt.Errorf("%w: idempotency key and E.164 recipient phone are required",
			domain.ErrInvalidInput)
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
	return in, nil
}

func validateValidityDays(days int) error {
	if days < minQuoteValidityDays || days > maxQuoteValidityDays {
		return fmt.Errorf("%w: expiry days must be between %d and %d", domain.ErrInvalidInput,
			minQuoteValidityDays, maxQuoteValidityDays)
	}
	return nil
}

func validateReplay(sends []domain.QuoteSend, versionID uuid.UUID,
	in domain.QuoteDeliveryInput, validityDays int) error {
	want := map[domain.ChannelType]string{domain.ChannelTypeWhatsApp: in.Phone}
	if in.Email != nil {
		want[domain.ChannelTypeEmail] = *in.Email
	}
	if len(sends) != len(want) {
		return domain.WithCode(domain.CodeIdempotencyMismatch, domain.ErrConflict)
	}
	for _, send := range sends {
		destination, ok := want[send.ChannelType]
		if !ok || send.VersionID != versionID || send.Destination != destination ||
			send.ValidityDays != validityDays {
			return domain.WithCode(domain.CodeIdempotencyMismatch, domain.ErrConflict)
		}
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

func (s *QuoteDeliveryService) publicURL(token string) string {
	return s.webappURL + "/quotes/" + url.PathEscape(token)
}

func (s *QuoteDeliveryService) decorateURLs(sends []domain.QuoteSend) {
	for i := range sends {
		sends[i].PublicURL = s.publicURL(sends[i].PublicToken)
	}
}

func newPublicToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate quote public token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
