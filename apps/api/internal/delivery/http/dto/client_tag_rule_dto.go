package dto

// ClientTagRuleResponse represents the effective configuration of an automatic tag.
type ClientTagRuleResponse struct {
	Kind       string `json:"kind" enums:"RECURRING,ABANDONER"`
	Enabled    bool   `json:"enabled"`
	Minimum    int32  `json:"minimum"`
	WindowDays int32  `json:"window_days"`
}

// ClientTagRulesResponse lists the account's automatic tag rules.
type ClientTagRulesResponse struct {
	Items []ClientTagRuleResponse `json:"items"`
}

// UpdateClientTagRuleRequest supplies the full configuration of one automatic tag.
type UpdateClientTagRuleRequest struct {
	Enabled    *bool  `json:"enabled" binding:"required"`
	Minimum    int32  `json:"minimum" binding:"min=1"`
	WindowDays *int32 `json:"window_days" binding:"required,gte=0"`
}

// SetClientTagExclusionRequest selects whether the client is excluded from an automatic rule.
type SetClientTagExclusionRequest struct {
	Excluded *bool `json:"excluded" binding:"required"`
}
