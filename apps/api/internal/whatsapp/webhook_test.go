package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func TestVerifySignature_AcceptsOnlyTheExpectedHMAC(t *testing.T) {
	t.Parallel()
	body := []byte(`{"object":"whatsapp_business_account"}`)
	mac := hmac.New(sha256.New, []byte("app-secret"))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !VerifySignature("app-secret", body, signature) {
		t.Fatal("VerifySignature() = false, want valid signature accepted")
	}
	if VerifySignature("app-secret", body, "sha256=0000") {
		t.Fatal("VerifySignature() = true, want malformed signature refused")
	}
}

func TestParseWebhook_ExtractsTextMessages(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"object":"whatsapp_business_account",
		"entry":[{"changes":[{"field":"messages","value":{
			"metadata":{"phone_number_id":"phone-id"},
			"contacts":[{"wa_id":"5491112345678","profile":{"name":"Obra Norte"}}],
			"messages":[
				{"from":"5491112345678","id":"wamid.inbound","timestamp":"1760000000","type":"text","text":{"body":"10 bolsas de cemento"}},
				{"from":"5491112345678","id":"wamid.location","timestamp":"1760000001","type":"location"}
			]
		}}]}]
	}`)

	messages, err := ParseWebhook(payload)
	if err != nil {
		t.Fatalf("ParseWebhook() = %v, want no error", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %#v, want one text message", messages)
	}
	message := messages[0]
	if message.ExternalMessageID != "wamid.inbound" || message.DestinationIdentifier != "phone-id" ||
		message.SenderID != "5491112345678" || message.Body != "10 bolsas de cemento" {
		t.Errorf("message = %#v, want provider identifiers and text", message)
	}
	if message.SenderLabel == nil || *message.SenderLabel != "Obra Norte (5491112345678)" {
		t.Errorf("sender label = %v, want the profile and sender", message.SenderLabel)
	}
	if !message.ProviderReceivedAt.Equal(time.Unix(1760000000, 0).UTC()) || len(message.Payload) == 0 {
		t.Errorf("message metadata = %#v, want timestamp and raw payload", message)
	}
}

func TestParseWebhook_TurnsAReactionIntoAConversationOnlyMessage(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{
		"metadata":{"phone_number_id":"phone-id"},
		"messages":[
			{"from":"5491112345678","id":"wamid.like","timestamp":"1760000000","type":"reaction","reaction":{"message_id":"wamid.quote","emoji":"👍"}},
			{"from":"5491112345678","id":"wamid.unlike","timestamp":"1760000001","type":"reaction","reaction":{"message_id":"wamid.quote","emoji":""}}
		]}}]}]}`)

	messages, err := ParseWebhook(payload)
	if err != nil {
		t.Fatalf("ParseWebhook() = %v, want no error", err)
	}
	if len(messages) != 1 || messages[0].Body != "👍" || !messages[0].ConversationOnly {
		t.Fatalf("messages = %#v, want one conversation-only 👍", messages)
	}
}

func TestParseStatuses_KeepsDeliveredReadAndFailedWithTheWindowRefusal(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{
		"metadata":{"phone_number_id":"phone-id"},
		"statuses":[
			{"id":"wamid.a","status":"sent","timestamp":"1760000000"},
			{"id":"wamid.a","status":"delivered","timestamp":"1760000001"},
			{"id":"wamid.b","status":"read","timestamp":"1760000002"},
			{"id":"wamid.c","status":"failed","timestamp":"1760000003","errors":[{"code":131047}]},
			{"id":"wamid.d","status":"deleted","timestamp":"1760000004"}
		]}}]}]}`)

	statuses, err := ParseStatuses(payload)
	if err != nil {
		t.Fatalf("ParseStatuses() = %v, want no error", err)
	}
	if len(statuses) != 3 {
		t.Fatalf("statuses = %#v, want delivered, read and failed only", statuses)
	}
	if statuses[0].ProviderReference != "wamid.a" || statuses[0].Status != domain.SendTrackingStatusDelivered ||
		statuses[1].Status != domain.SendTrackingStatusViewed || statuses[0].DestinationIdentifier != "phone-id" {
		t.Errorf("statuses = %#v, want delivered then read for the branch number", statuses)
	}
	if statuses[2].Status != domain.SendTrackingStatusFailed || statuses[2].ErrorCode != "131047" ||
		!statuses[2].OutsideWindow {
		t.Errorf("failed status = %#v, want the 24 h window refusal", statuses[2])
	}
}

// A file arrives as Meta's reference to it, with its caption as the body: a voice note has none,
// and an image sent as a document keeps the filename the client gave it.
func TestParseWebhook_ExtractsFilesAsReferencesWithTheirCaption(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"object":"whatsapp_business_account",
		"entry":[{"changes":[{"field":"messages","value":{
			"metadata":{"phone_number_id":"phone-id"},
			"messages":[
				{"from":"5491112345678","id":"wamid.photo","timestamp":"1760000000","type":"image",
				 "image":{"id":"media-photo","mime_type":"image/jpeg","caption":"esto para el lunes"}},
				{"from":"5491112345678","id":"wamid.voice","timestamp":"1760000001","type":"audio",
				 "audio":{"id":"media-voice","mime_type":"audio/ogg; codecs=opus","voice":true}},
				{"from":"5491112345678","id":"wamid.doc","timestamp":"1760000002","type":"document",
				 "document":{"id":"media-doc","mime_type":"image/png","filename":"pedido.png"}}
			]
		}}]}]
	}`)

	messages, err := ParseWebhook(payload)
	if err != nil {
		t.Fatalf("ParseWebhook() = %v, want no error", err)
	}
	want := []struct {
		body  string
		media domain.InboundMedia
	}{
		{"esto para el lunes", domain.InboundMedia{ProviderID: "media-photo", MediaType: "image/jpeg",
			Kind: domain.InboundMediaImage}},
		{"", domain.InboundMedia{ProviderID: "media-voice", MediaType: "audio/ogg; codecs=opus",
			Kind: domain.InboundMediaAudio}},
		{"", domain.InboundMedia{ProviderID: "media-doc", MediaType: "image/png",
			Filename: "pedido.png", Kind: domain.InboundMediaDocument}},
	}
	if len(messages) != len(want) {
		t.Fatalf("messages = %d, want %d", len(messages), len(want))
	}
	for i, w := range want {
		if messages[i].Body != w.body || messages[i].Media == nil || *messages[i].Media != w.media {
			t.Errorf("message %d = body %q media %+v, want %q %+v", i, messages[i].Body,
				messages[i].Media, w.body, w.media)
		}
	}
}

func TestParseWebhook_RefusesAFileWithNoMediaID(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages",
		"value":{"metadata":{"phone_number_id":"phone-id"},"messages":[
		{"from":"5491112345678","id":"wamid.photo","timestamp":"1760000000","type":"image"}]}}]}]}`)
	if _, err := ParseWebhook(payload); err == nil {
		t.Fatal("ParseWebhook() = nil, want a file with no media ID refused")
	}
}
