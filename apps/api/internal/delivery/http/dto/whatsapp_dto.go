package dto

// WhatsAppWebhookResponse is returned after a webhook delivery is accepted.
type WhatsAppWebhookResponse struct {
	Received    int `json:"received"`
	CreatedRFQs int `json:"created_rfqs"`
	Duplicates  int `json:"duplicates"`
	Ignored     int `json:"ignored"`
}
