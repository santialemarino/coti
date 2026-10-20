package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type fakeContactStore struct {
	contactStore
	profiles     []domain.Client
	contacts     map[uuid.UUID][]domain.ClientContact
	observations map[uuid.UUID]domain.ContactObservation
	scope        uuid.UUID
	associated   bool
	writes       int
}

func (f *fakeContactStore) Lock(_ context.Context, _ repository.Querier, accountID uuid.UUID, _ string) error {
	f.scope = accountID
	return nil
}
func (f *fakeContactStore) HasObservation(_ context.Context, _ repository.Querier, _ uuid.UUID, id uuid.UUID) (bool, error) {
	_, found := f.observations[id]
	return found, nil
}
func (f *fakeContactStore) HasPendingForTaxID(context.Context, repository.Querier, uuid.UUID, string) (bool, error) {
	return false, nil
}
func (f *fakeContactStore) HasAssociations(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (bool, error) {
	return f.associated, nil
}
func (f *fakeContactStore) PriorSeparation(context.Context, repository.Querier, uuid.UUID, string, domain.ChannelType, string) (bool, error) {
	return false, nil
}
func (f *fakeContactStore) FindProfiles(context.Context, repository.Querier, uuid.UUID, string, domain.ChannelType, string) ([]domain.Client, error) {
	return f.profiles, nil
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

func TestClientContactService_RecordInbound_ClassifiesDeclarations(t *testing.T) {
	t.Parallel()
	for _, kind := range []domain.ChannelType{domain.ChannelTypeWhatsApp, domain.ChannelTypeEmail} {
		for _, scenario := range []string{"first without tax", "first with tax", "new contact matching tax", "known contact", "shared contact", "conflicting name", "enrichment exposes history"} {
			t.Run(string(kind)+"/"+scenario, func(t *testing.T) {
				service, profiles, contacts := newContactHarness()
				taxID := "20123456786"
				name := "Pepe"
				otherName := "Other"
				value := "one@example.test"
				if kind == domain.ChannelTypeWhatsApp {
					value = "+5491155550101"
				}
				client := domain.Client{ID: uuid.New(), AccountID: testAccountID, Name: &name, TaxID: &taxID}
				hints := domain.ClientContactHints{Name: &name, TaxID: &taxID}
				wantPending := false
				switch scenario {
				case "first without tax":
					hints.TaxID = nil
				case "first with tax":
				case "new contact matching tax":
					contacts.profiles = []domain.Client{client}
					wantPending = true
				case "known contact":
					contacts.profiles = []domain.Client{client}
					contacts.contacts[client.ID] = []domain.ClientContact{{Kind: kind, Value: value, Primary: true}}
				case "shared contact":
					other := domain.Client{ID: uuid.New(), AccountID: testAccountID}
					contacts.profiles = []domain.Client{client, other}
					contacts.contacts[client.ID] = []domain.ClientContact{{Kind: kind, Value: value, Primary: true}}
					contacts.contacts[other.ID] = []domain.ClientContact{{Kind: kind, Value: value, Primary: true}}
					wantPending = true
				case "conflicting name":
					contacts.profiles = []domain.Client{client}
					contacts.contacts[client.ID] = []domain.ClientContact{{Kind: kind, Value: value, Primary: true}}
					hints.Name = &otherName
					wantPending = true
				case "enrichment exposes history":
					client.TaxID = nil
					contacts.profiles = []domain.Client{client}
					contacts.contacts[client.ID] = []domain.ClientContact{{Kind: kind, Value: value, Primary: true}}
					contacts.associated = true
					wantPending = true
				}
				id := uuid.New()
				route := domain.InboundChannelRoute{AccountID: testAccountID, BranchID: testBranchID}
				err := service.RecordInbound(context.Background(), nil, route, id, kind, value, "Original message", hints)
				if err != nil {
					t.Fatal(err)
				}
				o := contacts.observations[id]
				if (o.Status == "PENDING") != wantPending {
					t.Fatalf("status=%s, want pending=%v", o.Status, wantPending)
				}
				if o.Original != "Original message" || contacts.scope != testAccountID {
					t.Fatal("lost original message or account scope")
				}
				if wantPending && (contacts.writes != 0 || profiles.created != nil) {
					t.Fatal("pending declaration wrote profile data")
				}
				if scenario == "first without tax" && (profiles.created == nil || profiles.created.TaxID != nil) {
					t.Fatal("intake invented or required a tax ID")
				}
				writes := contacts.writes
				if err := service.RecordInbound(context.Background(), nil, route, id, kind, value, "Original message", hints); err != nil {
					t.Fatal(err)
				}
				if contacts.writes != writes || len(contacts.observations) != 1 {
					t.Fatal("replay changed customer data")
				}
			})
		}
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
	if err := service.UpdateProfile(context.Background(), p, domain.ClientContactUpdate{}); err != nil {
		t.Fatal(err)
	}
	if contacts.writes != 2 || contacts.scope != p.AccountID {
		t.Fatal("authorized partial profile was not saved directly")
	}
}

func TestDeclaredClientData_RejectsAmbiguousAndInferredData(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		body      string
		name, tax bool
	}{
		{"Hola, soy Pepe\nCUIT: 20-12345678-6", true, true},
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
