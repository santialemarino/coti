package services

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

const (
	maxClientNameLength = 255
	maxClientTagLength  = 128
)

type clientProfileRepository interface {
	ListSummaries(ctx context.Context, q repository.Querier,
		tenant domain.Tenant) ([]domain.ClientSummary, error)
	GetByID(ctx context.Context, q repository.Querier, accountID,
		clientID uuid.UUID) (*domain.Client, error)
	FindByContacts(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		phone, email *string) ([]domain.Client, error)
	ListAcceptedSales(ctx context.Context, q repository.Querier, tenant domain.Tenant,
		clientID uuid.UUID) ([]domain.ClientSale, error)
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		in domain.NewClient) (*domain.Client, error)
}

type clientTagRepository interface {
	List(ctx context.Context, q repository.Querier,
		accountID uuid.UUID) ([]domain.ClientTag, error)
	ListByClientIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		clientIDs []uuid.UUID) (map[uuid.UUID][]domain.ClientTag, error)
	GetByIDs(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		tagIDs []uuid.UUID) ([]domain.ClientTag, error)
	Create(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		name string) (*domain.ClientTag, error)
	ReplaceClientTags(ctx context.Context, q repository.Querier, accountID,
		clientID uuid.UUID, tagIDs []uuid.UUID) error
}

type clientQuoteRepository interface {
	GetByID(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) (*domain.Quote, error)
	GetByIDForUpdate(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) (*domain.Quote, error)
	SetClient(ctx context.Context, q repository.Querier, accountID, branchID, quoteID,
		clientID uuid.UUID) error
}

type clientRFQRepository interface {
	SetClient(ctx context.Context, q repository.Querier, accountID, branchID, rfqID,
		clientID uuid.UUID) error
}

type clientSendRepository interface {
	ListByQuote(ctx context.Context, q repository.Querier, accountID, branchID,
		quoteID uuid.UUID) ([]domain.QuoteSend, error)
}

// ClientService manages confirmed client profiles, tags, and accepted-sale associations.
type ClientService struct {
	db      tenantTxRunner
	clients clientProfileRepository
	tags    clientTagRepository
	quotes  clientQuoteRepository
	rfqs    clientRFQRepository
	sends   clientSendRepository
}

// NewClientService builds the client profile use cases.
func NewClientService(
	db tenantTxRunner, clients clientProfileRepository, tags clientTagRepository,
	quotes clientQuoteRepository, rfqs clientRFQRepository, sends clientSendRepository,
) *ClientService {
	return &ClientService{db: db, clients: clients, tags: tags, quotes: quotes, rfqs: rfqs,
		sends: sends}
}

// ListClients returns account profiles with activity constrained to the caller's sales reach.
func (s *ClientService) ListClients(
	ctx context.Context, tenant domain.Tenant,
) ([]domain.ClientSummary, error) {
	var summaries []domain.ClientSummary
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		summaries, err = s.clients.ListSummaries(ctx, q, tenant)
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, len(summaries))
		for i := range summaries {
			ids[i] = summaries[i].ID
		}
		byClient, err := s.tags.ListByClientIDs(ctx, q, tenant.AccountID, ids)
		if err != nil {
			return err
		}
		for i := range summaries {
			summaries[i].Tags = nonNilTags(byClient[summaries[i].ID])
		}
		return nil
	})
	return summaries, err
}

// GetClient returns one profile and the accepted sales visible to the caller.
func (s *ClientService) GetClient(
	ctx context.Context, tenant domain.Tenant, clientID uuid.UUID,
) (*domain.ClientProfile, error) {
	var profile *domain.ClientProfile
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		client, err := s.clients.GetByID(ctx, q, tenant.AccountID, clientID)
		if err != nil {
			return err
		}
		byClient, err := s.tags.ListByClientIDs(ctx, q, tenant.AccountID, []uuid.UUID{clientID})
		if err != nil {
			return err
		}
		sales, err := s.clients.ListAcceptedSales(ctx, q, tenant, clientID)
		if err != nil {
			return err
		}
		profile = &domain.ClientProfile{Client: *client, Tags: nonNilTags(byClient[clientID]),
			Sales: sales}
		return nil
	})
	return profile, err
}

