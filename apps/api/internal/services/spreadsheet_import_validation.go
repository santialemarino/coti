package services

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^\d{1,12}(?:\.\d{1,2})?$`)

func normalizeMoney(raw string) (string, *big.Rat, error) {
	value := strings.NewReplacer("$", "", "\u00a0", "", " ", "").Replace(strings.TrimSpace(raw))
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
