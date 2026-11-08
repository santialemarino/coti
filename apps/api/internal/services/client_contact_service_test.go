package services

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeContactStore struct {
	contactStore
	profiles      []domain.Client
	contacts      map[uuid.UUID][]domain.ClientContact
	observations  map[uuid.UUID]domain.ContactObservation
	scope         uuid.UUID
	mergeProfiles map[uuid.UUID]domain.MergeProfile
	writes        int
	lockKeys      []string
}

func (f *fakeContactStore) GetFiscal(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (*domain.ClientFiscal, error) {
	return &domain.ClientFiscal{}, nil
}

func (f *fakeContactStore) SaveFiscal(context.Context, repository.Querier, uuid.UUID, uuid.UUID, domain.ClientContactUpdate) error {
	return nil
}

func (f *fakeContactStore) Lock(_ context.Context, _ repository.Querier, accountID uuid.UUID, keys ...string) error {
	f.scope = accountID
	f.lockKeys = append(f.lockKeys, keys...)
	return nil
}
func (f *fakeContactStore) HasObservation(_ context.Context, _ repository.Querier, _ uuid.UUID, id uuid.UUID) (bool, error) {
	_, found := f.observations[id]
	return found, nil
}
func (f *fakeContactStore) ListMergeCandidates(context.Context, repository.Querier, domain.Tenant, uuid.UUID) ([]domain.Client, error) {
	return nil, nil
}
func (f *fakeContactStore) LockMergeProfiles(_ context.Context, _ repository.Querier, _ domain.Tenant, ids []uuid.UUID) (map[uuid.UUID]domain.MergeProfile, error) {
	result := make(map[uuid.UUID]domain.MergeProfile)
	for _, id := range ids {
		if p, ok := f.mergeProfiles[id]; ok {
			result[id] = p
		}
	}
	return result, nil
}
func (f *fakeContactStore) MoveContacts(context.Context, repository.Querier, uuid.UUID, uuid.UUID, uuid.UUID) error {
	f.writes++
	return nil
}
func (f *fakeContactStore) CompleteNameRequests(context.Context, repository.Querier, uuid.UUID, uuid.UUID) error {
	f.writes++
	return nil
}
func (f *fakeContactStore) FindProfiles(context.Context, repository.Querier, uuid.UUID, string, domain.ChannelType, string) ([]domain.Client, error) {
	return f.profiles, nil
}
func (f *fakeContactStore) FindMergedRoute(context.Context, repository.Querier, uuid.UUID, domain.ChannelType, string) ([]domain.Client, error) {
	return nil, nil
}
func (f *fakeContactStore) List(_ context.Context, _ repository.Querier, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID][]domain.ClientContact, error) {
	result := make(map[uuid.UUID][]domain.ClientContact)
	for _, id := range ids {
		result[id] = f.contacts[id]
	}
	return result, nil
}
func (f *fakeContactStore) Save(_ context.Context, _ repository.Querier, _ uuid.UUID, id uuid.UUID, contacts []domain.ClientContact, _ string) error {
	f.contacts[id] = contacts
	f.writes++
	return nil
}
func (f *fakeContactStore) UpdateName(context.Context, repository.Querier, uuid.UUID, uuid.UUID, string) error {
	f.writes++
	return nil
}
func (f *fakeContactStore) CreateObservation(_ context.Context, _ repository.Querier, _ uuid.UUID, id uuid.UUID, o domain.ContactObservation) error {
	f.observations[id] = o
	return nil
}

func newContactHarness() (*ClientContactService, *fakeClientProfiles, *fakeContactStore) {
	profiles := &fakeClientProfiles{clients: map[uuid.UUID]domain.Client{}}
	contacts := &fakeContactStore{contacts: map[uuid.UUID][]domain.ClientContact{}, observations: map[uuid.UUID]domain.ContactObservation{}}
	return NewClientContactService(&fakeDB{}, profiles, contacts), profiles, contacts
}

