package dto

// ErrorResponse is the body every failed request returns, so the API spec describes one
// error shape instead of one per route.
type ErrorResponse struct {
	Error string `json:"error"`
	// Code is the stable identifier a client branches on; the English text above is for logs.
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"` // set when a body failed binding or validation.
}

// RateLimitResponse is returned when a caller has spent its allowance. It names no limit —
// only how long until retrying works.
type RateLimitResponse struct {
	Error             string `json:"error"`
	Code              string `json:"code"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}

// HealthResponse is returned by the liveness and readiness probes. detail names the
// dependency that failed, and is only present when one did.
type HealthResponse struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}
