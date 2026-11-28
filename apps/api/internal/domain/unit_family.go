package domain

import "strings"

// unitFamily is what a unit measures. Two units of different families cannot be priced against
// each other without a conversion only the seller knows: a bag of sand against a cubic metre.
type unitFamily int

const (
	unitFamilyUnknown unitFamily = iota
	unitFamilyCount
	unitFamilyLength
	unitFamilyArea
	unitFamilyVolume
	unitFamilyWeight
)

// unitFamilies maps the first word of a unit, lowercased and unaccented, onto its family. A unit
// not listed here is unknown and never conflicts.
var unitFamilies = map[string]unitFamily{
	"u": unitFamilyCount, "un": unitFamilyCount, "uni": unitFamilyCount, "unid": unitFamilyCount,
	"unidad": unitFamilyCount, "unidades": unitFamilyCount, "ud": unitFamilyCount,
	"uds": unitFamilyCount, "bolsa": unitFamilyCount, "bolsas": unitFamilyCount,
	"bolsita": unitFamilyCount, "bolsitas": unitFamilyCount, "bol": unitFamilyCount,
	"bolson": unitFamilyCount, "bolsones": unitFamilyCount, "balde": unitFamilyCount,
	"baldes": unitFamilyCount, "rollo": unitFamilyCount, "rollos": unitFamilyCount,
	"caja": unitFamilyCount, "cajas": unitFamilyCount, "lata": unitFamilyCount,
	"latas": unitFamilyCount, "barra": unitFamilyCount, "barras": unitFamilyCount,
	"tira": unitFamilyCount, "tiras": unitFamilyCount, "paquete": unitFamilyCount,
	"paquetes": unitFamilyCount, "pallet": unitFamilyCount, "pallets": unitFamilyCount,
	"panel": unitFamilyCount, "paneles": unitFamilyCount, "placa": unitFamilyCount,
	"placas": unitFamilyCount, "pieza": unitFamilyCount, "piezas": unitFamilyCount,
	"par": unitFamilyCount, "pares": unitFamilyCount,
	"m": unitFamilyLength, "mt": unitFamilyLength, "mts": unitFamilyLength,
	"metro": unitFamilyLength, "metros": unitFamilyLength, "ml": unitFamilyLength,
	"cm": unitFamilyLength, "mm": unitFamilyLength,
	"m2": unitFamilyArea, "mt2": unitFamilyArea, "mts2": unitFamilyArea,
	"m3": unitFamilyVolume, "mt3": unitFamilyVolume, "mts3": unitFamilyVolume,
	"l": unitFamilyVolume, "lt": unitFamilyVolume, "lts": unitFamilyVolume,
	"litro": unitFamilyVolume, "litros": unitFamilyVolume,
	"kg": unitFamilyWeight, "kgs": unitFamilyWeight, "kilo": unitFamilyWeight,
	"kilos": unitFamilyWeight, "g": unitFamilyWeight, "gr": unitFamilyWeight,
	"tn": unitFamilyWeight, "t": unitFamilyWeight, "tonelada": unitFamilyWeight,
	"toneladas": unitFamilyWeight,
}

var unitFolder = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
	"²", "2", "³", "3", ".", " ")

// UnitsConflict reports whether a line asked for in requested cannot be priced in product's unit
// as it stands: a bag against a cubic metre or a kilo. An unknown or missing unit never conflicts,
// nor do two counts. A bare metre is how a client asks for sand or gravel by the cubic metre and
// for flooring by the square one, so it conflicts only with a count or a weight.
func UnitsConflict(requested, product *string) bool {
	a, b := familyOf(requested), familyOf(product)
	if a == unitFamilyUnknown || b == unitFamilyUnknown || a == b {
		return false
	}
	if a == unitFamilyLength && (b == unitFamilyVolume || b == unitFamilyArea) ||
		b == unitFamilyLength && (a == unitFamilyVolume || a == unitFamilyArea) {
		return false
	}
	return true
}

func familyOf(unit *string) unitFamily {
	if unit == nil {
		return unitFamilyUnknown
	}
	words := strings.Fields(unitFolder.Replace(strings.ToLower(*unit)))
	if len(words) == 0 {
		return unitFamilyUnknown
	}
	return unitFamilies[words[0]]
}
