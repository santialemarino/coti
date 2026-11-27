package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type fakeAcceptedRepresentations struct {
	fakeQuoteDeliverRepresentations
	ensured []uuid.UUID
	tenant  domain.Tenant
	err     error
}

func (f *fakeAcceptedRepresentations) EnsureAccepted(_ context.Context, tenant domain.Tenant, _ uuid.UUID,
	versionID uuid.UUID) (*domain.QuoteRepresentationResult, error) {
	f.ensured = append(f.ensured, versionID)
	f.tenant = tenant
	return nil, f.err
}

// acceptedHarness is a send answered with ACCEPT whose answer froze acceptedVersion in the same
// transaction (same created_at), now the quote's current version.
func acceptedHarness(t *testing.T, acceptedVersion uuid.UUID, ensureErr error) (*quoteActionHarness, *fakeAcceptedRepresentations) {
	t.Helper()
	h := newQuoteActionHarness()
	send := h.seedActiveSend(testVersionID)
	h.quotes.quote.CurrentVersionID = &acceptedVersion
	h.quotes.quote.CurrentStatus = domain.QuoteStatusAccepted
	h.quotes.version = &domain.QuoteVersion{ID: acceptedVersion, QuoteID: testQuoteID, VersionNumber: 2,
		Currency: "ARS", IsImmutable: true, CreatedAt: fixedNow}
	h.clientActions.existing = &domain.ClientAction{ID: uuid.New(), VersionID: testVersionID,
		Type: domain.ClientActionAccept, CreatedAt: fixedNow}
	representations := &fakeAcceptedRepresentations{err: ensureErr}
	representations.resolve = &domain.PublicQuoteRepresentation{Status: "ACTIVE", ExpiresAt: *send.ExpiresAt}
	h.service.WithRepresentationService(representations)
	return h, representations
}

func resolveVersion(t *testing.T, h *quoteActionHarness, representations *fakeAcceptedRepresentations) (uuid.UUID, bool) {
	t.Helper()
	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	return representations.versionID, result.AcceptedVersion
}

// TC-12: accepting with an alternative froze a new version; the portal shows that one, not the
// sent version with the replaced line.
func TestQuoteDeliveryService_ResolvePublic_ShowsTheVersionTheCustomerAccepted(t *testing.T) {
	t.Parallel()
	accepted := uuid.New()
	h, representations := acceptedHarness(t, accepted, nil)

	if version, isAccepted := resolveVersion(t, h, representations); version != accepted || !isAccepted {
		t.Errorf("resolved %v (accepted=%v), want the accepted version %v", version, isAccepted, accepted)
	}
	if len(representations.ensured) != 1 || representations.tenant.BranchID != testBranchID {
		t.Errorf("ensured %v for %+v, want the accepted version in the quote's branch", representations.ensured,
			representations.tenant)
	}
}

func TestQuoteDeliveryService_ResolvePublic_KeepsTheSentVersionWhenTheAcceptedOneCannotRender(t *testing.T) {
	t.Parallel()
	h, representations := acceptedHarness(t, uuid.New(), errors.New("renderer down"))

	if version, isAccepted := resolveVersion(t, h, representations); version != testVersionID || isAccepted {
		t.Errorf("resolved %v (accepted=%v), want the sent version", version, isAccepted)
	}
}

// An acceptance without alternatives leaves the sent version current: nothing changes for it.
func TestQuoteDeliveryService_ResolvePublic_PlainAcceptanceShowsTheSentVersion(t *testing.T) {
	t.Parallel()
	h, representations := acceptedHarness(t, testVersionID, nil)

	if version, isAccepted := resolveVersion(t, h, representations); version != testVersionID || isAccepted ||
		len(representations.ensured) != 0 {
		t.Errorf("resolved %v (accepted=%v, ensured %v), want the sent version untouched", version, isAccepted,
			representations.ensured)
	}
}

// Only the version this send's own answer froze is shown: an old link accepted plainly, then a
// reactivation, a new version and an acceptance elsewhere, keeps showing what this link sent.
func TestQuoteDeliveryService_ResolvePublic_AVersionMadeAfterwardsKeepsTheSentOne(t *testing.T) {
	t.Parallel()
	h, representations := acceptedHarness(t, uuid.New(), nil)
	h.quotes.version.CreatedAt = fixedNow.Add(2 * time.Hour)

	if version, isAccepted := resolveVersion(t, h, representations); version != testVersionID || isAccepted ||
		len(representations.ensured) != 0 {
		t.Errorf("resolved %v (accepted=%v, ensured %v), want the sent version", version, isAccepted,
			representations.ensured)
	}
}

func TestQuoteDeliveryService_ResolvePublic_OnlyAStandingAcceptanceShowsTheAcceptedVersion(t *testing.T) {
	t.Parallel()
	for label, mutate := range map[string]func(h *quoteActionHarness){
		"no answer through this send": func(h *quoteActionHarness) { h.clientActions.existing = nil },
		"a rejection through this send": func(h *quoteActionHarness) {
			h.clientActions.existing.Type = domain.ClientActionReject
		},
		"a quote no longer accepted": func(h *quoteActionHarness) {
			h.quotes.quote.CurrentStatus = domain.QuoteStatusSent
		},
	} {
		h, representations := acceptedHarness(t, uuid.New(), nil)
		mutate(h)
		if version, isAccepted := resolveVersion(t, h, representations); version != testVersionID || isAccepted {
			t.Errorf("%s: resolved %v (accepted=%v), want the sent version", label, version, isAccepted)
		}
	}
}
