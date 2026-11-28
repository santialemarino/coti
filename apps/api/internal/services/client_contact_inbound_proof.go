package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// inboundProofPattern is the message a customer sends to prove their number: «VERIFICAR 123456».
var inboundProofPattern = regexp.MustCompile(`(?i)^\s*` + domain.InboundVerificationPrefix + `\s*:?\s*(\d{6})\s*\.?\s*$`)

// inboundCodeAttempts bounds the retries for a code no other live proof in the account uses.
const inboundCodeAttempts = 5

// errProofTaken rolls back a proof whose number another account already verified.
var errProofTaken = errors.New("number verified on another account")

// errProofRefused rolls back a proof the customer's profile can no longer take.
var errProofRefused = errors.New("proof refused by the profile")

// ErrProofReplyUndelivered is a proof that committed but whose fixed answer did not go out.
var ErrProofReplyUndelivered = errors.New("WhatsApp proof answer undelivered")

type inboundProofStore interface {
	GetPendingInboundChallenge(context.Context, repository.Querier, domain.CustomerPrincipal) (*domain.ContactChallenge, error)
	HasPendingInboundCode(context.Context, repository.Querier, uuid.UUID, string) (bool, error)
	LockInboundChallenge(context.Context, repository.Querier, uuid.UUID, string) (*domain.ContactChallenge, error)
	ProveInboundChallenge(context.Context, repository.Querier, domain.CustomerPrincipal, uuid.UUID, string) error
	CreateSenderFailure(context.Context, repository.Querier, uuid.UUID, domain.ChannelType, string) error
	GetVerificationNumber(context.Context, repository.Querier, uuid.UUID, string) (string, error)
}

// InboundProofCode returns the code of a «VERIFICAR 123456» message, or false for any other text.
func InboundProofCode(body string) (string, bool) {
	match := inboundProofPattern.FindStringSubmatch(body)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// WithInboundProofs makes a WhatsApp verification a code the customer sends from their own number.
// secret derives each proof's code from its id, so its owner can read it again without it being stored.
func (s *ClientContactVerificationService) WithInboundProofs(secret []byte) *ClientContactVerificationService {
	s.inboundSecret = secret
	return s
}

func (s *ClientContactVerificationService) inboundProofs() (inboundProofStore, bool) {
	store, ok := s.store.(inboundProofStore)
	return store, ok && len(s.inboundSecret) > 0
}

func (s *ClientContactVerificationService) inboundCode(id uuid.UUID) string {
	mac := hmac.New(sha256.New, s.inboundSecret)
	mac.Write([]byte("whatsapp-verification:" + id.String()))
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(mac.Sum(nil)[:4])%1000000)
}

// inboundCodeHash is what a proof stores: a code only means something inside its supplier.
func inboundCodeHash(accountID uuid.UUID, code string) string {
	return hashToken("whatsapp-verification:" + accountID.String() + ":" + code)
}

