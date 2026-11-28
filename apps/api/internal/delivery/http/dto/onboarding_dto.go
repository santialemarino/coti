package dto

import (
	"github.com/google/uuid"
	"time"
)

// OnboardingReadinessResponse is the current operational setup diagnostic.
type OnboardingReadinessResponse struct {
	AccountReady bool                      `json:"account_ready"`
	Branches     []BranchReadinessResponse `json:"branches"`
}

// BranchReadinessResponse names missing configuration without exposing integration credentials.
type BranchReadinessResponse struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Products      int       `json:"products"`
	MissingPrices int       `json:"missing_prices"`
	HasStaff      bool      `json:"has_staff"`
	HasChannel    bool      `json:"has_channel"`
}

// SaveOnboardingProgressRequest is the body for PUT /v1/onboarding.
type SaveOnboardingProgressRequest struct {
	Step        string `json:"step" binding:"required,oneof=WELCOME BRAND PORTAL FIRST_BRANCH CATALOG_UPLOAD CATALOG_REVIEW TEAM COMPLETE"`
	StepStatus  string `json:"step_status" binding:"required,oneof=COMPLETED SKIPPED"`
	CurrentStep string `json:"current_step" binding:"required,oneof=WELCOME BRAND PORTAL FIRST_BRANCH CATALOG_UPLOAD CATALOG_REVIEW TEAM COMPLETE"`
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
	Step string `json:"step" enums:"BRAND,PORTAL,CATALOG_UPLOAD,TEAM"`
	Done bool   `json:"done"`
}
