package dto

// ErrorResponse is the body every failed request returns, so the API spec describes one
// error shape instead of one per route.
type ErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"` // set when a body failed binding or validation.
}
