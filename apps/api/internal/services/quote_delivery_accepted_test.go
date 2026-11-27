package services

import (
	"context"
	"errors"
	"testing"

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

// acceptedHarness is a send answered with ACCEPT whose quote moved to acceptedVersion.
func acceptedHarness(t *testing.T, acceptedVersion uuid.UUID, ensureErr error) (*quoteActionHarness, *fakeAcceptedRepresentations) {
	t.Helper()
	h := newQuoteActionHarness()
	send := h.seedActiveSend(testVersionID)
	h.quotes.quote.CurrentVersionID = &acceptedVersion
	h.quotes.quote.CurrentStatus = domain.QuoteStatusAccepted
	h.clientActions.existing = &domain.ClientAction{ID: uuid.New(), VersionID: testVersionID,
		Type: domain.ClientActionAccept, CreatedAt: fixedNow}
	representations := &fakeAcceptedRepresentations{err: ensureErr}
	representations.resolve = &domain.PublicQuoteRepresentation{Status: "ACTIVE", ExpiresAt: *send.ExpiresAt}
	h.service.WithRepresentationService(representations)
	return h, representations
}

// TC-12: accepting with an alternative froze a new version; the portal shows that one, not the
// sent version with the replaced line.
func TestQuoteDeliveryService_ResolvePublic_ShowsTheVersionTheCustomerAccepted(t *testing.T) {
	t.Parallel()
	accepted := uuid.New()
	h, representations := acceptedHarness(t, accepted, nil)

	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	if representations.versionID != accepted || !result.AcceptedVersion {
		t.Errorf("resolved %v (accepted=%v), want the accepted version %v", representations.versionID,
			result.AcceptedVersion, accepted)
	}
	if len(representations.ensured) != 1 || representations.tenant.BranchID != testBranchID {
		t.Errorf("ensured %v for %+v, want the accepted version in the quote's branch", representations.ensured,
			representations.tenant)
	}
}

func TestQuoteDeliveryService_ResolvePublic_KeepsTheSentVersionWhenTheAcceptedOneCannotRender(t *testing.T) {
	t.Parallel()
	h, representations := acceptedHarness(t, uuid.New(), errors.New("renderer down"))

	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	if representations.versionID != testVersionID || result.AcceptedVersion {
		t.Errorf("resolved %v (accepted=%v), want the sent version", representations.versionID, result.AcceptedVersion)
	}
}

// An acceptance without alternatives leaves the sent version current: nothing changes for it.
func TestQuoteDeliveryService_ResolvePublic_PlainAcceptanceShowsTheSentVersion(t *testing.T) {
	t.Parallel()
	h, representations := acceptedHarness(t, testVersionID, nil)

	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	if representations.versionID != testVersionID || result.AcceptedVersion || len(representations.ensured) != 0 {
		t.Errorf("resolved %v (accepted=%v, ensured %v), want the sent version untouched",
			representations.versionID, result.AcceptedVersion, representations.ensured)
	}
}

// Only this send's own acceptance moves the view: a quote accepted elsewhere keeps showing what was sent.
func TestQuoteDeliveryService_ResolvePublic_AnotherAnswerKeepsTheSentVersion(t *testing.T) {
	t.Parallel()
	h, representations := acceptedHarness(t, uuid.New(), nil)
	h.clientActions.existing = nil

	result, err := h.service.ResolvePublic(context.Background(), h.token)
	if err != nil {
		t.Fatalf("ResolvePublic returned %v", err)
	}
	if representations.versionID != testVersionID || result.AcceptedVersion {
		t.Errorf("resolved %v (accepted=%v), want the sent version", representations.versionID, result.AcceptedVersion)
	}
}
