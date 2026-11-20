package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type mergeRecorder struct {
	*fakeContactStore
	merged [][2]uuid.UUID
	held   []bool
}

type survivingProfiles struct {
	*fakeClientProfiles
	survivors map[uuid.UUID]uuid.UUID
}

func (f *survivingProfiles) GetSurvivor(_ context.Context, _ repository.Querier, _ uuid.UUID, id uuid.UUID) (uuid.UUID, error) {
	return f.survivors[id], nil
}

type reviewRecorder struct {
	*mergeRecorder
	observation domain.ContactObservation
	decisions   int
}

func (f *reviewRecorder) GetObservation(context.Context, repository.Querier, domain.Tenant, uuid.UUID) (*domain.ContactObservation, error) {
	return &f.observation, nil
}

func (f *reviewRecorder) Decide(context.Context, repository.Querier, domain.Tenant, uuid.UUID, string) error {
	f.decisions++
	return nil
}

func TestClientContactService_ResolveSuggestion_UsesCurrentSurvivors(t *testing.T) {
	t.Parallel()
	for _, sameSurvivor := range []bool{false, true} {
		t.Run(map[bool]string{false: "selects the surviving source", true: "already merged"}[sameSurvivor], func(t *testing.T) {
			_, profiles, store := newContactHarness()
			oldSource, source, target := uuid.New(), uuid.New(), uuid.New()
			if sameSurvivor {
				target = source
			}
			recorder := &reviewRecorder{mergeRecorder: &mergeRecorder{fakeContactStore: store}, observation: domain.ContactObservation{SourceClientID: &oldSource, CandidateClientID: &target, Status: domain.ContactReviewPending}}
			store.mergeProfiles = map[uuid.UUID]domain.MergeProfile{source: {ID: source, InReach: true}, target: {ID: target, InReach: true}}
			survivors := &survivingProfiles{fakeClientProfiles: profiles, survivors: map[uuid.UUID]uuid.UUID{oldSource: source, target: target}}
			service := NewClientContactService(&fakeDB{}, survivors, recorder)
			if err := service.ResolveSuggestion(context.Background(), domain.Tenant{AccountID: testAccountID}, uuid.New(), domain.ContactReviewMerged, &source); err != nil {
				t.Fatal(err)
			}
			if recorder.decisions != 1 {
				t.Fatal("the observation was not settled")
			}
			if sameSurvivor && len(recorder.merged) != 0 {
				t.Fatal("already merged history moved twice")
			}
			if !sameSurvivor && (len(recorder.merged) != 1 || recorder.merged[0] != [2]uuid.UUID{target, source}) {
				t.Fatalf("wrong survivor: %v", recorder.merged)
			}
		})
	}
}

func (f *mergeRecorder) Merge(_ context.Context, _ repository.Querier, _ domain.Tenant, source, target uuid.UUID, hold bool) error {
	f.merged = append(f.merged, [2]uuid.UUID{source, target})
	f.held = append(f.held, hold)
	return nil
}

func TestClientContactService_MergeClients_IntoACustomerAccount(t *testing.T) {
	t.Parallel()
	cuit := "20123456786"
	for _, accountNamed := range []string{"target", "source"} {
		t.Run("the account is the "+accountNamed, func(t *testing.T) {
			t.Parallel()
			_, profiles, store := newContactHarness()
			recorder := &mergeRecorder{fakeContactStore: store}
			service := NewClientContactService(&fakeDB{}, profiles, recorder)
			provisional, account := uuid.New(), uuid.New()
			store.mergeProfiles = map[uuid.UUID]domain.MergeProfile{
				provisional: {ID: provisional, InReach: true},
				account:     {ID: account, TaxID: &cuit, Registered: true},
			}
			target, source := account, provisional
			if accountNamed == "source" {
				target, source = provisional, account
			}
			survivor, err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, source)
			if err != nil {
				t.Fatal(err)
			}
			if survivor != account || len(recorder.merged) != 1 || recorder.merged[0] != [2]uuid.UUID{provisional, account} {
				t.Fatalf("survivor %s, merged %v; want the provisional profile into the account", survivor, recorder.merged)
			}
			if !recorder.held[0] {
				t.Fatal("a seller merge into an account must hold the orders until the customer's proof")
			}
			if len(store.keptVerification) != 1 || store.keptVerification[0] {
				t.Fatalf("contacts copied into an account kept their verification: %v", store.keptVerification)
			}
		})
	}
}

func TestClientContactService_MergeClients_RetrySucceeds(t *testing.T) {
	t.Parallel()
	_, profiles, store := newContactHarness()
	recorder := &mergeRecorder{fakeContactStore: store}
	service := NewClientContactService(&fakeDB{}, profiles, recorder)
	source, target := uuid.New(), uuid.New()
	store.mergeProfiles = map[uuid.UUID]domain.MergeProfile{
		source: {ID: source, MergedInto: &target, InReach: true},
		target: {ID: target, InReach: true},
	}
	survivor, err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, source)
	if err != nil || survivor != target || len(recorder.merged) != 0 {
		t.Fatalf("retry = %s, %v, merges %v; want the survivor and nothing moved again", survivor, err, recorder.merged)
	}
}

