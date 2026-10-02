package dto

import "time"

// SaveOnboardingProgressRequest is the body for PUT /v1/onboarding.
type SaveOnboardingProgressRequest struct {
	Step        string `json:"step" binding:"required,oneof=WELCOME BRAND FIRST_BRANCH CATALOG_UPLOAD CATALOG_REVIEW TEAM COMPLETE"`
	StepStatus  string `json:"step_status" binding:"required,oneof=COMPLETED SKIPPED"`
	CurrentStep string `json:"current_step" binding:"required,oneof=WELCOME BRAND FIRST_BRANCH CATALOG_UPLOAD CATALOG_REVIEW TEAM COMPLETE"`
}

// OnboardingResponse is returned by onboarding reads and progress writes.
type OnboardingResponse struct {
	FlowVersion       int                               `json:"flow_version"`
	Status            string                            `json:"status"`
	CurrentStep       string                            `json:"current_step"`
	Steps             map[string]string                 `json:"steps"`
	Checklist         []OnboardingChecklistItemResponse `json:"checklist"`
	ChecklistHiddenAt *time.Time                        `json:"checklist_hidden_at"`
	CompletedAt       *time.Time                        `json:"completed_at"`
}

// OnboardingChecklistItemResponse is one setup step the checklist tracks; a skipped step is not done.
type OnboardingChecklistItemResponse struct {
	Step string `json:"step" enums:"BRAND,CATALOG_UPLOAD,TEAM"`
	Done bool   `json:"done"`
}
