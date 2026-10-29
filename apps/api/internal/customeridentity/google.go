package customeridentity

import (
	"context"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"golang.org/x/oauth2"
)

// Google verifies authorization-code responses against Google's signature, issuer, audience and nonce.
type Google struct {
	clientID     string
	clientSecret string
	timeout      time.Duration
}

// NewGoogle builds the external OpenID Connect adapter without fetching metadata at startup.
func NewGoogle(clientID, clientSecret string, timeout time.Duration) *Google {
	return &Google{clientID: clientID, clientSecret: clientSecret, timeout: timeout}
}

// Exchange consumes a PKCE-bound code and discards provider access tokens after verifying identity.
func (g *Google) Exchange(ctx context.Context, code, verifier, redirect, nonce string) (*domain.CustomerGoogleIdentity, error) {
	if g.clientID == "" || g.clientSecret == "" {
		return nil, domain.ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, err
	}
	cfg := oauth2.Config{ClientID: g.clientID, ClientSecret: g.clientSecret, Endpoint: provider.Endpoint(), RedirectURL: redirect}
	tokens, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, domain.ErrUnauthenticated
	}
	raw, ok := tokens.Extra("id_token").(string)
	if !ok {
		return nil, domain.ErrUnauthenticated
	}
	id, err := provider.Verifier(&oidc.Config{ClientID: g.clientID}).Verify(ctx, raw)
	if err != nil || id.Nonce != nonce {
		return nil, domain.ErrUnauthenticated
	}
	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Name     string `json:"name"`
	}
	if err := id.Claims(&claims); err != nil || id.Subject == "" {
		return nil, domain.ErrUnauthenticated
	}
	return &domain.CustomerGoogleIdentity{Subject: id.Subject, Email: claims.Email, Name: claims.Name, EmailVerified: claims.Verified}, nil
}
