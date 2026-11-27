package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeDeliveryStatusStore struct {
	calls   []uuid.UUID
	batches [][]domain.ProviderDeliveryStatus
	changed []domain.QuoteSend
}

func (f *fakeDeliveryStatusStore) ApplyProviderStatuses(_ context.Context, _ repository.Querier,
	_, channelID uuid.UUID, statuses []domain.ProviderDeliveryStatus,
) ([]domain.QuoteSend, error) {
	f.calls = append(f.calls, channelID)
	f.batches = append(f.batches, statuses)
	return f.changed, nil
}

func TestWhatsAppDeliveryStatusService_RecordKeepsTheFurthestOutcomePerMessage(t *testing.T) {
	t.Parallel()
	route := domain.InboundChannelRoute{AccountID: uuid.New(), BranchID: uuid.New(),
		ChannelID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp}
	db := &fakeWhatsAppInboundDB{}
	store := &fakeDeliveryStatusStore{}
	service := NewWhatsAppDeliveryStatusService(db, &fakeWhatsAppInboundRouter{
		routes: map[string]domain.InboundChannelRoute{"phone-id": route}}, store, nil)

	err := service.Record(context.Background(), []domain.ProviderDeliveryStatus{
		{DestinationIdentifier: "phone-id", ProviderReference: "wamid.a", Status: domain.SendTrackingStatusViewed},
		{DestinationIdentifier: "phone-id", ProviderReference: "wamid.a", Status: domain.SendTrackingStatusDelivered},
		{DestinationIdentifier: "phone-id", ProviderReference: "wamid.b", Status: domain.SendTrackingStatusFailed,
			ErrorCode: "131047", OutsideWindow: true},
		{DestinationIdentifier: "other-phone", ProviderReference: "wamid.c", Status: domain.SendTrackingStatusDelivered},
	})
	if err != nil {
		t.Fatalf("Record() = %v, want no error", err)
	}
	if len(store.calls) != 1 || store.calls[0] != route.ChannelID || len(db.scopes) != 1 ||
		db.scopes[0].AccountID != route.AccountID || db.scopes[0].BranchID != route.BranchID {
		t.Fatalf("calls/scopes = %v/%v, want one update under the routed channel's tenant", store.calls, db.scopes)
	}
	got := map[string]domain.SendTrackingStatus{}
	for _, status := range store.batches[0] {
		got[status.ProviderReference] = status.Status
	}
	if len(got) != 2 || got["wamid.a"] != domain.SendTrackingStatusViewed ||
		got["wamid.b"] != domain.SendTrackingStatusFailed {
		t.Errorf("batch = %v, want read kept over a later delivered and the failure, nothing unrouted", got)
	}
}

func TestWhatsAppDeliveryStatusService_RecordWithNoStatusesTouchesNothing(t *testing.T) {
	t.Parallel()
	db := &fakeWhatsAppInboundDB{}
	store := &fakeDeliveryStatusStore{}
	service := NewWhatsAppDeliveryStatusService(db, &fakeWhatsAppInboundRouter{}, store, nil)
	if err := service.Record(context.Background(), nil); err != nil || len(store.calls) != 0 {
		t.Errorf("Record(nil) = %v with %d calls, want nothing", err, len(store.calls))
	}
}

func TestWhatsAppDeliveryStatusService_RecordPrefersADeliveryOverAFailure(t *testing.T) {
	t.Parallel()
	route := domain.InboundChannelRoute{AccountID: uuid.New(), BranchID: uuid.New(),
		ChannelID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp}
	store := &fakeDeliveryStatusStore{}
	service := NewWhatsAppDeliveryStatusService(&fakeWhatsAppInboundDB{}, &fakeWhatsAppInboundRouter{
		routes: map[string]domain.InboundChannelRoute{"phone-id": route}}, store, nil)

	if err := service.Record(context.Background(), []domain.ProviderDeliveryStatus{
		{DestinationIdentifier: "phone-id", ProviderReference: "wamid.a", Status: domain.SendTrackingStatusDelivered},
		{DestinationIdentifier: "phone-id", ProviderReference: "wamid.a", Status: domain.SendTrackingStatusFailed},
	}); err != nil {
		t.Fatalf("Record() = %v, want no error", err)
	}
	if len(store.batches) != 1 || len(store.batches[0]) != 1 ||
		store.batches[0][0].Status != domain.SendTrackingStatusDelivered {
		t.Errorf("batches = %v, want the delivery kept over a failure for the same message", store.batches)
	}
}

func TestWhatsAppDeliveryStatusService_RecordDropsAnInactiveOwnerQuietly(t *testing.T) {
	t.Parallel()
	active := domain.InboundChannelRoute{AccountID: uuid.New(), BranchID: uuid.New(),
		ChannelID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp}
	store := &fakeDeliveryStatusStore{}
	service := NewWhatsAppDeliveryStatusService(&fakeWhatsAppInboundDB{}, &fakeWhatsAppInboundRouter{
		routes: map[string]domain.InboundChannelRoute{"active-phone": active},
		err:    domain.ErrInboundDestinationInactive}, store, nil)

	err := service.Record(context.Background(), []domain.ProviderDeliveryStatus{
		{DestinationIdentifier: "active-phone", ProviderReference: "wamid.a", Status: domain.SendTrackingStatusViewed},
		{DestinationIdentifier: "inactive-phone", ProviderReference: "wamid.b", Status: domain.SendTrackingStatusViewed},
	})
	if err != nil {
		t.Fatalf("Record() = %v, want an inactive owner's statuses dropped without a retry", err)
	}
	if len(store.calls) != 1 || store.calls[0] != active.ChannelID {
		t.Errorf("calls = %v, want only the active channel updated", store.calls)
	}
}
