package services

import (
	"math"
	"slices"
	"testing"
)

// The fixtures below are real catalog names and real client phrasings: the tokenizer exists for
// exactly the ways these two disagree.

// tokenTexts renders tokens as text, with a figure's unit after a slash, for readable asserts.
func tokenTexts(tokens []matchToken) []string {
	texts := make([]string, len(tokens))
	for i, token := range tokens {
		texts[i] = token.text
		if token.unit != "" {
			texts[i] += "/" + token.unit
		}
	}
	return texts
}

func TestTokenizeCatalogText_ReadsCatalogAndClientSpellingsAlike(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		// A dimension written with dashes or with x is the same three figures.
		{"LADRILLO HUECO 8-18-33 MARTIN (216)",
			[]string{"ladrill", "huec", "8", "18", "33", "martin", "216"}},
		{"ladrillos huecos 8x18x33", []string{"ladrill", "huec", "8", "18", "33"}},
		// A decimal comma is a decimal point, and the unit after a figure travels with it.
		{"placa durlock 12,5", []string{"plac", "durlock", "12.5"}},
		{"DURLOCK PLACA STAND 12.5 MM", []string{"durlock", "plac", "stand", "12.5/mm"}},
		{"HIERRO 8MM DE 12M", []string{"hierr", "8/mm", "12/m"}},
		// A whole number joined to a fraction is one figure, so 1/2 never meets 1-1/2.
		{"HIDRO 3 CAÑO(VERDE) 1-1/2 X 6MT", []string{"hidr", "3", "cano", "verde", "1+1/2", "6/m"}},
		{"HIDRO 3 BUJE REDUCCION 1 1/4 X 3/4",
			[]string{"hidr", "3", "buje", "reduccion", "1+1/4", "3/4"}},
		// Abbreviations with a slash are stop words; the hand of a door is read either way.
		{"MICROFIBRA P/ HORMIGON X 600 G", []string{"microfibr", "hormigon", "600/g"}},
		{"HERFASA PTA PLACA PINO M/ALUM 07 85 D",
			[]string{"herfas", "puert", "plac", "pino", "alum", "07", "85", "der"}},
		// Accents, the degree sign and superscripts decorate; inches are a unit.
		{"PVC CURVA 63 A 90°", []string{"pvc", "curv", "63", "90"}},
		// A superscript is the digit it stands for, and a metre squared is one unit either way.
		{"ARENA POR M²", []string{"aren", "m2"}},
		{"LOSETA 40X40 CEMENTO X M2", []string{"loset", "40", "40", "cement", "m2"}},
		{"MALLA DE FIBRA DE VIDRIO XM²", []string{"mall", "fibr", "vidri", "m2"}},
		// The ordinal sign decorates a figure the way the degree sign does.
		{"codo 90º", []string{"codo", "90"}},
		{"caño nº 10", []string{"cano", "10"}},
		// Argentine text groups thousands with points; a decimal point takes one or two digits.
		{"tanque 1.000 lts", []string{"tanque", "1000/l"}},
		{"vigueta 4.50", []string{"viguet", "4.50"}},
		// A lone letter is a shape; a letter before a slash abbreviates a word.
		{"PERFIL C GALVANIZADO 100", []string{"perfil", "c", "galvanizad", "100"}},
		{"PERFIL U NEGRO 160X60", []string{"perfil", "u", "negr", "160", "60"}},
		{"AISLANTE 50MM S/ ALUM", []string{"aislante", "50/mm", "alum"}},
		{`CLAVOS P/PARIS 2"`, []string{"clav", "pari", "2/in"}},
	} {
		t.Run(tc.text, func(t *testing.T) {
			if got := tokenTexts(tokenizeCatalogText(tc.text)); !slices.Equal(got, tc.want) {
				t.Errorf("tokens = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLineTokens_DropsTheCountOpeningTheLine(t *testing.T) {
	for _, tc := range []struct {
		line string
		want []string
	}{
		// The count goes; the packaging stays, weighed as the weak evidence it is.
		{"10 bolsas de cemento", []string{"bols", "cement"}},
		{"2 pallets de ladrillos", []string{"pallet", "ladrill"}},
		// A unit bound to the count goes with it.
		{"3 metros de arena", []string{"aren"}},
		{"8 hierros del 10", []string{"hierr", "10"}},
		// A spec after the material is the spec, and a leading fraction is a size, not a count.
		{"hierro del 8", []string{"hierr", "8"}},
		{"3/4 codo", []string{"3/4", "codo"}},
		// A size in a unit nobody counts in is a spec, wherever it stands.
		{"8mm hierro", []string{"8/mm", "hierr"}},
		// A line that is only a number keeps it: there is nothing else to match.
		{"110", []string{"110"}},
	} {
		t.Run(tc.line, func(t *testing.T) {
			if got := tokenTexts(lineTokens(tc.line)); !slices.Equal(got, tc.want) {
				t.Errorf("tokens = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCatalogCoverage_CreditsWhatTheCatalogTextAccountsFor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		line     string
		document string
		want     float64
	}{
		{"every word and figure", "vigueta 4.50", "VIGUETA PRET 4.50", 1},
		// The figure weighs 1.5 against the word's 1, so missing it costs most of the line.
		{"the wrong size", "vigueta 4.50", "VIGUETA PRET 4.40", 1 / 2.5},
		// 3 meets 3.00: figures compare by value.
		{"a figure written with decimals", "vigueta 3", "VIGUETA PRET 3.00", 1},
		{"a figure in another unit", "membrana 4mm", "PINTURA ASFALTICA SECADO RAPIDO X 4L", 0},
		{"a figure with no unit on one side", "hierro 8", "HIERRO 8MM DE 12M", 1},
		// Each catalog token answers one line token: two 18s need two 18s.
		{"a repeated figure", "ladrillo hueco 18x18x33", "LADRILLO HUECO 12-18-33 CORMELA",
			(1 + 1 + 1.5 + 1.5) / (1 + 1 + 1.5 + 1.5 + 1.5)},
		{"a plural and a gender", "ladrillos comunes", "LADRILLO COMUN", 1},
		{"pastina blanca", "pastina blanca", "PREMECOL PASTINA 1KG BLANCO", 1},
		{"a spelling typed by ear", "ladriyo comun", "LADRILLO COMUN", (phoneticCredit + 1) / 2},
		{"a silent h", "ierro 8", "HIERRO 8MM DE 12M", (phoneticCredit + 1.5) / 2.5},
		{"an abbreviation in the catalog", "viguetas pretensadas", "VIGUETA PRET 3.00",
			(1 + prefixCredit) / 2},
		{"an abbreviation from the client", "durlo 12.5", "DURLOCK PLACA STAND 12.5 MM",
			(prefixCredit + 1.5) / 2.5},
		// The catalog's own typo still meets the right spelling, one letter off.
		{"a typo in the catalog", "pegamento porcelanato", "PREMECOL PEGAMENTO PORCELLANATO 25KG",
			(1 + editDistanceCredit) / 2},
		// Three letters are too few to read as an abbreviation of a longer word.
		{"a short word that only starts another", "cal", "CERRO NEGRO CERAM CALACATA", 0},
		{"stop words alone", "de la para", "CEMENTO", 0},
		{"thousands against a plain figure", "tanque 1.000 lts", "TANQUE AGUA 1000 L", 1},
		{"thousands against one litre", "tanque 1.000 lts", "TANQUE AGUA 1 L", 1 / 2.5},
		{"a typed m2 against a superscript", "ceramica 45x45 m2", "CERAMICA 45X45 x m²", 1},
		{"an ordinal against a degree sign", "codo 90º", "CODO PVC 110 90°", 1},
		{"the other profile shape", "perfil u 160", "PERFIL C GALVANIZADO 160X2.50X12MM",
			(1 + 1.5) / 3.5},
		// A packaging word weighs 0.3: missing it costs little, carrying it helps a little.
		{"a packaging word the product lacks", "bolsas de cemento", "CEMENTO LOMA NEGRA 25KG",
			1 / 1.3},
		{"a packaging word the product carries", "bolsas de cemento", "CEMENTO EN BOLSA 10KG", 1},
		// "bol" is how a bag is typed; the synonym the seller loaded carries the rest.
		{"an abbreviated bag and a synonym", "bol de portland", "Cemento Loma Negra 50 kg portland",
			1 / 1.3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := catalogCoverage(lineTokens(tc.line), tokenizeCatalogText(tc.document))
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("coverage(%q, %q) = %.4f, want %.4f", tc.line, tc.document, got, tc.want)
			}
		})
	}
}

func TestUnaskedFigureShare_CountsTheSpecsTheLineNeverNamed(t *testing.T) {
	line := lineTokens("ramal 110 a 45")
	if got := unaskedFigureShare(line, tokenizeCatalogText("PVC RAMAL 110X45°")); got != 0 {
		t.Errorf("share for the part asked for = %v, want 0", got)
	}
	reducer := tokenizeCatalogText("PVC RAMAL 160 X 110 A 45")
	if got := unaskedFigureShare(line, reducer); math.Abs(got-1.0/3) > 1e-9 {
		t.Errorf("share for the reducer = %v, want a third", got)
	}
	if got := unaskedFigureShare(line, tokenizeCatalogText("PVC RAMAL")); got != 0 {
		t.Errorf("share for a name with no figures = %v, want 0", got)
	}
}

func TestCalibratedSimilarity_MapsTheModelsBandOntoTheScale(t *testing.T) {
	for _, tc := range []struct {
		cosine, want float64
	}{
		{0.25, 0}, {0.10, 0}, {0.90, 1}, {0.97, 1}, {0.575, 0.5},
	} {
		if got := calibratedSimilarity(tc.cosine, 0.25, 0.90); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("calibrated(%v) = %v, want %v", tc.cosine, got, tc.want)
		}
	}
	// A band with no width has no scale, and says nothing rather than divide by zero.
	if got := calibratedSimilarity(0.5, 0.5, 0.5); got != 0 {
		t.Errorf("calibrated in an empty band = %v, want 0", got)
	}
}

func TestWithinOneEdit(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"porcelanato", "porcelanato", true},
		{"porcelanato", "porcellanato", true},
		{"porselanato", "porseyanato", true},
		{"aislante", "aislador", false},
		{"cement", "cemento", true},
		{"cemento", "semento", true},
		{"hueco", "hueso", true},
		{"tubo", "tubos", true},
		{"abc", "abcde", false},
	} {
		shorter, longer := tc.a, tc.b
		if len(shorter) > len(longer) {
			shorter, longer = longer, shorter
		}
		if got := withinOneEdit(shorter, longer); got != tc.want {
			t.Errorf("withinOneEdit(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
