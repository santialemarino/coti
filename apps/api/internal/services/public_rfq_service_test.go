package services

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type publicIntakeFake struct {
	destination  domain.PublicRFQDestination
	message      *domain.InboundChannelMessage
	source       *domain.RFQ
	fileStored   bool
	fileErr      error
	processErr   error
	processCalls int
	sourceCalls  int
	fileCalls    int
	tenant       domain.Tenant
}

func (f *publicIntakeFake) CrossAccount() repository.Querier { return nil }
func (f *publicIntakeFake) InTenantTx(_ context.Context, tenant domain.Tenant, fn func(repository.Querier) error) error {
	if tenant.AccountID != f.destination.Route.AccountID || tenant.BranchID != f.destination.Route.BranchID {
		return domain.ErrForbidden
	}
	f.tenant = tenant
	return fn(nil)
}
func (f *publicIntakeFake) WithAdvisoryLock(_ context.Context, _ string, fn func() error) error {
	return fn()
}
func (f *publicIntakeFake) GetDestinationCrossAccount(_ context.Context, _ repository.Querier, id uuid.UUID) (*domain.PublicRFQDestination, error) {
	if id != f.destination.Route.ChannelID {
		return nil, domain.ErrNotFound
	}
	return &f.destination, nil
}
func (f *publicIntakeFake) Reserve(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID, in domain.NewInboundChannelMessage) (*domain.InboundChannelMessage, error) {
	if f.message == nil {
		f.message = &domain.InboundChannelMessage{ID: in.ID, RFQID: in.RFQID, Payload: in.Payload}
	}
	return f.message, nil
}
func (f *publicIntakeFake) Create(_ context.Context, _ repository.Querier, accountID uuid.UUID, in domain.NewRFQ) (*domain.RFQ, error) {
	f.sourceCalls++
	f.source = &domain.RFQ{ID: *in.ID, AccountID: accountID, BranchID: in.BranchID, ChannelID: in.ChannelID,
		ClientID: in.ClientID, ClientLabel: in.ClientLabel, RawText: in.RawText, Status: domain.RFQStatusReceived}
	return f.source, nil
}
func (f *publicIntakeFake) GetByID(_ context.Context, _ repository.Querier, _, _, id uuid.UUID) (*domain.RFQ, error) {
	if f.source == nil || id != f.source.ID {
		return nil, domain.ErrNotFound
	}
	return f.source, nil
}
func (f *publicIntakeFake) StorePendingForRFQ(_ context.Context, _ domain.Tenant, _ uuid.UUID, _ domain.AttachmentUpload, _ []byte) (uuid.UUID, error) {
	f.fileCalls++
	if f.fileErr != nil {
		return uuid.Nil, f.fileErr
	}
	f.fileStored = true
	return uuid.New(), nil
}
func (f *publicIntakeFake) ListByRFQ(_ context.Context, _ repository.Querier, _, _, _ uuid.UUID) ([]domain.RFQAttachment, error) {
	if f.fileStored {
		return []domain.RFQAttachment{{ID: uuid.New()}}, nil
	}
	return nil, nil
}
func (f *publicIntakeFake) CreateInboundTextDraftForMessage(_ context.Context, _ domain.InboundChannelRoute, _ domain.InboundTextRFQDraftInput, _ uuid.UUID) (*domain.TextRFQDraft, error) {
	f.processCalls++
	if f.processErr != nil {
		f.source.Status = domain.RFQStatusFailed
		return nil, f.processErr
	}
	f.source.Status = domain.RFQStatusGenerated
	return &domain.TextRFQDraft{RFQ: *f.source}, nil
}

func publicIntakeFixture() (*PublicRFQService, *publicIntakeFake, domain.PublicRFQInput) {
	f := &publicIntakeFake{destination: domain.PublicRFQDestination{Route: domain.InboundChannelRoute{
		AccountID: uuid.New(), BranchID: uuid.New(), ChannelID: uuid.New(), ChannelType: domain.ChannelTypeWebApp}}}
	s := NewPublicRFQService(f, f, f, f, f, f, f, config.RFQConfig{MaxTextCharacters: 1000}, 1024)
	in := domain.PublicRFQInput{RequestID: uuid.New(), Contact: domain.PublicRFQContact{
		TaxID: "20123456786", Name: "Obra Norte", Email: "contacto@example.com"}, Text: "10 bolsas de cemento"}
	return s, f, in
}

func TestPublicRFQTextRetriesPreserveOneUnassignedRequest(t *testing.T) {
	s, f, in := publicIntakeFixture()
	for range 2 {
		if err := s.Create(context.Background(), f.destination.Route.ChannelID, in); err != nil {
			t.Fatal(err)
		}
	}
	if f.sourceCalls != 1 || f.processCalls != 1 || f.tenant.UserID != uuid.Nil || f.source.ClientID != nil {
		t.Fatalf("unexpected writes or client association: source=%d processing=%d tenant=%+v", f.sourceCalls, f.processCalls, f.tenant)
	}
	in.Contact.Name = "Different customer"
	if err := s.Create(context.Background(), f.destination.Route.ChannelID, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed retry got %v", err)
	}
}

func TestPublicRFQFilesAreDurablyQueuedAndRetryStorageFailures(t *testing.T) {
	s, f, in := publicIntakeFixture()
	in.Text = ""
	in.Filename = "pedido.txt"
	in.File = &domain.AttachmentUpload{ContentType: "text/plain", Size: 7, Content: bytes.NewBufferString("cemento")}
	f.fileErr = errors.New("storage unavailable")
	if err := s.Create(context.Background(), f.destination.Route.ChannelID, in); err == nil {
		t.Fatal("acknowledged a file that was not stored")
	}
	f.fileErr = nil
	for range 2 {
		in.File.Content = bytes.NewBufferString("cemento")
		if err := s.Create(context.Background(), f.destination.Route.ChannelID, in); err != nil {
			t.Fatal(err)
		}
	}
	if f.sourceCalls != 1 || f.processCalls != 0 || f.fileCalls != 2 || !f.fileStored {
		t.Fatalf("unexpected queued file writes: source=%d processing=%d file=%d", f.sourceCalls, f.processCalls, f.fileCalls)
	}
}

func TestPublicRFQProcessingFailureStillAcknowledgesRetainedSource(t *testing.T) {
	s, f, in := publicIntakeFixture()
	f.processErr = domain.ErrAIUnavailable
	if err := s.Create(context.Background(), f.destination.Route.ChannelID, in); err != nil {
		t.Fatal(err)
	}
	if f.source == nil || f.source.Status != domain.RFQStatusFailed {
		t.Fatal("lost source after provider failure")
	}
}

func TestPublicRFQRejectsUnknownDestinationAndAmbiguousModes(t *testing.T) {
	s, f, in := publicIntakeFixture()
	if err := s.Create(context.Background(), uuid.New(), in); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown destination: %v", err)
	}
	in.File = &domain.AttachmentUpload{}
	if err := s.Create(context.Background(), f.destination.Route.ChannelID, in); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("mixed modes: %v", err)
	}
	if f.sourceCalls != 0 {
		t.Fatal("refused input persisted a source")
	}
}
