package services

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Weights of one line token in the coverage figure. A figure carries the spec that tells two
// products of a family apart — 4.40 from 4.50, 1/2 from 3/4 — which is exactly what an embedding
// blurs, so it weighs more than a word. A unit only qualifies a figure, and a packaging word is
// weak evidence: some products are named by it and most are not.
const (
	wordWeight      = 1.0
	figureWeight    = 1.5
	unitWeight      = 0.4
	packagingWeight = 0.3
)

// Credit a query word earns from a catalog word that is not the same word. Only the exact form
// earns full credit, so a line that spells every word right still outranks one that did not.
const (
	phoneticCredit     = 0.9
	editDistanceCredit = 0.8
	prefixCredit       = 0.75
)

// minPrefixLength is the shortest abbreviation read as one: "pret" for pretensada, "durlo" for
// durlock. Three letters would make "cal" a prefix of calacata and caliente.
const minPrefixLength = 4

// minEditDistanceLength is the shortest word a one-letter slip is forgiven on. Below it a single
// edit turns one real word into another.
const minEditDistanceLength = 5

// matchToken is one normalized word, figure or unit of a catalog text.
type matchToken struct {
	text string
	kind tokenKind
	// value is a plain figure's numeric value, so "3" meets "3.00" and "12,5" meets "12.5".
	// NaN for fractions, which only meet their own spelling.
	value float64
	// key is a word's phonetic form, which forgives the spellings a client types by ear.
	key string
	// unit is the unit written right after a figure, so "4mm" does not meet "4L".
	unit string
	// packaging marks a word a material is counted in, such as "bolsas".
	packaging bool
}

type tokenKind int

const (
	tokenWord tokenKind = iota
	tokenFigure
	tokenUnit
)

// accentFolder folds the characters Spanish text varies on into the ones a client may type
// instead. The degree and ordinal signs only decorate a figure ("90°", "90º", "Nº") and are
// dropped; superscripts become the digit they are, so "m²" reads like "m2".
var accentFolder = strings.NewReplacer(
	"á", "a", "à", "a", "ä", "a", "â", "a", "é", "e", "è", "e", "ë", "e", "ê", "e",
	"í", "i", "ì", "i", "ï", "i", "î", "i", "ó", "o", "ò", "o", "ö", "o", "ô", "o",
	"ú", "u", "ù", "u", "ü", "u", "û", "u", "ñ", "n", "°", " ", "º", " ", "ª", " ",
	"²", "2", "³", "3", `"`, " pulgada ", "´", " ", "'", " ",
)

// thousandsFigure is a figure grouped with points ("1.000", "2.500"), the way Argentine text
// writes thousands; a decimal point takes one or two digits.
var thousandsFigure = regexp.MustCompile(`^\d{1,3}(\.\d{3})+$`)

// stopWords carry no identity of their own, including the "x" between two dimensions. A single
// letter abbreviating one ("p/" para, "c/" con, "s/" sin) is dropped by its slash instead, since
// on its own a letter can be a shape: a "perfil C" is not a "perfil U".
var stopWords = map[string]bool{
	"a": true, "al": true, "con": true, "de": true, "del": true, "e": true, "el": true,
	"en": true, "la": true, "las": true, "lo": true, "los": true, "n": true, "o": true,
	"para": true, "por": true, "sin": true, "un": true, "una": true, "x": true, "y": true,
}

// unitWords map every spelling of a unit onto one form, so "3 metros" meets "3 m".
var unitWords = map[string]string{
	"m": "m", "mt": "m", "mts": "m", "metro": "m", "metros": "m",
	"mm": "mm", "milimetro": "mm", "milimetros": "mm",
	"cm": "cm", "centimetro": "cm", "centimetros": "cm",
	"kg": "kg", "kgs": "kg", "kilo": "kg", "kilos": "kg", "kilogramo": "kg", "kilogramos": "kg",
	"g": "g", "gr": "g", "grs": "g", "gramo": "g", "gramos": "g",
	"l": "l", "lt": "l", "lts": "l", "litro": "l", "litros": "l",
	"cc": "cc", "pulgada": "in", "pulgadas": "in", "m2": "m2", "m3": "m3",
}