// requestByMessage returns the customer's live WhatsApp proof, or starts one, with the code and the
// chat to send it to: the branch number the quote in next went out on, else the account's.
func (s *ClientContactVerificationService) requestByMessage(ctx context.Context, store inboundProofStore,
	p domain.CustomerPrincipal, next string,
) (*domain.ContactChallenge, error) {
	var challenge *domain.ContactChallenge
	err := s.contacts.db.InTenantTx(ctx, domain.Tenant{AccountID: p.AccountID}, func(q repository.Querier) error {
		existing, err := store.GetPendingInboundChallenge(ctx, q, p)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if existing != nil {
			challenge = existing
			challenge.InboundCode = s.inboundCode(existing.ID)
		} else {
			profiles, err := s.contacts.profiles.FindByTaxID(ctx, q, p.AccountID, p.TaxID)
			if err != nil {
				return err
			}
			if len(profiles) != 1 {
				return domain.ErrNotFound
			}
			c := &domain.ContactChallenge{AccountID: p.AccountID, CustomerID: p.CustomerID, ClientID: profiles[0].ID,
				Purpose: domain.ContactChallengeVerify, Kind: domain.ChannelTypeWhatsApp,
				ExpiresAt: s.now().Add(s.cfg.ContactVerificationTTL), NextPath: domain.CustomerNextPath(next)}
			for attempt := 0; c.TokenHash == ""; attempt++ {
				if attempt == inboundCodeAttempts {
					return fmt.Errorf("no free WhatsApp verification code: %w", domain.ErrLocked)
				}
				c.ID = uuid.New()
				c.InboundCode = s.inboundCode(c.ID)
				taken, err := store.HasPendingInboundCode(ctx, q, p.AccountID, inboundCodeHash(p.AccountID, c.InboundCode))
				if err != nil {
					return err
				}
				if !taken {
					c.TokenHash = inboundCodeHash(p.AccountID, c.InboundCode)
				}
			}
			if err := s.store.CreateChallenge(ctx, q, *c); err != nil {
				return err
			}
			challenge = c
		}
		number, err := store.GetVerificationNumber(ctx, q, p.AccountID, quoteToken(challenge.NextPath))
		if err != nil {
			return err
		}
		challenge.WhatsAppLink = domain.WhatsAppVerificationLink(number, challenge.InboundCode)
		if challenge.WhatsAppLink == "" {
			return fmt.Errorf("%w: no WhatsApp number to verify against", domain.ErrNotConfigured)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return challenge, nil
}

// ProveByMessage verifies the number a «VERIFICAR» message came from for the customer whose proof
// carries the code, and answers the sender with fixed text (a reply to the message they just sent).
// It never creates an order.
func (s *ClientContactVerificationService) ProveByMessage(ctx context.Context, route domain.InboundChannelRoute,
	sender, code string,
) error {
	store, ok := s.inboundProofs()
	if !ok {
		return domain.ErrNotConfigured
	}
	value, err := domain.NormalizeClientContact(domain.ChannelTypeWhatsApp, "+"+strings.TrimPrefix(sender, "+"))
	if err != nil {
		return err
	}
	reply := whatsAppProofInvalid
	err = s.contacts.db.InTenantTx(ctx, domain.Tenant{AccountID: route.AccountID}, func(q repository.Querier) error {
		contact := []domain.ClientContact{{Kind: domain.ChannelTypeWhatsApp, Value: value}}
		if err := s.contacts.lockContactValues(ctx, q, route.AccountID, contact); err != nil {
			return err
		}
		if err := s.store.LockDestination(ctx, q, domain.ChannelTypeWhatsApp, value); err != nil {
			return err
		}
		// A sender past the failure cap is answered with silence, so guessing costs the supplier nothing.
		if err := s.checkDestination(ctx, q, route.AccountID, domain.ChannelTypeWhatsApp, value, false); err != nil {
			if errors.Is(err, domain.ErrLocked) {
				reply = ""
				return nil
			}
			return err
		}
		failed := func() error {
			return store.CreateSenderFailure(ctx, q, route.AccountID, domain.ChannelTypeWhatsApp, value)
		}
		c, err := store.LockInboundChallenge(ctx, q, route.AccountID, inboundCodeHash(route.AccountID, code))
		if errors.Is(err, domain.ErrNotFound) {
			return failed()
		}
		if err != nil {
			return err
		}
		if c.ProvedAt != nil {
			// A redelivered message gets the same answer; another number with a spent code does not.
			if c.Value == value {
				reply = whatsAppProofVerified
				return nil
			}
			return failed()
		}
		if c.ConsumedAt != nil || !c.ExpiresAt.After(s.now()) {
			return failed()
		}
		profile, err := s.contacts.profiles.GetByID(ctx, q, route.AccountID, c.ClientID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if profile.TaxID == nil {
			return nil
		}
		p := domain.CustomerPrincipal{AccountID: route.AccountID, CustomerID: c.CustomerID, TaxID: *profile.TaxID}
		if err := s.addProvedNumber(ctx, q, p, c.ClientID, value); err != nil {
			if errors.Is(err, domain.ErrInvalidInput) {
				return errProofRefused
			}
			return err
		}
		if err := s.applyProof(ctx, q, p, c.ClientID, domain.ChannelTypeWhatsApp, value); err != nil {
			switch {
			case errors.Is(err, domain.ErrConflict):
				return errProofTaken
			case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrForbidden):
				return errProofRefused
			}
			return err
		}
		if err := store.ProveInboundChallenge(ctx, q, p, c.ID, value); err != nil {
			return err
		}
		reply = whatsAppProofVerified
		return nil
	})
	switch {
	case errors.Is(err, errProofTaken):
		reply, err = whatsAppProofTaken, nil
	case errors.Is(err, errProofRefused):
		reply, err = whatsAppProofInvalid, nil
	}
	if err != nil {
		return err
	}
	if reply == "" || s.messenger == nil {
		return nil
	}
	if _, err := s.messenger.SendContact(ctx, domain.ContactMessage{AccountID: route.AccountID,
		BranchID: route.BranchID, ChannelID: route.ChannelID, Purpose: domain.ContactPurposeVerification,
		Kind: domain.ChannelTypeWhatsApp, To: value, Body: reply}); err != nil {
		return fmt.Errorf("%w: %w", ErrProofReplyUndelivered, err)
	}
	return nil
}

// addProvedNumber puts the sender's number on the customer's profile when it is not there yet, by
// the rules a profile edit follows: the customer never types it.
func (s *ClientContactVerificationService) addProvedNumber(ctx context.Context, q repository.Querier,
	p domain.CustomerPrincipal, clientID uuid.UUID, value string,
) error {
	access, err := s.contacts.contacts.GetProfileAccess(ctx, q, p, clientID)
	if err != nil {
		return err
	}
	stored, err := s.contacts.contacts.ListForCustomer(ctx, q, p, clientID, access.Proved)
	if err != nil {
		return err
	}
	hasKind := false
	for _, contact := range stored {
		if contact.Kind == domain.ChannelTypeWhatsApp {
			if contact.Value == value {
				return nil
			}
			hasKind = true
		}
	}
	contacts := append(stored, domain.ClientContact{Kind: domain.ChannelTypeWhatsApp, Value: value, Primary: !hasKind})
	if err := s.contacts.checkContactLimit(contacts, len(stored)); err != nil {
		return err
	}
	if !access.Proved {
		return s.contacts.contacts.SaveDeclared(ctx, q, p, clientID, contacts)
	}
	return s.contacts.contacts.Save(ctx, q, p.AccountID, clientID, contacts, "CUSTOMER")
}

// quoteToken is the delivery token of a portal quote page, empty for any other page.
func quoteToken(next string) string {
	token, ok := strings.CutPrefix(next, "/quotes/")
	if !ok {
		return ""
	}
	return token
}
