package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMailbox_RetainsUnparseableOriginalForManualReview(t *testing.T) {
	original := []byte("not a valid RFC 5322 message")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":"m1","internalDate":"1767225600000","raw":%q}`,
			base64.RawURLEncoding.EncodeToString(original))
	}))
	defer server.Close()
	mailbox, err := NewMailbox(testClient(server), 1024, 1024, 5)
	if err != nil {
		t.Fatal(err)
	}
	message, err := mailbox.ReadMessage(context.Background(), "m1")
	if err != nil || string(message.Original) != string(original) || message.Body != "" ||
		message.ExternalMessageID != "m1" || message.ProviderReceivedAt.Unix() != 1767225600 {
		t.Fatalf("message = %+v, error = %v", message, err)
	}
}
