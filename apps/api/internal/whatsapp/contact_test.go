package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetaSenderContactVerificationUsesApprovedCopyCodeTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v25.0/phone-id/messages" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("invalid delivery scope")
		}
		var body struct {
			To       string
			Type     string
			Template struct {
				Name       string
				Language   struct{ Code string }
				Components []struct {
					Type       string
					SubType    string `json:"sub_type"`
					Index      string
					Parameters []struct {
						Type string
						Text string
					}
				}
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.To != "5491112345678" || body.Type != "template" || body.Template.Name != "contact_code" || body.Template.Language.Code != "es_AR" || len(body.Template.Components) != 2 {
			t.Errorf("invalid authentication payload: %+v", body)
		} else {
			a, b := body.Template.Components[0], body.Template.Components[1]
			if a.Type != "body" || len(a.Parameters) != 1 || a.Parameters[0].Text != "123456" || b.Type != "button" || b.SubType != "url" || b.Index != "0" || len(b.Parameters) != 1 || b.Parameters[0].Text != "123456" {
				t.Errorf("invalid code parameters: %+v", body.Template.Components)
			}
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.proof"}]}`))
	}))
	defer server.Close()
	sender := newMetaSender(server.Client(), server.URL, "v25.0")
	message := domain.ContactWhatsAppMessage{To: "+5491112345678", Code: "123456", Template: "contact_code", Language: "es_AR", Credentials: domain.WhatsAppDeliveryCredentials{PhoneNumberID: "phone-id", AccessToken: "secret"}}
	receipt, err := sender.SendContact(context.Background(), message)
	if err != nil || receipt == nil || receipt.ProviderReference != "wamid.proof" {
		t.Fatalf("receipt: %+v %v", receipt, err)
	}
	message.Template = ""
	if _, err := sender.SendContact(context.Background(), message); !errors.Is(err, domain.ErrNotConfigured) {
		t.Fatalf("missing approved template: %v", err)
	}
}
