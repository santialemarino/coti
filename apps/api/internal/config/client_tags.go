package config

import (
	"fmt"
	"math"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// DefaultClientTagRules returns the fallback account rules before environment overrides.
func DefaultClientTagRules() [2]domain.ClientTagRule {
	return [2]domain.ClientTagRule{
		{Kind: domain.ClientTagRecurring, Enabled: true, Minimum: 3},
		{Kind: domain.ClientTagAbandoner, Enabled: true, Minimum: 3},
	}
}

func loadClientTagRules(problems *[]string) [2]domain.ClientTagRule {
	rules := DefaultClientTagRules()
	prefixes := [...]string{"CLIENT_TAG_RECURRING", "CLIENT_TAG_ABANDONER"}
	for i, prefix := range prefixes {
		minimumKey, windowKey := prefix+"_MINIMUM", prefix+"_WINDOW_DAYS"
		minimum := getInt(minimumKey, int(rules[i].Minimum), problems)
		window := getInt(windowKey, int(rules[i].WindowDays), problems)
		if minimum < 1 || minimum > math.MaxInt32 {
			*problems = append(*problems, fmt.Sprintf("%s must be between 1 and %d", minimumKey, math.MaxInt32))
		}
		if window < 0 || window > math.MaxInt32 {
			*problems = append(*problems, fmt.Sprintf("%s must be between 0 and %d", windowKey, math.MaxInt32))
		}
		rules[i].Minimum, rules[i].WindowDays = int32(minimum), int32(window)
	}
	return rules
}
