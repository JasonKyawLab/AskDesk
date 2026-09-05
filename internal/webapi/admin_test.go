package webapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

type fakeAdminStore struct {
	pending  []store.PendingQuestion
	target   store.UnansweredTarget
	getErr   error
	resolved bool
}

func (f *fakeAdminStore) TodayStats(context.Context, int64) (store.DailyStats, error) {
	return store.DailyStats{Total: 4, Answered: 1, Unanswered: 3}, nil
}
func (f *fakeAdminStore) PendingUnanswered(context.Context, int64, int) ([]store.PendingQuestion, error) {
	return f.pending, nil
}
func (f *fakeAdminStore) GetUnanswered(context.Context, int64, int64) (store.UnansweredTarget, error) {
	return f.target, f.getErr
}
func (f *fakeAdminStore) ResolveUnanswered(context.Context, int64, int64) error {
	f.resolved = true
	return nil
}

type fakeAdminDeliverer struct {
	channel core.Channel
	replyTo string
	text    string
	called  bool
}

func (d *fakeAdminDeliverer) Deliver(_ context.Context, ch core.Channel, replyTo, text string) error {
	d.called, d.channel, d.replyTo, d.text = true, ch, replyTo, text
	return nil
}

type fakeAdminAuth struct{ valid string }

func (f fakeAdminAuth) IDByAdminKey(_ context.Context, key string) (int64, error) {
	if key == f.valid {
		return 1, nil
	}
	return 0, store.ErrUnknownAPIKey
}

