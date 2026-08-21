// Package storage holds the adapters behind the domain.ObjectStorage port. Which one is in
// use is a startup decision the composition root makes; nothing above the port knows.
package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// LinkPath is where the API serves the objects the local adapter signs links for. The adapter
// builds those links and the router mounts the route, so the two read it from here.
const LinkPath = "/v1/files"

// URLSigner signs an object key together with the instant its link stops working, and checks
// both again on the way back. Spaces signs its own links; this is how the local adapter gets a
// link that expires on its own rather than one a reader has to be trusted to honour.
type URLSigner struct {
	secret []byte
	now    func() time.Time
}

// NewURLSigner builds a URLSigner over secret, reading the clock through now. A nil now means
// time.Now.
func NewURLSigner(secret []byte, now func() time.Time) *URLSigner {
	if now == nil {
		now = time.Now
	}
	return &URLSigner{secret: secret, now: now}
}

// Sign returns the signature covering exactly this key and deadline.
func (s *URLSigner) Sign(key string, expiresAt time.Time) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(key))
	mac.Write([]byte{'\n'})
	mac.Write([]byte(strconv.FormatInt(expiresAt.Unix(), 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify refuses a signature that does not cover this key and deadline, and a deadline that has
// passed. Both are ErrForbidden; only the code tells them apart, so a client can offer a fresh
// link for the one case where that helps.
func (s *URLSigner) Verify(key string, expiresAt time.Time, signature string) error {
	expected := s.Sign(key, expiresAt)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return domain.WithCode(domain.CodeInvalidLink,
			fmt.Errorf("%w: signature does not cover this object", domain.ErrForbidden))
	}
	if !s.now().Before(expiresAt) {
		return domain.WithCode(domain.CodeLinkExpired,
			fmt.Errorf("%w: link expired", domain.ErrForbidden))
	}
	return nil
}

// deadline returns the instant a link created now stops working, at the whole-second precision
// the signature covers.
func (s *URLSigner) deadline(expiresIn time.Duration) time.Time {
	return time.Unix(s.now().Add(expiresIn).Unix(), 0)
}