// dimensionUnits are units a size is written in and a quantity never is, so a figure opening a
// line in one of them is a spec: "8mm hierro" asks for 8mm iron.
var dimensionUnits = map[string]bool{"mm": true, "cm": true, "in": true}

// shapeLetters are the single letters a profile is named by: "perfil C", "perfil U", "perfil L".
var shapeLetters = map[string]bool{"c": true, "u": true, "t": true, "h": true, "z": true}

// abbreviations catalogs write that a prefix cannot recover, being shorter than one.
var abbreviations = map[string]string{"pta": "puerta", "ptas": "puerta"}

// sideWords map both spellings of a hand onto one: catalogs write "85 D" and "85 I".
var sideWords = map[string]string{
	"d": "der", "der": "der", "derecha": "der", "derecho": "der",
	"i": "izq", "izq": "izq", "izquierda": "izq", "izquierdo": "izq",
}

// packagingWords are what a client counts a material in ("10 bolsas", "rollos de cinta"). Keyed by
// stem, built through the same stemmer the tokens go through so the two cannot drift apart.
var packagingWords = stems("bol", "bolsa", "bolsas", "bolson", "bolsones", "pallet", "pallets",
	"palet", "palets", "rollo", "rollos", "unidad", "unidades", "unid", "uds", "ud", "lata",
	"latas", "caja", "cajas", "barra", "barras", "balde", "baldes", "paquete", "paquetes", "tira",
	"tiras", "tonelada", "toneladas", "viaje", "viajes")

// phoneticFolder collapses the spellings Spanish pronounces alike; "c" left over becomes "k" after.
var phoneticFolder = strings.NewReplacer(
	"ll", "y", "qu", "k", "ce", "se", "ci", "si", "v", "b", "z", "s", "h", "",
)

// tokenizeCatalogText splits a catalog name or a client's line into the tokens coverage reads.
// Figures are split from the letters around them ("8mm", "15x15x6", "q188"), decimal commas
// are read as points, and a whole number joined to a fraction ("1-1/2", "1 1/2") stays one
// figure, so a line asking for 1/2 is not read as asking for 1-1/2.
func tokenizeCatalogText(text string) []matchToken {
	folded := accentFolder.Replace(strings.ToLower(text))
	runes := []rune(folded)
	var tokens []matchToken
	var pendingWhole string
	pendingJoinable := false
	flushWhole := func() {
		if pendingWhole != "" {
			tokens = append(tokens, figureToken(pendingWhole))
			pendingWhole = ""
		}
	}
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case unicode.IsDigit(r):
			start := i
			i = scanFigure(runes, i)
			figure := string(runes[start:i])
			if thousandsFigure.MatchString(figure) {
				figure = strings.ReplaceAll(figure, ".", "")
			}
			figure = strings.ReplaceAll(figure, ",", ".")
			if strings.Contains(figure, "/") {
				if pendingWhole != "" && pendingJoinable {
					figure = pendingWhole + "+" + figure
					pendingWhole = ""
				}
				flushWhole()
				tokens = append(tokens, figureToken(figure))
				continue
			}
			flushWhole()
			if strings.Contains(figure, ".") {
				tokens = append(tokens, figureToken(figure))
				continue
			}
			pendingWhole = figure
			pendingJoinable = i < len(runes) && (runes[i] == ' ' || runes[i] == '-') &&
				i+1 < len(runes) && unicode.IsDigit(runes[i+1])
			if pendingJoinable {
				i++
			}
		case unicode.IsLetter(r):
			flushWhole()
			start := i
			for i < len(runes) && unicode.IsLetter(runes[i]) {
				i++
			}
			word := string(runes[start:i])
			// "x m2" is often written "xm2": the "x" joining a unit is the separator, not a word.
			if len(word) > 1 && word[0] == 'x' && unitWords[word[1:]] != "" {
				word = word[1:]
			}
			// A metre followed by 2 or 3 is an area or a volume, not a metre and a figure.
			if word == "m" && i < len(runes) && (runes[i] == '2' || runes[i] == '3') &&
				(i+1 == len(runes) || !unicode.IsDigit(runes[i+1])) {
				word += string(runes[i])
				i++
			}
			// A single letter before a slash abbreviates a word ("p/", "c/", "s/").
			if utf8.RuneCountInString(word) == 1 && i < len(runes) && runes[i] == '/' {
				continue
			}
			if token, ok := wordToken(word); ok {
				tokens = append(tokens, token)
			}
		default:
			flushWhole()
			i++
		}
	}
	flushWhole()
	return bindUnits(tokens)
}