func adminReq(h *AdminHandler, method, path, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("X-Admin-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newAdmin(s AdminStore, del Deliverer) *AdminHandler {
	return NewAdmin(s, nil, nil, del, fakeAdminAuth{valid: "adminkey"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

type fakeAdminLeads struct {
	list      []store.Lead
	profile   store.LeadProfile
	deleted   string
	deleteHit bool
}

func (f fakeAdminLeads) List(context.Context, int64, int) ([]store.Lead, error) { return f.list, nil }
func (f fakeAdminLeads) Profile(context.Context, int64, string) (store.LeadProfile, error) {
	return f.profile, nil
}
func (f *fakeAdminLeads) Delete(_ context.Context, _ int64, session string) error {
	f.deleted, f.deleteHit = session, true
	return nil
}

func TestAdmin_Leads(t *testing.T) {
	leads := &fakeAdminLeads{list: []store.Lead{{Name: "Aung", Email: "a@b.com", Phone: "09"}}}
	h := NewAdmin(&fakeAdminStore{}, leads, nil, &fakeAdminDeliverer{}, fakeAdminAuth{valid: "adminkey"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := adminReq(h, http.MethodGet, "/api/v1/admin/leads", "adminkey", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "a@b.com") || !strings.Contains(rec.Body.String(), "Aung") {
		t.Errorf("leads response missing data: %s", rec.Body.String())
	}
}

func TestAdmin_RejectsMissingKey(t *testing.T) {
	rec := adminReq(newAdmin(&fakeAdminStore{}, &fakeAdminDeliverer{}), http.MethodGet, "/api/v1/admin/pending", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAdmin_RejectsPublicKeyIsSeparate(t *testing.T) {
	// The public api key must not work on the admin API.
	rec := adminReq(newAdmin(&fakeAdminStore{}, &fakeAdminDeliverer{}), http.MethodGet, "/api/v1/admin/pending", "publickey", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("a non-admin key must be rejected, got %d", rec.Code)
	}
}

func TestAdmin_Pending(t *testing.T) {
	s := &fakeAdminStore{pending: []store.PendingQuestion{{ID: 3, Question: "hours?", UserName: "Aung"}}}
	rec := adminReq(newAdmin(s, &fakeAdminDeliverer{}), http.MethodGet, "/api/v1/admin/pending", "adminkey", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"id":3`) || !strings.Contains(rec.Body.String(), "hours?") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestAdmin_ReplyRoutesAndResolves(t *testing.T) {
	s := &fakeAdminStore{target: store.UnansweredTarget{Channel: core.ChannelWidget, ReplyTo: "sess-1"}}
	del := &fakeAdminDeliverer{}
	rec := adminReq(newAdmin(s, del), http.MethodPost, "/api/v1/admin/reply", "adminkey", `{"id":3,"message":"Open 9-6."}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !del.called || del.channel != core.ChannelWidget || del.replyTo != "sess-1" || del.text != "Open 9-6." {
		t.Errorf("reply not delivered correctly: %+v", del)
	}
	if !s.resolved {
		t.Error("item should be resolved")
	}
}

func TestAdmin_ReplyMissingFields(t *testing.T) {
	rec := adminReq(newAdmin(&fakeAdminStore{}, &fakeAdminDeliverer{}), http.MethodPost, "/api/v1/admin/reply", "adminkey", `{"id":3}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAdmin_Dismiss(t *testing.T) {
	s := &fakeAdminStore{}
	rec := adminReq(newAdmin(s, &fakeAdminDeliverer{}), http.MethodPost, "/api/v1/admin/dismiss", "adminkey", `{"id":3}`)
	if rec.Code != http.StatusOK || !s.resolved {
		t.Errorf("dismiss failed: status=%d resolved=%v", rec.Code, s.resolved)
	}
}

type fakeAnalytics struct{}

func (fakeAnalytics) AnswerRate(context.Context, int64, int) (store.AnswerStats, error) {
	return store.AnswerStats{Total: 10, Answered: 7, Unanswered: 3}, nil
}
func (fakeAnalytics) TopQuestions(_ context.Context, _ int64, _, _ int, onlyUnanswered bool) ([]store.QuestionCount, error) {
	if onlyUnanswered {
		return []store.QuestionCount{{Question: "do you deliver?", Count: 4, Unanswered: 4}}, nil
	}
	return []store.QuestionCount{{Question: "how to sign up?", Count: 9, Unanswered: 0}}, nil
}
func (fakeAnalytics) BusyHours(context.Context, int64, int) ([]store.HourBucket, error) {
	return []store.HourBucket{{Hour: 14, Count: 5}}, nil
}
func (fakeAnalytics) BusyDays(context.Context, int64, int) ([]store.DayBucket, error) {
	return []store.DayBucket{{Weekday: 1, Count: 6}}, nil
}

func TestAdmin_Analytics(t *testing.T) {
	h := NewAdmin(&fakeAdminStore{}, nil, fakeAnalytics{}, &fakeAdminDeliverer{},
		fakeAdminAuth{valid: "adminkey"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := adminReq(h, http.MethodGet, "/api/v1/admin/analytics?days=30", "adminkey", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`"answered_pct":70`, "how to sign up?", "do you deliver?", `"hour":14`, `"weekday":1`} {
		if !strings.Contains(body, want) {
			t.Errorf("analytics body missing %q: %s", want, body)
		}
	}
}

func TestAdmin_AnalyticsDisabledWhenNil(t *testing.T) {
	rec := adminReq(newAdmin(&fakeAdminStore{}, &fakeAdminDeliverer{}), http.MethodGet, "/api/v1/admin/analytics", "adminkey", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when analytics nil", rec.Code)
	}
}

func TestAdmin_LeadProfile(t *testing.T) {
	leads := &fakeAdminLeads{profile: store.LeadProfile{
		Lead:     store.Lead{SessionID: "s1", Name: "Aung", Email: "a@b.com"},
		Messages: []store.LeadMessage{{Question: "hours?", Answered: true, Channel: "widget"}},
	}}
	h := NewAdmin(&fakeAdminStore{}, leads, nil, &fakeAdminDeliverer{},
		fakeAdminAuth{valid: "adminkey"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := adminReq(h, http.MethodGet, "/api/v1/admin/lead?session=s1", "adminkey", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "hours?") || !strings.Contains(rec.Body.String(), "a@b.com") {
		t.Errorf("profile body wrong: %s", rec.Body.String())
	}
}

func TestAdmin_LeadProfileRequiresSession(t *testing.T) {
	h := NewAdmin(&fakeAdminStore{}, &fakeAdminLeads{}, nil, &fakeAdminDeliverer{},
		fakeAdminAuth{valid: "adminkey"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := adminReq(h, http.MethodGet, "/api/v1/admin/lead", "adminkey", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 without session", rec.Code)
	}
}

func TestAdmin_LeadDelete(t *testing.T) {
	leads := &fakeAdminLeads{}
	h := NewAdmin(&fakeAdminStore{}, leads, nil, &fakeAdminDeliverer{},
		fakeAdminAuth{valid: "adminkey"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := adminReq(h, http.MethodPost, "/api/v1/admin/lead/delete", "adminkey", `{"session_id":"s1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !leads.deleteHit || leads.deleted != "s1" {
		t.Errorf("delete not called correctly: %+v", leads)
	}
}

func TestAdmin_LeadDeleteRequiresSession(t *testing.T) {
	h := NewAdmin(&fakeAdminStore{}, &fakeAdminLeads{}, nil, &fakeAdminDeliverer{},
		fakeAdminAuth{valid: "adminkey"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := adminReq(h, http.MethodPost, "/api/v1/admin/lead/delete", "adminkey", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 without session_id", rec.Code)
	}
}
