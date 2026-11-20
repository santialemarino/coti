package services

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^\d{1,12}(?:\.\d{1,2})?$`)

// thousandsOnlyPattern is an amount grouped in thousands with one kind of separator and no
// decimals: "1.500" is fifteen hundred to an Argentine reader and one and a half to an English one.
var thousandsOnlyPattern = regexp.MustCompile(`^\d{1,3}(?:(?:\.\d{3})+|(?:,\d{3})+)$`)

// errAmbiguousMoney refuses an amount whose only separator could be a thousands mark or a decimal one.
var errAmbiguousMoney = errors.New("ambiguous money")

func normalizeMoney(raw string) (string, *big.Rat, error) {
	value := strings.NewReplacer("$", "", " ", "", " ", "").Replace(strings.TrimSpace(raw))
	if thousandsOnlyPattern.MatchString(value) {
		return "", nil, errAmbiguousMoney
	}
	if strings.Contains(value, ",") && strings.Contains(value, ".") {
		if strings.LastIndex(value, ",") > strings.LastIndex(value, ".") {
			value = strings.ReplaceAll(value, ".", "")
			value = strings.ReplaceAll(value, ",", ".")
		} else {
			value = strings.ReplaceAll(value, ",", "")
		}
	} else {
		value = strings.ReplaceAll(value, ",", ".")
	}
	if !decimalPattern.MatchString(value) {
		return "", nil, fmt.Errorf("invalid money")
	}
	rational, ok := new(big.Rat).SetString(value)
	if !ok {
		return "", nil, fmt.Errorf("invalid money")
	}
	return value, rational, nil
}

// moneyError names the row error for an amount normalizeMoney refused or found not above zero.
func moneyError(field string, err error) string {
	if errors.Is(err, errAmbiguousMoney) {
		return "ambiguous_" + field
	}
	return "invalid_" + field
}

// sameMoney compares two decimal strings by value, so "1500" equals "1500.00".
func sameMoney(a, b string) bool {
	x, okX := new(big.Rat).SetString(a)
	y, okY := new(big.Rat).SetString(b)
	return okX && okY && x.Cmp(y) == 0
}

// sameOptionalMoney compares two nullable amounts; a missing one only equals another missing one.
func sameOptionalMoney(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return sameMoney(*a, *b)
}
