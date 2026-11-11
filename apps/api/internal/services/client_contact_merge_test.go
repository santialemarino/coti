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
}

func (f *mergeRecorder) Merge(_ context.Context, _ repository.Querier, _ domain.Tenant, source, target uuid.UUID) error {
	f.merged = append(f.merged, [2]uuid.UUID{source, target})
	return nil
}

func TestClientContactService_MergeClients_RefusesWhatNoSellerMerges(t *testing.T) {
	t.Parallel()
	cuit := "20123456786"
	other := uuid.New()
	cases := []struct {
		name   string
		source domain.MergeProfile
		absent bool
		want   error
		code   domain.ErrorCode
	}{
		{name: "registered", source: domain.MergeProfile{TaxID: &cuit, Registered: true, InReach: true}, want: domain.ErrForbidden, code: domain.CodeClientRegistered},
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
			store.mergeProfiles = map[uuid.UUID]domain.MergeProfile{target: {ID: target, InReach: true}}
			if !tc.absent {
				tc.source.ID = source
				store.mergeProfiles[source] = tc.source
			}
			err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, source)
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
	if err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, source); err != nil {
		t.Fatal(err)
	}
	if len(recorder.merged) != 1 || recorder.merged[0] != [2]uuid.UUID{source, target} {
		t.Fatalf("merged %v, want source %s into target %s", recorder.merged, source, target)
	}
	if err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, target); !errors.Is(err, domain.ErrInvalidInput) {
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
			err := service.MergeClients(context.Background(), domain.Tenant{AccountID: uuid.New()}, target, source)
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
