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
	FlowVersion int               `json:"flow_version"`
	Status      string            `json:"status"`
	CurrentStep string            `json:"current_step"`
	Steps       map[string]string `json:"steps"`
	CompletedAt *time.Time        `json:"completed_at"`
}
