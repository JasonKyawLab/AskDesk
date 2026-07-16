package auth

import (
	"strings"
	"testing"
	"time"
)

func TestSignVerify_RoundTrip(t *testing.T) {
	s := NewSigner("test-secret")
	want := Claims{BusinessID: 1, AdminID: "999", Channel: "telegram", ExpiresAt: time.Now().Add(time.Minute).Unix()}

	tok, err := s.Sign(want)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	got, err := s.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got != want {
		t.Errorf("claims = %+v, want %+v", got, want)
	}
}

func TestVerify_RejectsTamperedToken(t *testing.T) {
	s := NewSigner("test-secret")
	tok, _ := s.Sign(Claims{BusinessID: 1, ExpiresAt: time.Now().Add(time.Minute).Unix()})

	// Flip the last character of the payload.
	tampered := "x" + tok[1:]
	if _, err := s.Verify(tampered); err == nil {
		t.Error("expected error for tampered token")
	}
}

func TestVerify_RejectsWrongSecret(t *testing.T) {
	tok, _ := NewSigner("secret-a").Sign(Claims{BusinessID: 1, ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if _, err := NewSigner("secret-b").Verify(tok); err == nil {
		t.Error("expected error when verifying with a different secret")
	}
}

func TestVerify_RejectsExpired(t *testing.T) {
	s := NewSigner("test-secret")
	tok, _ := s.Sign(Claims{BusinessID: 1, ExpiresAt: time.Now().Add(-time.Second).Unix()})
	if _, err := s.Verify(tok); err == nil {
		t.Error("expected error for expired token")
	}
}

func TestMagicLink(t *testing.T) {
	s := NewSigner("test-secret")
	link, err := s.MagicLink("https://askdesk.example.com/", Claims{BusinessID: 1, ExpiresAt: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "https://askdesk.example.com/edit?token=") {
		t.Errorf("unexpected link: %s", link)
	}
}
