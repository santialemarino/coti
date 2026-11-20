package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// storedArena is the product a matching row of the diff tests describes, as the database holds it.
func storedArena() domain.CatalogProductState {
	active := true
	return domain.CatalogProductState{
		Name: "Arena fina", Description: "", Unit: "m3", FamilyID: &testFamilyID,
		SubgroupID: &testSubgroupID, IsActive: true, BranchActive: &active,
		Price: ptr("1500.00"), MinPrice: ptr("1200.00"),
	}
}

func arenaRow() domain.CatalogImportRow {
	return domain.CatalogImportRow{
		Code: "ARE-1", Name: "Arena fina", Unit: "m3", FamilyID: testFamilyID,
		SubgroupID: &testSubgroupID, Price: "1500", MinPrice: ptr("1200"), IsActive: true,
		Action: domain.CatalogImportUpdate,
	}
}

func TestCatalogRowChanges(t *testing.T) {
	t.Parallel()
	inactive := false
	otherID := uuid.New()
	cases := []struct {
		name  string
		row   func(*domain.CatalogImportRow)
		state func(*domain.CatalogProductState)
		want  bool
	}{
		{name: "identical", want: false},
		{name: "price written with its decimals", row: func(r *domain.CatalogImportRow) { r.Price = "1500.00" }, want: false},
		{name: "price with one decimal", row: func(r *domain.CatalogImportRow) { r.Price, r.MinPrice = "1500.5", nil },
			state: func(s *domain.CatalogProductState) { s.Price, s.MinPrice = ptr("1500.50"), nil }, want: false},
		{name: "price one cent up", row: func(r *domain.CatalogImportRow) { r.Price = "1500.01" }, want: true},
		{name: "blank price keeps the one in force", row: func(r *domain.CatalogImportRow) { r.Price, r.MinPrice = "", nil }, want: false},
		{name: "first price at the branch", state: func(s *domain.CatalogProductState) { s.Price, s.MinPrice = nil, nil }, want: true},
		{name: "floor dropped", row: func(r *domain.CatalogImportRow) { r.MinPrice = nil }, want: true},
		{name: "floor added", state: func(s *domain.CatalogProductState) { s.MinPrice = nil }, want: true},
		{name: "floor changed", row: func(r *domain.CatalogImportRow) { r.MinPrice = ptr("1100") }, want: true},
		{name: "stored name padded", state: func(s *domain.CatalogProductState) { s.Name = " Arena fina  " }, want: false},
		{name: "accent added", row: func(r *domain.CatalogImportRow) { r.Name = "Arena finá" }, want: true},
		{name: "case changed", row: func(r *domain.CatalogImportRow) { r.Name = "arena fina" }, want: true},
		{name: "inner space added", row: func(r *domain.CatalogImportRow) { r.Name = "Arena  fina" }, want: true},
		{name: "description added", row: func(r *domain.CatalogImportRow) { r.Description = "Lavada" }, want: true},
		{name: "description cleared", state: func(s *domain.CatalogProductState) { s.Description = "Lavada" }, want: true},
		{name: "unit changed", row: func(r *domain.CatalogImportRow) { r.Unit = "M3" }, want: true},
		{name: "family changed", row: func(r *domain.CatalogImportRow) { r.FamilyID = otherID }, want: true},
		{name: "no stored family", state: func(s *domain.CatalogProductState) { s.FamilyID = nil }, want: true},
		{name: "subgroup cleared", row: func(r *domain.CatalogImportRow) { r.SubgroupID = nil }, want: true},
		{name: "subgroup changed", row: func(r *domain.CatalogImportRow) { r.SubgroupID = &otherID }, want: true},
		{name: "both without subgroup", row: func(r *domain.CatalogImportRow) { r.SubgroupID = nil },
			state: func(s *domain.CatalogProductState) { s.SubgroupID = nil }, want: false},
		{name: "deactivated", row: func(r *domain.CatalogImportRow) { r.IsActive = false }, want: true},
		{name: "not carried at the branch", state: func(s *domain.CatalogProductState) { s.BranchActive = nil }, want: true},
		{name: "off at the branch only", state: func(s *domain.CatalogProductState) { s.BranchActive = &inactive }, want: true},
		{name: "slot set", row: func(r *domain.CatalogImportRow) { r.EconomyAlternative = ptr("ARE-2") }, want: true},
		{name: "slot cleared", row: func(r *domain.CatalogImportRow) { r.PremiumAlternative = ptr("") }, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row, state := arenaRow(), storedArena()
			if tc.row != nil {
				tc.row(&row)
			}
			if tc.state != nil {
				tc.state(&state)
			}
			if got := catalogRowChanges(row, state); got != tc.want {
				t.Fatalf("catalogRowChanges() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeMoney_RefusesAnAmountGroupedInThousandsOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw, want string
		ambiguous bool
	}{
		{raw: "1.500", ambiguous: true},
		{raw: "1,500", ambiguous: true},
		{raw: "$ 12.500", ambiguous: true},
		{raw: "1.500.000", ambiguous: true},
		{raw: "1500", want: "1500"},
		{raw: "1.500,00", want: "1500.00"},
		{raw: "1,500.00", want: "1500.00"},
		{raw: "1.50", want: "1.50"},
		{raw: "1500,5", want: "1500.5"},
		{raw: " $ 12.500,50 ", want: "12500.50"},
	}
	for _, tc := range cases {
		got, _, err := normalizeMoney(tc.raw)
		if tc.ambiguous {
			if !errors.Is(err, errAmbiguousMoney) {
				t.Errorf("normalizeMoney(%q) = %q, %v; want errAmbiguousMoney", tc.raw, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("normalizeMoney(%q) = %q, %v; want %q", tc.raw, got, err, tc.want)
		}
	}
}

func TestCatalogImportService_Preview_CountsOnlyTheRowsThatChangeSomething(t *testing.T) {
	t.Parallel()
	same := storedArena()
	same.Name = "Arena"
	repo := &catalogImportTestRepository{states: map[string]domain.CatalogProductState{
		"SAME": same, "PRICE": same, "BLANK": same,
	}}
	header := "codigo;nombre;descripcion;unidad;familia;subgrupo;precio;precio_minimo;activo;alternativa_economica\n"
	// SAME as exported; PRICE edits one cell; BLANK keeps the price and clears an empty slot.
	rows := "SAME; Arena ;;m3;materiales de construccion;aridos;1500,00;1.200,00;SI;\n" +
		"PRICE;Arena;;m3;MATERIALES DE CONSTRUCCION;ARIDOS;1600;1200;SI;\n" +
		"BLANK;Arena;;m3;MATERIALES DE CONSTRUCCION;ARIDOS;;;si;NINGUNA\n" +
		"NEW;Arena gruesa;;m3;MATERIALES DE CONSTRUCCION;ARIDOS;1400;;SI;\n"
	service := NewCatalogImportService(catalogImportTestDB{}, repo, nil)
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}

	preview, err := service.Preview(context.Background(), tenant, strings.NewReader(header+rows))
	if err != nil {
		t.Fatalf("Preview() = %v", err)
	}
	wantActions := []string{domain.CatalogImportUnchanged, domain.CatalogImportUpdate,
		domain.CatalogImportUnchanged, domain.CatalogImportCreate}
	for i, row := range preview.Rows {
		if len(row.Errors) != 0 || row.Action != wantActions[i] {
			t.Errorf("row %s = %s %v, want %s", row.Code, row.Action, row.Errors, wantActions[i])
		}
	}
	if preview.Rows[2].EconomyAlternative != nil {
		t.Errorf("clear of an empty slot = %q, want it read as keep", *preview.Rows[2].EconomyAlternative)
	}
	if preview.ValidRows != 4 || preview.CreateRows != 1 || preview.UpdateRows != 1 ||
		preview.UnchangedRows != 2 || !preview.CanConfirm {
		t.Fatalf("preview counts = %+v", preview)
	}

	preview, err = service.Preview(context.Background(), tenant, strings.NewReader(header+
		"SAME;Arena;;m3;MATERIALES DE CONSTRUCCION;ARIDOS;1500;1200;SI;\n"))
	if err != nil {
		t.Fatalf("Preview() = %v", err)
	}
	if preview.ValidRows != 1 || preview.UnchangedRows != 1 || preview.CanConfirm {
		t.Fatalf("preview of an untouched export = %+v, want nothing to confirm", preview)
	}
}

func TestCatalogImportService_Preview_KeepsAClearOfASlotHoldingACodelessProduct(t *testing.T) {
	t.Parallel()
	state := storedArena()
	state.Name, state.MinPrice = "Arena", nil
	state.Slots = domain.CatalogSlots{Premium: ptr("")}
	repo := &catalogImportTestRepository{states: map[string]domain.CatalogProductState{"A": state}}
	preview := previewAlternatives(t, repo, "A;Arena;m3;MATERIALES DE CONSTRUCCION;ARIDOS;1500;SI;;-\n")
	row := preview.Rows[0]
	if len(row.Errors) != 0 || row.PremiumAlternative == nil || *row.PremiumAlternative != "" ||
		row.Action != domain.CatalogImportUpdate {
		t.Fatalf("row = %s %v %v; want the occupied slot cleared as an update", row.Action, row.Errors, row.PremiumAlternative)
	}
}

func TestCatalogImportService_Confirm_WritesOnlyTheRowsThatChange(t *testing.T) {
	t.Parallel()
	same := storedArena()
	same.Name = "Arena"
	repo := &catalogImportTestRepository{states: map[string]domain.CatalogProductState{"SAME": same, "PRICE": same}}
	service := NewCatalogImportService(catalogImportTestDB{}, repo, nil)
	tenant := domain.Tenant{AccountID: uuid.New(), UserID: uuid.New(), BranchID: uuid.New()}
	subgroup := "ARIDOS"
	input := func(code, price string) domain.CatalogImportInput {
		return domain.CatalogImportInput{Code: code, Name: "Arena", Unit: "m3",
			Family: "MATERIALES DE CONSTRUCCION", Subgroup: &subgroup, Price: price,
			MinPrice: ptr("1200"), IsActive: true}
	}

	result, err := service.Confirm(context.Background(), tenant,
		[]domain.CatalogImportInput{input("SAME", "1500"), input("PRICE", "1500")})
	if err != nil {
		t.Fatalf("Confirm() = %v, want an import with nothing to change accepted", err)
	}
	if result.UnchangedRows != 2 || result.CreatedRows+result.UpdatedRows != 0 || repo.applied != nil || repo.slots != nil {
		t.Fatalf("result = %+v, applied = %v; want nothing written", result, repo.applied)
	}

	result, err = service.Confirm(context.Background(), tenant,
		[]domain.CatalogImportInput{input("SAME", "1500"), input("PRICE", "1600")})
	if err != nil {
		t.Fatalf("Confirm() = %v", err)
	}
	if result.UpdatedRows != 1 || result.UnchangedRows != 1 || len(repo.applied) != 1 || repo.applied[0].Code != "PRICE" {
		t.Fatalf("result = %+v, applied = %v; want only PRICE written", result, repo.applied)
	}
}

func TestCatalogImportService_Preview_RefusesAThousandsGroupedPrice(t *testing.T) {
	t.Parallel()
	repo := &catalogImportTestRepository{}
	preview := previewAlternatives(t, repo, "A;Arena;m3;MATERIALES DE CONSTRUCCION;ARIDOS;1.500;SI;;\n")
	if !containsError(preview.Rows[0].Errors, "ambiguous_price") {
		t.Fatalf("errors = %v, want ambiguous_price", preview.Rows[0].Errors)
	}
}
