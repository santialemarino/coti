package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
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

func TestParseWebhook_ExtractsOnlyInboundTextMessages(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"object":"whatsapp_business_account",
		"entry":[{"changes":[{"field":"messages","value":{
			"metadata":{"phone_number_id":"phone-id"},
			"contacts":[{"wa_id":"5491112345678","profile":{"name":"Obra Norte"}}],
			"messages":[
				{"from":"5491112345678","id":"wamid.inbound","timestamp":"1760000000","type":"text","text":{"body":"10 bolsas de cemento"}},
				{"from":"5491112345678","id":"wamid.image","timestamp":"1760000001","type":"image"}
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