// lineTokens reads a client's line for coverage. A figure opening the line is how many the client
// wants — specs follow the material they qualify — so it is dropped rather than counted as a size
// no product carries, and so is a unit bound to it: "3 metros de arena" asks for sand. A fraction
// or a size in a dimension unit ("8mm hierro") is a spec wherever it stands.
func lineTokens(description string) []matchToken {
	tokens := tokenizeCatalogText(description)
	if len(tokens) < 2 || tokens[0].kind != tokenFigure || strings.Contains(tokens[0].text, "/") ||
		dimensionUnits[tokens[0].unit] {
		return tokens
	}
	return tokens[1:]
}

// catalogCoverage is how much of a client's line a catalog text accounts for, on 0..1: every
// query token earns the best credit a document token not already spent gives it, weighted by its
// kind. A figure the document does not carry costs the most, since it is usually the wrong size,
// and spending each document token once is what keeps "18x18x33" from meeting "12-18-33".
func catalogCoverage(query, document []matchToken) float64 {
	spent := make([]bool, len(document))
	var total, earned float64
	for _, q := range query {
		weight := tokenWeight(q)
		total += weight
		best, bestAt := 0.0, -1
		for j, d := range document {
			if spent[j] {
				continue
			}
			if credit := tokenCredit(q, d); credit > best {
				best, bestAt = credit, j
				if best == 1 {
					break
				}
			}
		}
		if bestAt >= 0 {
			spent[bestAt] = true
		}
		earned += weight * best
	}
	if total == 0 {
		return 0
	}
	return earned / total
}

// unaskedFigureShare is the share of a catalog text's figures the line never asked for. Between
// two products that both answer "ramal 110 a 45", the one that also says 160 is the other part.
func unaskedFigureShare(query, document []matchToken) float64 {
	var figures, unasked int
	for _, d := range document {
		if d.kind != tokenFigure {
			continue
		}
		figures++
		if !slices.ContainsFunc(query, func(q matchToken) bool { return tokenCredit(q, d) == 1 }) {
			unasked++
		}
	}
	if figures == 0 {
		return 0
	}
	return float64(unasked) / float64(figures)
}

// calibratedSimilarity maps a cosine similarity onto 0..1 between the floor an unrelated pair of
// catalog texts reaches and the ceiling a near-verbatim one does. Raw cosine from a modern
// embedding model lives in a narrow band, and reading it as a probability is what made every
// correct match look weak.
func calibratedSimilarity(cosine, floor, ceiling float64) float64 {
	if ceiling <= floor {
		return 0
	}
	return math.Min(math.Max((cosine-floor)/(ceiling-floor), 0), 1)
}

// bindUnits folds a unit written right after a figure into it, so the pair is compared as one
// quantity. A unit standing alone ("por kg", "x m") stays a token of its own.
func bindUnits(tokens []matchToken) []matchToken {
	bound := tokens[:0]
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.kind == tokenFigure && i+1 < len(tokens) && tokens[i+1].kind == tokenUnit {
			token.unit = tokens[i+1].text
			i++
		}
		bound = append(bound, token)
	}
	return bound
}

func stems(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[stemSpanish(word)] = true
	}
	return set
}

