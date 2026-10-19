package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClient(server *httptest.Server) *Client {
	return &Client{http: server.Client(), baseURL: server.URL, maxJSONBytes: 1 << 20}
}

func TestClient_HistoryRequestsInboxAdditionsAndPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/history" || r.URL.Query().Get("startHistoryId") != "123" ||
			r.URL.Query().Get("historyTypes") != "messageAdded" ||
			r.URL.Query().Get("labelId") != "INBOX" || r.URL.Query().Get("pageToken") != "page-2" {
			t.Errorf("unexpected history request: %s", r.URL.String())
		}
		fmt.Fprint(w, `{"history":[{"messagesAdded":[{"message":{"id":"message-1"}}]}],"historyId":"130"}`)
	}))
	defer server.Close()
	page, err := testClient(server).ListHistory(context.Background(), "123", "page-2")
	if err != nil || page.HistoryID != "130" || len(page.History) != 1 ||
		page.History[0].MessagesAdded[0].Message.ID != "message-1" {
		t.Fatalf("history page = %+v, %v", page, err)
	}
}

func TestClient_StartWatchTargetsInbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/watch" {
			t.Fatalf("unexpected watch request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Topic  string   `json:"topicName"`
			Labels []string `json:"labelIds"`
			Filter string   `json:"labelFilterBehavior"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Topic != "projects/p/topics/t" || len(body.Labels) != 1 ||
			body.Labels[0] != "INBOX" || body.Filter != "include" {
			t.Fatalf("watch body = %+v", body)
		}
		fmt.Fprint(w, `{"historyId":"123","expiration":"1700000000000"}`)
	}))
	defer server.Close()
	watch, err := testClient(server).StartWatch(context.Background(), "projects/p/topics/t")
	if err != nil || watch.HistoryID != "123" || watch.Expiration != "1700000000000" {
		t.Fatalf("watch = %+v, %v", watch, err)
	}
}

func TestClient_MessageAndAttachmentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/messages/message-1":
			if r.URL.Query().Get("format") != "full" {
				t.Fatalf("message format = %q", r.URL.Query().Get("format"))
			}
			fmt.Fprint(w, `{"id":"message-1","labelIds":["INBOX"],"payload":{"parts":[{"filename":"order.pdf","body":{"attachmentId":"file-1","size":8}}]}}`)
		case "/messages/message-1/attachments/file-1":
			fmt.Fprint(w, `{"size":8,"data":"cGRmLWRhdGE"}`)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := testClient(server)
	message, err := client.GetMessage(context.Background(), "message-1", "full")
	if err != nil || len(message.Payload.Parts) != 1 ||
		message.Payload.Parts[0].Body.AttachmentID != "file-1" {
		t.Fatalf("message = %+v, %v", message, err)
	}
	body, err := client.GetAttachment(context.Background(), message.ID, "file-1")
	if err != nil || body.Size != 8 || body.Data != "cGRmLWRhdGE" {
		t.Fatalf("attachment = %+v, %v", body, err)
	}
}

func TestClient_ExpiredHistoryAndDeletedMessagesAreDistinct(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client := testClient(server)
	if _, err := client.ListHistory(context.Background(), "123", ""); !errors.Is(err, ErrHistoryExpired) {
		t.Errorf("history error = %v, want expired history", err)
	}
	if _, err := client.GetMessage(context.Background(), "deleted", "raw"); !errors.Is(err, ErrMessageGone) {
		t.Errorf("message error = %v, want deleted message", err)
	}
}

func TestClient_FullSyncQueriesInboxSinceConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" || r.URL.Query().Get("labelIds") != "INBOX" ||
			r.URL.Query().Get("q") != "after:1767225599" || r.URL.Query().Get("pageToken") != "next" {
			t.Fatalf("unexpected full sync request: %s", r.URL.String())
		}
		fmt.Fprint(w, `{"messages":[{"id":"m1"}],"nextPageToken":"after"}`)
	}))
	defer server.Close()
	refs, next, err := testClient(server).ListMessagesPage(context.Background(),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "next")
	if err != nil || len(refs) != 1 || refs[0].ID != "m1" || next != "after" {
		t.Fatalf("references = %+v, next = %q, error = %v", refs, next, err)
	}
}

func TestClient_RejectsOversizedResponsesWithoutIncludingProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Repeat("private message content", 20))
	}))
	defer server.Close()
	client := testClient(server)
	client.maxJSONBytes = 10
	_, err := client.GetProfile(context.Background())
	if err == nil || strings.Contains(err.Error(), "private message content") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestNewClient_RequiresCredentialsAndBounds(t *testing.T) {
	if _, err := NewClient(context.Background(), "id", "secret", "", time.Second, 1024); err == nil {
		t.Fatal("a missing refresh token must be refused")
	}
	if _, err := NewClient(context.Background(), "id", "secret", "refresh", 0, 1024); err == nil {
		t.Fatal("a missing timeout must be refused")
	}
}
