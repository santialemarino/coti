package domain

import "testing"

func TestUnitsConflict(t *testing.T) {
	unit := func(s string) *string { return &s }
	tests := []struct {
		name               string
		requested, product *string
		want               bool
	}{
		{"bag against cubic metre", unit("bolsa"), unit("m3"), true},
		{"bag against kilo", unit("bolsas"), unit("kg"), true},
		{"cubic metre against bag", unit("m³"), unit("Bolsa 25 kg"), true},
		{"tonne against cubic metre", unit("toneladas"), unit("m3"), true},
		{"two counts", unit("bolsa"), unit("unidad"), false},
		{"bare metre against cubic metre", unit("metros"), unit("m3"), false},
		{"bare metre against square metre", unit("mts"), unit("m2"), false},
		{"bare metre against a bar", unit("metros"), unit("barra"), true},
		{"same unit", unit("kg"), unit("kgs"), false},
		{"unknown requested unit", unit("ladrillos"), unit("m3"), false},
		{"no requested unit", nil, unit("m3"), false},
		{"no product unit", unit("bolsa"), nil, false},
		{"blank unit", unit("  "), unit("m3"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := UnitsConflict(tc.requested, tc.product); got != tc.want {
				t.Errorf("UnitsConflict() = %v, want %v", got, tc.want)
			}
		})
	}
}
