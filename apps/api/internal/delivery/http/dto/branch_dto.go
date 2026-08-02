package dto

// BranchResponse is returned by GET /v1/branches.
type BranchResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
