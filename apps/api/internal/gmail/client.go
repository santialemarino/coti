// Package gmail connects a branch mailbox to the Gmail API.
package gmail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"golang.org/x/oauth2"
)

const apiBaseURL = "https://gmail.googleapis.com/gmail/v1/users/me"

// ErrHistoryExpired means Gmail requires a full mailbox synchronization.
var ErrHistoryExpired = domain.ErrEmailHistoryExpired

// ErrMessageGone means the mailbox no longer contains a requested message.
var ErrMessageGone = domain.ErrEmailMessageGone

// Client calls Gmail with the mailbox owner's OAuth authorization.
type Client struct {
	http         *http.Client
	baseURL      string
	maxJSONBytes int64
}

// NewClient builds a client that refreshes its access token as needed.
func NewClient(ctx context.Context, clientID, clientSecret, refreshToken string,
	timeout time.Duration, maxJSONBytes int64,
) (*Client, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" ||
		strings.TrimSpace(refreshToken) == "" || timeout <= 0 || maxJSONBytes <= 0 {
		return nil, errors.New("gmail client credentials, timeout, and response limit are required")
	}
	cfg := oauth2.Config{
		ClientID: clientID, ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{TokenURL: "https://oauth2.googleapis.com/token"},
	}
	httpClient := cfg.Client(ctx, &oauth2.Token{RefreshToken: refreshToken})
	httpClient.Timeout = timeout
	return &Client{http: httpClient, baseURL: apiBaseURL, maxJSONBytes: maxJSONBytes}, nil
}

// Profile identifies the mailbox for which the owner granted access.
type Profile struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    string `json:"historyId"`
}

// Watch describes a mailbox watch and its expiration.
type Watch struct {
	HistoryID  string `json:"historyId"`
	Expiration string `json:"expiration"`
}

// HistoryPage holds a page of new messages and the next synchronization cursor.
type HistoryPage struct {
	History []struct {
		MessagesAdded []struct {
			Message MessageReference `json:"message"`
		} `json:"messagesAdded"`
	} `json:"history"`
	NextPageToken string `json:"nextPageToken"`
	HistoryID     string `json:"historyId"`
}

// MessageReference is a message's stable provider identifier.
type MessageReference struct {
	ID string `json:"id"`
}

// Message is the metadata and MIME structure returned by Gmail.
type Message struct {
	ID           string      `json:"id"`
	ThreadID     string      `json:"threadId"`
	LabelIDs     []string    `json:"labelIds"`
	InternalDate string      `json:"internalDate"`
	Payload      MessagePart `json:"payload"`
	Raw          string      `json:"raw"`
}

// MessagePart is a MIME part parsed by Gmail.
type MessagePart struct {
	PartID   string        `json:"partId"`
	MimeType string        `json:"mimeType"`
	Filename string        `json:"filename"`
	Headers  []Header      `json:"headers"`
	Body     PartBody      `json:"body"`
	Parts    []MessagePart `json:"parts"`
}

// Header is a message or MIME part header.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PartBody locates a MIME part's bytes in Gmail.
type PartBody struct {
	AttachmentID string `json:"attachmentId"`
	Size         int64  `json:"size"`
	Data         string `json:"data"`
}

// GetProfile returns the authorized mailbox and its current history cursor.
func (c *Client) GetProfile(ctx context.Context) (Profile, error) {
	var result Profile
	err := c.get(ctx, "/profile", nil, &result)
	return result, err
}

// StartWatch subscribes an inbox to notifications through a configured Pub/Sub topic.
func (c *Client) StartWatch(ctx context.Context, topic string) (Watch, error) {
	var result Watch
	if strings.TrimSpace(topic) == "" {
		return result, errors.New("gmail watch topic is required")
	}
	body := map[string]any{"topicName": topic, "labelIds": []string{"INBOX"},
		"labelFilterBehavior": "include"}
	err := c.request(ctx, http.MethodPost, "/watch", nil, body, &result)
	return result, err
}

// ListHistory returns a page of messages added to the inbox after the saved cursor.
func (c *Client) ListHistory(ctx context.Context, startHistoryID, pageToken string) (HistoryPage, error) {
	var result HistoryPage
	if strings.TrimSpace(startHistoryID) == "" {
		return result, errors.New("gmail history cursor is required")
	}
	params := url.Values{"startHistoryId": {startHistoryID}, "labelId": {"INBOX"},
		"historyTypes": {"messageAdded"}, "maxResults": {strconv.Itoa(500)}}
	if pageToken != "" {
		params.Set("pageToken", pageToken)
	}
	err := c.get(ctx, "/history", params, &result)
	if errors.Is(err, errNotFound) {
		return result, ErrHistoryExpired
	}
	return result, err
}

// GetMessage returns a message in Gmail's full or raw format.
func (c *Client) GetMessage(ctx context.Context, id, format string) (Message, error) {
	var result Message
	if id == "" || (format != "full" && format != "raw") {
		return result, errors.New("gmail message ID and supported format are required")
	}
	err := c.get(ctx, "/messages/"+url.PathEscape(id), url.Values{"format": {format}}, &result)
	if errors.Is(err, errNotFound) {
		return result, ErrMessageGone
	}
	return result, err
}

// GetAttachment returns a part whose bytes Gmail keeps separately from the message payload.
func (c *Client) GetAttachment(ctx context.Context, messageID, attachmentID string) (PartBody, error) {
	var result PartBody
	if messageID == "" || attachmentID == "" {
		return result, errors.New("gmail message and attachment IDs are required")
	}
	err := c.get(ctx, "/messages/"+url.PathEscape(messageID)+"/attachments/"+
		url.PathEscape(attachmentID), nil, &result)
	if errors.Is(err, errNotFound) {
		return result, ErrMessageGone
	}
	return result, err
}

// ListMessagesPage returns one page of the inbox for a full synchronization.
func (c *Client) ListMessagesPage(ctx context.Context, since time.Time, pageToken string) ([]MessageReference, string, error) {
	if since.IsZero() {
		return nil, "", errors.New("gmail synchronization start time is required")
	}
	params := url.Values{"labelIds": {"INBOX"}, "maxResults": {strconv.Itoa(500)},
		"q": {"after:" + strconv.FormatInt(since.Unix()-1, 10)}}
	if pageToken != "" {
		params.Set("pageToken", pageToken)
	}
	var result struct {
		Messages      []MessageReference `json:"messages"`
		NextPageToken string             `json:"nextPageToken"`
	}
	err := c.get(ctx, "/messages", params, &result)
	return result.Messages, result.NextPageToken, err
}

var errNotFound = errors.New("gmail resource not found")

func (c *Client) get(ctx context.Context, path string, params url.Values, result any) error {
	return c.request(ctx, http.MethodGet, path, params, nil, result)
}

func (c *Client) request(ctx context.Context, method, path string, params url.Values,
	body, result any,
) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	target := c.baseURL + path
	if len(params) != 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return fmt.Errorf("create gmail request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("gmail request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gmail request returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxJSONBytes+1))
	if err != nil {
		return fmt.Errorf("read gmail response: %w", err)
	}
	if int64(len(data)) > c.maxJSONBytes {
		return errors.New("gmail response exceeds the configured limit")
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("decode gmail response: %w", err)
	}
	return nil
}
