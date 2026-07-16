// Package auth issues and verifies the signed, short-lived tokens behind the
// magic-link FAQ editor. Possession of the admin's messaging account already
// proved identity, so a signed link is the login — no passwords.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
)

// ErrInvalidToken means the token is malformed, tampered with, or expired.
var ErrInvalidToken = errors.New("invalid or expired token")

// Claims is the signed payload identifying an authorized admin session.
type Claims struct {
	BusinessID int64  `json:"biz"`
	AdminID    string `json:"adm"` // the admin's channel external id
	Channel    string `json:"chn"`
	ExpiresAt  int64  `json:"exp"` // unix seconds
}

// Signer signs and verifies tokens with an HMAC secret.
type Signer struct {
	secret []byte
}

// NewSigner constructs a Signer from a secret key.
func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

// Sign returns a "payload.signature" token for the claims.
func (s *Signer) Sign(c Claims) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + s.mac(body), nil
}

// Verify checks the signature and expiry, returning the claims.
func (s *Signer) Verify(token string) (Claims, error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return Claims{}, ErrInvalidToken
	}
	if !hmac.Equal([]byte(sig), []byte(s.mac(body))) {
		return Claims{}, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if time.Now().Unix() > c.ExpiresAt {
		return Claims{}, ErrInvalidToken
	}
	return c, nil
}

// MagicLink builds the full editor URL carrying a freshly signed token.
func (s *Signer) MagicLink(baseURL string, c Claims) (string, error) {
	tok, err := s.Sign(c)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(baseURL, "/") + "/edit?token=" + url.QueryEscape(tok), nil
}

func (s *Signer) mac(body string) string {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
