package domain

import "github.com/google/uuid"

// OnboardingReadiness is a live operational diagnostic, independent of wizard history.
type OnboardingReadiness struct {
	AccountReady bool
	Branches     []BranchReadiness
}

// BranchReadiness describes the active catalog and usable prices of one authorized branch.
type BranchReadiness struct {
	ID            uuid.UUID
	Name          string
	Products      int
	MissingPrices int
	HasStaff      bool
	HasChannel    bool
}
