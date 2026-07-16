package telegram

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

type fakeSubmitter struct {
	gotMsg  core.Message
	replyTo string
	called  bool
}

func (f *fakeSubmitter) Submit(_ context.Context, msg core.Message, replyTo string) error {
	f.called = true
	f.gotMsg = msg
	f.replyTo = replyTo
	return nil
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

const updateJSON = `{"message":{"text":"do you deliver?","chat":{"id":555},"from":{"id":999}}}`

func TestHandler_NormalizesAndSubmits(t *testing.T) {
	sub := &fakeSubmitter{}
	h := NewHandler(sub, 1, "sekret", discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/telegram", strings.NewReader(updateJSON))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "sekret")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !sub.called {
		t.Fatal("expected submitter to be called")
	}
	if sub.gotMsg.BusinessID != 1 || sub.gotMsg.Channel != core.ChannelTelegram {
		t.Errorf("normalized message wrong: %+v", sub.gotMsg)
	}
	if sub.gotMsg.UserID != "999" || sub.gotMsg.Text != "do you deliver?" {
		t.Errorf("normalized message wrong: %+v", sub.gotMsg)
	}
	if sub.replyTo != "555" {
		t.Errorf("replyTo = %q, want %q (chat id)", sub.replyTo, "555")
	}
}

func TestHandler_RejectsBadSecret(t *testing.T) {
	sub := &fakeSubmitter{}
	h := NewHandler(sub, 1, "sekret", discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/telegram", strings.NewReader(updateJSON))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if sub.called {
		t.Error("must not submit with an invalid secret")
	}
}

func TestHandler_IgnoresNonTextUpdate(t *testing.T) {
	sub := &fakeSubmitter{}
	h := NewHandler(sub, 1, "", discardLogger())

	req := httptest.NewRequest(http.MethodPost, "/telegram", strings.NewReader(`{"message":{"chat":{"id":1}}}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if sub.called {
		t.Error("non-text update should be ignored")
	}
}
