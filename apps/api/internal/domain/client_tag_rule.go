package domain

// ClientTagRuleKind identifies an automatic client tag.
type ClientTagRuleKind string

const (
	ClientTagRecurring ClientTagRuleKind = "RECURRING"
	ClientTagAbandoner ClientTagRuleKind = "ABANDONER"
)

// Valid reports whether a client tag rule kind is supported.
func (k ClientTagRuleKind) Valid() bool {
	return k == ClientTagRecurring || k == ClientTagAbandoner
}

// ClientTagRule defines the threshold and optional rolling window for an automatic tag.
type ClientTagRule struct {
	Kind       ClientTagRuleKind
	Enabled    bool
	Minimum    int32
	WindowDays int32
}

// AutomaticClientTags holds calculated tags and explicit exclusions for a client.
type AutomaticClientTags struct {
	Applied  []ClientTag
	Excluded []ClientTag
}
