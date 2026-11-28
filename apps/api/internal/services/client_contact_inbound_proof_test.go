package services

import "testing"

func TestInboundProofCode(t *testing.T) {
	t.Parallel()
	for body, want := range map[string]string{
		"VERIFICAR 123456":         "123456",
		"  verificar 004321  ":     "004321",
		"Verificar: 123456.":       "123456",
		"VERIFICAR123456":          "123456",
		"VERIFICAR 12345":          "",
		"VERIFICAR 1234567":        "",
		"quiero VERIFICAR 123456":  "",
		"VERIFICAR 123456 gracias": "",
		"20 bolsas de cemento":     "",
	} {
		code, ok := InboundProofCode(body)
		if code != want || ok != (want != "") {
			t.Errorf("InboundProofCode(%q) = %q, %v; want %q", body, code, ok, want)
		}
	}
}
