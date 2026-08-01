package dto

// ErrorResponse is the body every failed request returns, so the API spec describes one
// error shape instead of one per route.
type ErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"` // set when a body failed binding or validation.
}

// HealthResponse is returned by the liveness and readiness probes. detail names the
// dependency that failed, and is only present when one did.
type HealthResponse struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}