func TestClientContactService_RecordInbound_UsesOnlyContactIdentity(t *testing.T) {
	t.Parallel()
	for _, kind := range []domain.ChannelType{domain.ChannelTypeWhatsApp, domain.ChannelTypeEmail} {
		for _, known := range []bool{false, true} {
			service, profiles, contacts := newContactHarness()
			name, taxID := "Juan Perez", "20123456786"
			value := "one@example.test"
			if kind == domain.ChannelTypeWhatsApp {
				value = "+5491155550101"
			}
			existing := domain.Client{ID: uuid.New(), AccountID: testAccountID, Name: &name, TaxID: &taxID}
			if known {
				contacts.profiles = []domain.Client{existing}
			}
			message := uuid.New()
			route := domain.InboundChannelRoute{AccountID: testAccountID, BranchID: testBranchID}
			err := service.RecordInbound(context.Background(), nil, route, message, kind, value, "Original message", domain.ClientContactHints{Name: &name, TaxID: &taxID})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(contacts.lockKeys, "contact:"+string(kind)+":"+value) || slices.Contains(contacts.lockKeys, "commercial-contacts") {
				t.Fatal("intake used a supplier-wide lock")
			}
			if !known && (profiles.created == nil || profiles.created.TaxID != nil) {
				t.Fatal("an inbound declaration assigned a CUIT")
			}
			if known && contacts.writes != 0 {
				t.Fatal("an inbound name replaced the registered profile")
			}
			before := contacts.writes
			if err := service.RecordInbound(context.Background(), nil, route, message, kind, value, "Original message", domain.ClientContactHints{Name: &name}); err != nil {
				t.Fatal(err)
			}
			if contacts.writes != before || len(contacts.observations) != 1 {
				t.Fatal("replay duplicated customer data")
			}
		}
	}
	service, profiles, contacts := newContactHarness()
	err := service.RecordInbound(context.Background(), nil, domain.InboundChannelRoute{AccountID: testAccountID, BranchID: testBranchID}, uuid.New(), domain.ChannelTypeEmail, "unknown@example.test", "An order without a name", domain.ClientContactHints{})
	if err != nil || profiles.created == nil || contacts.writes != 1 {
		t.Fatalf("unnamed sender did not receive a provisional profile: %v", err)
	}
}

func TestClientContactService_UpdateProfile_ValidatesBeforeWriting(t *testing.T) {
	t.Parallel()
	service, _, contacts := newContactHarness()
	p := domain.CustomerPrincipal{CustomerID: uuid.New(), AccountID: testAccountID, TaxID: "20123456786"}
	err := service.UpdateProfile(context.Background(), p, domain.ClientContactUpdate{Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: "one@example.test"}}})
	if !errors.Is(err, domain.ErrInvalidInput) || contacts.writes != 0 {
		t.Fatalf("invalid primary: %v", err)
	}
	err = service.UpdateProfile(context.Background(), domain.CustomerPrincipal{}, domain.ClientContactUpdate{})
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("anonymous update: %v", err)
	}
	if err := service.UpdateProfile(context.Background(), p, domain.ClientContactUpdate{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("empty required profile: %v", err)
	}
	if err := service.UpdateProfile(context.Background(), p, domain.ClientContactUpdate{Name: "Juan Perez"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("missing contact: %v", err)
	}
	if err := service.UpdateProfile(context.Background(), p, domain.ClientContactUpdate{Name: "Juan Perez", Contacts: []domain.ClientContact{{Kind: domain.ChannelTypeEmail, Value: "one@example.test", Primary: true}}}); err != nil {
		t.Fatal(err)
	}
	if contacts.writes != 2 || contacts.scope != p.AccountID {
		t.Fatal("authorized required profile was not saved directly")
	}
}

func TestDeclaredClientData_RejectsAmbiguousAndInferredData(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		body      string
		name, tax bool
	}{
		{"Nombre y apellido: Juan Perez\nCUIT: 20-12345678-6", true, true},
		{"Hola soy Pepe necesito cemento", false, false},
		{"Pepe 20123456786", false, false},
		{"CUIT 20123456786 CUIL 20123456786", false, false},
		{"CUIT 20123456780", false, false},
		{"{\"name\":\"Pepe\",\"tax_id\":\"20123456786\"}", false, false},
	} {
		h := declaredClientData(tc.body)
		if (h.Name != nil) != tc.name || (h.TaxID != nil) != tc.tax {
			t.Fatalf("%q: %#v", tc.body, h)
		}
	}
}

func (f *fakeContactStore) ListForCustomer(_ context.Context, _ repository.Querier, _ domain.CustomerPrincipal, id uuid.UUID, _ bool) ([]domain.ClientContact, error) {
	return f.contacts[id], nil
}

func (f *fakeContactStore) GetProfileAccess(context.Context, repository.Querier, domain.CustomerPrincipal, uuid.UUID) (*domain.CustomerProfileAccess, error) {
	return &domain.CustomerProfileAccess{Proved: true}, nil
}

func (f *fakeContactStore) Declare(context.Context, repository.Querier, domain.CustomerPrincipal, uuid.UUID, []domain.ClientContact) error {
	return nil
}

func (f *fakeContactStore) ProjectPrimaries(context.Context, repository.Querier, uuid.UUID, uuid.UUID) error {
	return nil
}