func TestClientContactService_MergeClients_RefusesWhatNoSellerMerges(t *testing.T) {
	t.Parallel()
	cuit := "20123456786"
	other := uuid.New()
	cases := []struct {
		name             string
		source           domain.MergeProfile
		targetRegistered bool
		absent           bool
		want             error
		code             domain.ErrorCode
	}{
		{name: "two customer accounts", source: domain.MergeProfile{TaxID: &cuit, Registered: true, InReach: true}, targetRegistered: true, want: domain.ErrForbidden, code: domain.CodeClientRegistered},
		{name: "already merged", source: domain.MergeProfile{MergedInto: &other, InReach: true}, want: domain.ErrConflict, code: domain.CodeClientMerged},
		{name: "outside the branches", source: domain.MergeProfile{}, want: domain.ErrForbidden, code: domain.CodeClientOutOfReach},
		{name: "another account", absent: true, want: domain.ErrNotFound, code: domain.CodeNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, profiles, store := newContactHarness()
			recorder := &mergeRecorder{fakeContactStore: store}
			service := NewClientContactService(&fakeDB{}, profiles, recorder)
			source, target := uuid.New(), uuid.New()
			otherCUIT := "27111111110"
			into := domain.MergeProfile{ID: target, InReach: true}
			if tc.targetRegistered {
				into.TaxID, into.Registered = &otherCUIT, true
			}
			store.mergeProfiles = map[uuid.UUID]domain.MergeProfile{target: into}
			if !tc.absent {
				tc.source.ID = source
				store.mergeProfiles[source] = tc.source
			}
			_, err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, source)
			if !errors.Is(err, tc.want) || domain.CodeOf(err) != tc.code {
				t.Fatalf("MergeClients() = %v (%s), want %v (%s)", err, domain.CodeOf(err), tc.want, tc.code)
			}
			if len(recorder.merged) != 0 || store.writes != 0 {
				t.Fatalf("a refused merge wrote: merges %d, writes %d", len(recorder.merged), store.writes)
			}
		})
	}
}

func TestClientContactService_MergeClients_MergesSourceIntoPathProfile(t *testing.T) {
	t.Parallel()
	_, profiles, store := newContactHarness()
	recorder := &mergeRecorder{fakeContactStore: store}
	service := NewClientContactService(&fakeDB{}, profiles, recorder)
	source, target := uuid.New(), uuid.New()
	store.mergeProfiles = map[uuid.UUID]domain.MergeProfile{source: {ID: source, InReach: true}, target: {ID: target, InReach: true}}
	survivor, err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, source)
	if err != nil {
		t.Fatal(err)
	}
	if survivor != target || len(recorder.merged) != 1 || recorder.merged[0] != [2]uuid.UUID{source, target} || recorder.held[0] {
		t.Fatalf("survivor %s, merged %v (held %v), want source %s into target %s", survivor, recorder.merged, recorder.held, source, target)
	}
	if len(store.keptVerification) != 1 || !store.keptVerification[0] {
		t.Fatalf("two provisional profiles must keep their channel verification: %v", store.keptVerification)
	}
	if _, err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, target); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("merging a profile into itself = %v, want ErrInvalidInput", err)
	}
}

func TestClientContactService_MergeClients_KeepsTheCUITOnTheSurvivor(t *testing.T) {
	t.Parallel()
	cuit, otherCUIT := "20123456786", "27111111110"
	cases := []struct {
		name               string
		sourceTax, target  *string
		wantSwap, conflict bool
	}{
		{name: "CUIT only on the source", sourceTax: &cuit, wantSwap: true},
		{name: "CUIT only on the target", target: &cuit},
		{name: "two CUITs", sourceTax: &cuit, target: &otherCUIT, conflict: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, profiles, store := newContactHarness()
			recorder := &mergeRecorder{fakeContactStore: store}
			service := NewClientContactService(&fakeDB{}, profiles, recorder)
			source, target := uuid.New(), uuid.New()
			store.mergeProfiles = map[uuid.UUID]domain.MergeProfile{
				source: {ID: source, TaxID: tc.sourceTax, InReach: true},
				target: {ID: target, TaxID: tc.target, InReach: true},
			}
			_, err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, source)
			if tc.conflict {
				if !errors.Is(err, domain.ErrConflict) || domain.CodeOf(err) != domain.CodeClientTaxIDConflict || len(recorder.merged) != 0 {
					t.Fatalf("MergeClients() = %v (%s), merges %v; want CLIENT_TAX_ID_CONFLICT and none", err, domain.CodeOf(err), recorder.merged)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := [2]uuid.UUID{source, target}
			if tc.wantSwap {
				want = [2]uuid.UUID{target, source}
			}
			if len(recorder.merged) != 1 || recorder.merged[0] != want {
				t.Fatalf("merged %v, want %v so the CUIT profile survives", recorder.merged, want)
			}
		})
	}
}