// ListTags returns the account's reusable client tags.
func (s *ClientService) ListTags(
	ctx context.Context, tenant domain.Tenant,
) ([]domain.ClientTag, error) {
	var tags []domain.ClientTag
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		tags, err = s.tags.List(ctx, q, tenant.AccountID)
		return err
	})
	return tags, err
}

// GetQuoteAssociation returns contact-based suggestions without changing the quote or any client.
func (s *ClientService) GetQuoteAssociation(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
) (*domain.QuoteClientAssociation, error) {
	if err := requireBranch(tenant, "a client association"); err != nil {
		return nil, err
	}
	var association *domain.QuoteClientAssociation
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByID(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusAccepted {
			return fmt.Errorf("%w: only an accepted quote can be associated", domain.ErrConflict)
		}
		sends, err := s.sends.ListByQuote(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		hints := contactHints(sends)
		matches, err := s.clients.FindByContacts(ctx, q, tenant.AccountID, hints.Phone, hints.Email)
		if err != nil {
			return err
		}

		ids := make([]uuid.UUID, 0, len(matches)+1)
		for i := range matches {
			ids = append(ids, matches[i].ID)
		}
		var current *domain.Client
		if quote.ClientID != nil {
			current, err = s.clients.GetByID(ctx, q, tenant.AccountID, *quote.ClientID)
			if err != nil {
				return err
			}
			ids = append(ids, current.ID)
		}
		byClient, err := s.tags.ListByClientIDs(ctx, q, tenant.AccountID, uniqueUUIDs(ids))
		if err != nil {
			return err
		}
		available, err := s.tags.List(ctx, q, tenant.AccountID)
		if err != nil {
			return err
		}

		association = &domain.QuoteClientAssociation{ContactHints: hints,
			AvailableTags: nonNilTags(available), Suggestions: make([]domain.ClientMatch, 0)}
		if current != nil {
			association.CurrentClient = &domain.ClientMatch{Client: *current,
				Tags: nonNilTags(byClient[current.ID])}
		}
		for i := range matches {
			if current != nil && matches[i].ID == current.ID {
				continue
			}
			association.Suggestions = append(association.Suggestions, domain.ClientMatch{
				Client: matches[i], Tags: nonNilTags(byClient[matches[i].ID])})
		}
		return nil
	})
	return association, err
}

// CreateTag normalizes and creates one reusable account tag, returning an existing match safely.
func (s *ClientService) CreateTag(
	ctx context.Context, tenant domain.Tenant, name string,
) (*domain.ClientTag, error) {
	name, err := normalizeTagName(name)
	if err != nil {
		return nil, err
	}
	var tag *domain.ClientTag
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var createErr error
		tag, createErr = s.tags.Create(ctx, q, tenant.AccountID, name)
		return createErr
	})
	return tag, err
}

// AssociateQuote confirms one accepted sale's client and replaces the selected profile tags.
func (s *ClientService) AssociateQuote(
	ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID,
	in domain.AssociateQuoteClientInput,
) (*domain.ClientMatch, error) {
	if err := requireBranch(tenant, "a client association"); err != nil {
		return nil, err
	}
	if (in.ClientID == nil) == (in.New == nil) {
		return nil, fmt.Errorf("%w: select one existing or one new client", domain.ErrInvalidInput)
	}
	if in.New != nil {
		if err := normalizeNewClient(in.New); err != nil {
			return nil, err
		}
	}
	in.TagIDs = uniqueUUIDs(in.TagIDs)

	var result *domain.ClientMatch
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		quote, err := s.quotes.GetByIDForUpdate(ctx, q, tenant.AccountID, tenant.BranchID, quoteID)
		if err != nil {
			return err
		}
		if quote.CurrentStatus != domain.QuoteStatusAccepted {
			return fmt.Errorf("%w: only an accepted quote can be associated", domain.ErrConflict)
		}

		var client *domain.Client
		if in.ClientID != nil {
			client, err = s.clients.GetByID(ctx, q, tenant.AccountID, *in.ClientID)
		} else {
			client, err = s.clients.Create(ctx, q, tenant.AccountID, *in.New)
		}
		if err != nil {
			return err
		}
		selected, err := s.tags.GetByIDs(ctx, q, tenant.AccountID, in.TagIDs)
		if err != nil {
			return err
		}
		if len(selected) != len(in.TagIDs) {
			return fmt.Errorf("%w: one or more tags do not belong to the account",
				domain.ErrInvalidInput)
		}
		if err := s.tags.ReplaceClientTags(ctx, q, tenant.AccountID, client.ID,
			in.TagIDs); err != nil {
			return err
		}
		if err := s.quotes.SetClient(ctx, q, tenant.AccountID, tenant.BranchID, quote.ID,
			client.ID); err != nil {
			return err
		}
		if err := s.rfqs.SetClient(ctx, q, tenant.AccountID, tenant.BranchID, quote.RFQID,
			client.ID); err != nil {
			return err
		}
		result = &domain.ClientMatch{Client: *client, Tags: nonNilTags(selected)}
		return nil
	})
	return result, err
}

