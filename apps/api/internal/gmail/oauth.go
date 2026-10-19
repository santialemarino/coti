package gmail

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"golang.org/x/oauth2"
)

// OAuth performs the Gmail offline authorization code flow.
type OAuth struct {
	config       oauth2.Config
	timeout      time.Duration
	maxJSONBytes int64
}

// NewOAuth builds a Google authorization client for an explicitly registered redirect URL.
func NewOAuth(clientID, clientSecret, redirectURL string, timeout time.Duration, maxJSONBytes int64) (*OAuth, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" ||
		strings.TrimSpace(redirectURL) == "" || timeout <= 0 || maxJSONBytes <= 0 {
		return nil, errors.New("gmail OAuth credentials, redirect URL and limits are required")
	}
	return &OAuth{config: oauth2.Config{ClientID: clientID, ClientSecret: clientSecret,
		RedirectURL: redirectURL, Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"},
		Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token"}}, timeout: timeout,
		maxJSONBytes: maxJSONBytes}, nil
}

// AuthorizationURL returns the consent URL for an opaque state and PKCE verifier.
func (o *OAuth) AuthorizationURL(state, verifier string) string {
	return o.config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"))
}

// Exchange obtains an offline credential and verifies the identity of the authorized mailbox.
func (o *OAuth) Exchange(ctx context.Context, code, verifier string) (domain.GmailGrant, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: o.timeout})
	token, err := o.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil || token.RefreshToken == "" {
		return domain.GmailGrant{}, errors.New("gmail authorization could not obtain an offline credential")
	}
	client := &Client{http: o.config.Client(ctx, token), baseURL: apiBaseURL, maxJSONBytes: o.maxJSONBytes}
	client.http.Timeout = o.timeout
	profile, err := client.GetProfile(ctx)
	if err != nil {
		return domain.GmailGrant{}, err
	}
	if profile.EmailAddress == "" || profile.HistoryID == "" {
		return domain.GmailGrant{}, errors.New("gmail profile is incomplete")
	}
	return domain.GmailGrant{Address: profile.EmailAddress, HistoryID: profile.HistoryID,
		RefreshToken: token.RefreshToken}, nil
}
