package editor

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/JasonKyawLab/AskDesk/internal/auth"
	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

type fakeFAQs struct{}

func (fakeFAQs) List(context.Context, int64) ([]store.FAQ, error) { return nil, nil }
func (fakeFAQs) Create(context.Context, int64, string, string, string) (int64, error) {
	return 1, nil
}
func (fakeFAQs) Update(context.Context, int64, int64, string, string, string) error { return nil }
func (fakeFAQs) Delete(context.Context, int64, int64) error                         { return nil }

type fakeSettings struct{}

func (fakeSettings) RawSettings(context.Context, int64) (store.BusinessSettings, error) {
	return store.BusinessSettings{}, nil
}
func (fakeSettings) UpdateSettings(context.Context, int64, store.BusinessSettings) error { return nil }

type fakeAdmin struct {
	target   store.UnansweredTarget
	resolved bool
}

func (f *fakeAdmin) PendingUnanswered(context.Context, int64, int) ([]store.PendingQuestion, error) {
	return nil, nil
}
func (f *fakeAdmin) GetUnanswered(context.Context, int64, int64) (store.UnansweredTarget, error) {
	return f.target, nil
}
func (f *fakeAdmin) ResolveUnanswered(context.Context, int64, int64) error {
	f.resolved = true
	return nil
}

type fakeDel struct {
	channel core.Channel
	replyTo string
	text    string
	called  bool
}

func (d *fakeDel) Deliver(_ context.Context, ch core.Channel, replyTo, text string) error {
	d.called, d.channel, d.replyTo, d.text = true, ch, replyTo, text
	return nil
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestHandleReply_DeliversAndResolves(t *testing.T) {
	signer := auth.NewSigner("k")
	adm := &fakeAdmin{target: store.UnansweredTarget{Channel: core.ChannelWidget, ReplyTo: "s1"}}
	del := &fakeDel{}
	h := NewHandler(fakeFAQs{}, fakeSettings{}, adm, del, signer, false, discardLogger())

	tok, _ := signer.Sign(auth.Claims{BusinessID: 1, ExpiresAt: time.Now().Add(time.Minute).Unix()})
	form := url.Values{"id": {"7"}, "message": {"We ship in 2 days."}}
	req := httptest.NewRequest(http.MethodPost, "/edit/reply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	rec := httptest.NewRecorder()

	h.HandleReply(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if !del.called || del.channel != core.ChannelWidget || del.replyTo != "s1" || del.text != "We ship in 2 days." {
		t.Errorf("reply not delivered correctly: %+v", del)
	}
	if !adm.resolved {
		t.Error("item should be resolved after reply")
	}
}

func TestHandleReply_NoSessionDenied(t *testing.T) {
	h := NewHandler(fakeFAQs{}, fakeSettings{}, &fakeAdmin{}, &fakeDel{}, auth.NewSigner("k"), false, discardLogger())
	req := httptest.NewRequest(http.MethodPost, "/edit/reply", strings.NewReader("id=1&message=hi"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.HandleReply(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 without a session", rec.Code)
	}
}