// ReplaceClientTags validates account ownership and replaces one profile's tag set.
func (s *ClientService) ReplaceClientTags(
	ctx context.Context, tenant domain.Tenant, clientID uuid.UUID, tagIDs []uuid.UUID,
) ([]domain.ClientTag, error) {
	tagIDs = uniqueUUIDs(tagIDs)
	var selected []domain.ClientTag
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if _, err := s.clients.GetByID(ctx, q, tenant.AccountID, clientID); err != nil {
			return err
		}
		var err error
		selected, err = s.tags.GetByIDs(ctx, q, tenant.AccountID, tagIDs)
		if err != nil {
			return err
		}
		if len(selected) != len(tagIDs) {
			return fmt.Errorf("%w: one or more tags do not belong to the account",
				domain.ErrInvalidInput)
		}
		return s.tags.ReplaceClientTags(ctx, q, tenant.AccountID, clientID, tagIDs)
	})
	return nonNilTags(selected), err
}

func normalizeNewClient(client *domain.NewClient) error {
	client.Name = normalizedOptional(client.Name)
	client.Phone = normalizedOptional(client.Phone)
	client.Email = normalizedOptional(client.Email)
	if client.Name == nil && client.Phone == nil && client.Email == nil {
		return fmt.Errorf("%w: a client needs a name, phone or email", domain.ErrInvalidInput)
	}
	if client.Name != nil && utf8.RuneCountInString(*client.Name) > maxClientNameLength {
		return fmt.Errorf("%w: client name exceeds %d characters", domain.ErrInvalidInput,
			maxClientNameLength)
	}
	if client.Phone != nil {
		phone := normalizePhone(*client.Phone)
		if !e164PhonePattern.MatchString(phone) {
			return fmt.Errorf("%w: client phone must use E.164 format", domain.ErrInvalidInput)
		}
		client.Phone = &phone
	}
	if client.Email != nil {
		email := domain.NormalizeEmail(*client.Email)
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email {
			return fmt.Errorf("%w: client email is invalid", domain.ErrInvalidInput)
		}
		client.Email = &email
	}
	if client.OriginChannel == nil {
		switch {
		case client.Phone != nil:
			origin := domain.ClientOriginWhatsApp
			client.OriginChannel = &origin
		case client.Email != nil:
			origin := domain.ClientOriginEmail
			client.OriginChannel = &origin
		}
	}
	return nil
}

func normalizeTagName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || utf8.RuneCountInString(name) > maxClientTagLength {
		return "", fmt.Errorf("%w: tag name must have 1 to %d characters",
			domain.ErrInvalidInput, maxClientTagLength)
	}
	return name, nil
}

func contactHints(sends []domain.QuoteSend) domain.ClientContactHints {
	var hints domain.ClientContactHints
	for i := range sends {
		destination := strings.TrimSpace(sends[i].Destination)
		if destination == "" {
			continue
		}
		switch sends[i].ChannelType {
		case domain.ChannelTypeWhatsApp:
			if hints.Phone == nil {
				phone := normalizePhone(destination)
				hints.Phone = &phone
			}
		case domain.ChannelTypeEmail:
			if hints.Email == nil {
				email := domain.NormalizeEmail(destination)
				hints.Email = &email
			}
		}
	}
	return hints
}

func normalizePhone(phone string) string {
	replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "")
	return replacer.Replace(strings.TrimSpace(phone))
}

func normalizedOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func uniqueUUIDs(values []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(values))
	unique := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if value == uuid.Nil {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func nonNilTags(tags []domain.ClientTag) []domain.ClientTag {
	if tags == nil {
		return []domain.ClientTag{}
	}
	return tags
}
