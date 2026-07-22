package messenger

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type fakeSubmitter struct {
	called  bool
	msg     core.Message
	replyTo string
}

func (f *fakeSubmitter) Submit(_ context.Context, msg core.Message, replyTo string) error {
	f.called = true
	f.msg = msg
	f.replyTo = replyTo
	return nil
}

const messageJSON = `{"object":"page","entry":[{"messaging":[{"sender":{"id":"USER123"},"message":{"text":"do you deliver?"}}]}]}`

func sign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHandler_VerifyHandshake(t *testing.T) {
	h := NewHandler(&fakeSubmitter{}, 1, "sekret", "verify-me", discardLogger())

	req := httptest.NewRequest(http.MethodGet,
		"/webhook/messenger?hub.mode=subscribe&hub.verify_token=verify-me&hub.challenge=CHALLENGE42", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "CHALLENGE42" {
		t.Errorf("body = %q, want CHALLENGE42", rec.Body.String())
	}
}

func TestHandler_VerifyWrongToken(t *testing.T) {
	h := NewHandler(&fakeSubmitter{}, 1, "sekret", "verify-me", discardLogger())

	req := httptest.NewRequest(http.MethodGet,
		"/webhook/messenger?hub.mode=subscribe&hub.verify_token=WRONG&hub.challenge=x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHandler_NormalizesAndSubmits(t *testing.T) {
	sub := &fakeSubmitter{}
	h := NewHandler(sub, 1, "sekret", "v", discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/webhook/messenger", strings.NewReader(messageJSON))
	req.Header.Set("X-Hub-Signature-256", sign("sekret", messageJSON))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !sub.called {
		t.Fatal("expected submit to be called")
	}
	if sub.msg.Channel != core.ChannelMessenger {
		t.Errorf("channel = %q, want messenger", sub.msg.Channel)
	}
	if sub.msg.Text != "do you deliver?" {
		t.Errorf("text = %q", sub.msg.Text)
	}
	if sub.msg.UserID != "USER123" || sub.replyTo != "USER123" {
		t.Errorf("userID/replyTo = %q/%q, want USER123", sub.msg.UserID, sub.replyTo)
	}
}

func TestHandler_RejectsBadSignature(t *testing.T) {
	sub := &fakeSubmitter{}
	h := NewHandler(sub, 1, "sekret", "v", discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/webhook/messenger", strings.NewReader(messageJSON))
	req.Header.Set("X-Hub-Signature-256", sign("wrong-secret", messageJSON))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if sub.called {
		t.Error("must not submit with an invalid signature")
	}
}

func TestHandler_IgnoresEchoAndNonText(t *testing.T) {
	sub := &fakeSubmitter{}
	h := NewHandler(sub, 1, "", "v", discardLogger()) // empty secret: signature check disabled

	// An echo of our own outbound message plus a delivery receipt — neither is a
	// customer question.
	const body = `{"object":"page","entry":[{"messaging":[{"sender":{"id":"PAGE"},"message":{"text":"hi","is_echo":true}},{"sender":{"id":"U"},"delivery":{}}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/messenger", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if sub.called {
		t.Error("echo/non-text events must not be submitted")
	}
}
