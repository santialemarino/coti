package domain

import (
	"slices"
	"testing"
)

func TestQuoteReferences(t *testing.T) {
	t.Parallel()
	for text, want := range map[string][]int64{
		"Sobre la cotización COT-000027 que me enviaron, necesito un cambio": {27},
		"cambiá la #27 por favor":        {27},
		"en la 27 sumame cal":            {27},
		"la cotizacion nro 31 está bien": {31},
		"Hola, necesito cotizar un pedido nuevo: 5 bolsas de Cemento Avellaneda de 50 kg y 2 baldes de Hidrófugo de 20 kg. Gracias.": {},
		"quiero 5 bolsas de 50 kg":          {},
		"dale, entregalo en la 9 de julio":  {},
		"dale mandalo a la 25 de mayo 300":  {},
		"cotización 12 de marzo, la de cal": {},
	} {
		if got := QuoteReferences(text); !slices.Equal(got, want) {
			t.Errorf("QuoteReferences(%q) = %v, want %v", text, got, want)
		}
	}
}