func scanFigure(runes []rune, i int) int {
	for i < len(runes) {
		r := runes[i]
		if unicode.IsDigit(r) {
			i++
			continue
		}
		// A separator only belongs to the figure when a digit follows it: "4.50" and "3/4" are
		// one figure, the full stop ending "12." is not.
		if (r == '.' || r == ',' || r == '/') && i+1 < len(runes) && unicode.IsDigit(runes[i+1]) {
			i++
			continue
		}
		break
	}
	return i
}

func figureToken(text string) matchToken {
	value := math.NaN()
	if !strings.ContainsAny(text, "/+") {
		if parsed, err := strconv.ParseFloat(text, 64); err == nil {
			value = parsed
		}
	}
	return matchToken{text: text, kind: tokenFigure, value: value}
}

func wordToken(word string) (matchToken, bool) {
	if unit, ok := unitWords[word]; ok {
		return matchToken{text: unit, kind: tokenUnit}, true
	}
	if side, ok := sideWords[word]; ok {
		return matchToken{text: side, kind: tokenWord, key: side}, true
	}
	if stopWords[word] || (utf8.RuneCountInString(word) < 2 && !shapeLetters[word]) {
		return matchToken{}, false
	}
	if full, ok := abbreviations[word]; ok {
		word = full
	}
	stem := stemSpanish(word)
	return matchToken{text: stem, kind: tokenWord, key: phoneticKey(stem),
		packaging: packagingWords[stem]}, true
}

// stemSpanish drops the number and gender endings clients and catalogs disagree on most:
// "ladrillos huecos" against "LADRILLO HUECO", "pastina blanca" against "PASTINA BLANCO".
func stemSpanish(word string) string {
	n := len(word)
	switch {
	case n > 4 && strings.HasSuffix(word, "es") && strings.ContainsRune("rlndz", rune(word[n-3])):
		word = word[:n-2]
	case n > 3 && word[n-1] == 's' && word[n-2] != 's':
		word = word[:n-1]
	}
	if n = len(word); n > 4 && (word[n-1] == 'a' || word[n-1] == 'o') {
		word = word[:n-1]
	}
	return word
}

// phoneticKey collapses the spellings Spanish pronounces alike, so a line typed by ear —
// "ladriyo", "sement", "ierro" — still meets its product.
func phoneticKey(word string) string {
	replaced := phoneticFolder.Replace(word)
	replaced = strings.ReplaceAll(replaced, "c", "k")
	var b strings.Builder
	var last rune
	for _, r := range replaced {
		if r != last {
			b.WriteRune(r)
		}
		last = r
	}
	return b.String()
}

func tokenWeight(token matchToken) float64 {
	switch {
	case token.kind == tokenFigure:
		return figureWeight
	case token.kind == tokenUnit:
		return unitWeight
	case token.packaging:
		return packagingWeight
	default:
		return wordWeight
	}
}

func tokenCredit(q, d matchToken) float64 {
	if q.kind != d.kind {
		return 0
	}
	switch q.kind {
	case tokenFigure:
		if q.unit != "" && d.unit != "" && q.unit != d.unit {
			return 0
		}
		if q.text == d.text || (!math.IsNaN(q.value) && q.value == d.value) {
			return 1
		}
		return 0
	case tokenUnit:
		if q.text == d.text {
			return 1
		}
		return 0
	}
	if q.text == d.text {
		return 1
	}
	if q.key != "" && q.key == d.key {
		return phoneticCredit
	}
	shorter, longer := q.key, d.key
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) >= minPrefixLength && strings.HasPrefix(longer, shorter) {
		return prefixCredit
	}
	if len(shorter) >= minEditDistanceLength && withinOneEdit(shorter, longer) {
		return editDistanceCredit
	}
	return 0
}

// withinOneEdit reports whether one insertion, deletion or substitution turns a into b.
func withinOneEdit(a, b string) bool {
	if len(b)-len(a) > 1 {
		return false
	}
	i := 0
	for i < len(a) && a[i] == b[i] {
		i++
	}
	if i == len(a) {
		return true
	}
	if len(a) == len(b) {
		return a[i+1:] == b[i+1:]
	}
	return a[i:] == b[i+1:]
}
