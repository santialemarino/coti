package dto

import (
	"time"

	"github.com/google/uuid"
)

// NonOrderMessageResponse is a channel message that opened no order.
type NonOrderMessageResponse struct {
	ID          uuid.UUID `json:"id"`
	ChannelID   uuid.UUID `json:"channel_id"`
	Kind        string    `json:"kind" enums:"GREETING,QUESTION,OTHER,AUTOMATED"`
	SenderID    string    `json:"sender_id"`
	SenderLabel *string   `json:"sender_label"`
	Body        string    `json:"body"`
	ReceivedAt  time.Time `json:"received_at"`
}

// NonOrderMessageListResponse is returned by GET /v1/inbound-messages/non-orders.
type NonOrderMessageListResponse struct {
	Messages []NonOrderMessageResponse `json:"messages"`
}

// NonOrderOutcomeResponse is returned by POST /v1/dev/whatsapp/messages when the message is not
// an order.
type NonOrderOutcomeResponse struct {
	NonOrder NonOrderMessageResponse `json:"non_order"`
}
