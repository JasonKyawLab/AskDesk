package webapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

type fakeEngine struct{ reply core.Reply }

func (f fakeEngine) GenerateCustomerReply(_ context.Context, _ core.Message) (core.Reply, error) {
	return f.reply, nil
}

type fakeFAQs struct {
	cats []string
	list []store.FAQ
}

func (f fakeFAQs) Categories(context.Context, int64) ([]string, error) { return f.cats, nil }
func (f fakeFAQs) List(context.Context, int64) ([]store.FAQ, error)    { return f.list, nil }

type fakeBiz struct {
	valid        string
	rate, global int // 0 = unlimited
}

func (f fakeBiz) IDByAPIKey(_ context.Context, key string) (int64, error) {
	if key == f.valid {
		return 1, nil
	}
	return 0, store.ErrUnknownAPIKey
}
func (f fakeBiz) Settings(context.Context, int64) (store.BusinessSettings, error) {
	return store.BusinessSettings{
		DisplayName: "MiniPOS", WelcomeMessage: "hi", AskPrompt: "ask", FallbackMessage: "busy",
		AskRatePerMin: f.rate, AskGlobalPerMin: f.global,
	}, nil
}

type fakeReplies struct{ list []store.WebReply }

func (f fakeReplies) Since(context.Context, int64, string, int64) ([]store.WebReply, error) {
	return f.list, nil
}

type fakeLeads struct {
	businessID            int64
	session, email, phone string
	called                bool
}

func (f *fakeLeads) Upsert(_ context.Context, businessID int64, session, name, email, phone string) error {
	f.called = true
	f.businessID, f.session, f.email, f.phone = businessID, session, email, phone
	return nil
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newAPI(engine Engine, faqs FAQStore) *Handler {
	return New(engine, faqs, fakeBiz{valid: "goodkey"}, fakeReplies{}, &fakeLeads{},
		Options{AllowedOrigins: []string{"*"}}, discardLog())
}

func do(h *Handler, method, path, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAPI_RejectsBadKey(t *testing.T) {
	h := newAPI(fakeEngine{}, fakeFAQs{})
	rec := do(h, http.MethodGet, "/api/v1/faqs", "wrong", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPI_FAQsGrouped(t *testing.T) {
	faqs := fakeFAQs{
		cats: []string{"Getting Started", "Pricing"},
		list: []store.FAQ{
			{ID: 1, Question: "What is it?", Answer: "A POS.", Category: "Getting Started"},
			{ID: 2, Question: "Cost?", Answer: "Free.", Category: "Pricing"},
		},
	}
	rec := do(newAPI(fakeEngine{}, faqs), http.MethodGet, "/api/v1/faqs", "goodkey", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got struct {
		Categories []struct {
			Name string
			FAQs []struct{ Question string }
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 2 || got.Categories[0].Name != "Getting Started" || got.Categories[0].FAQs[0].Question != "What is it?" {
		t.Errorf("unexpected grouping: %+v", got.Categories)
	}
}

func TestAPI_EmptyFAQsIsEmptyNotError(t *testing.T) {
	rec := do(newAPI(fakeEngine{}, fakeFAQs{}), http.MethodGet, "/api/v1/faqs", "goodkey", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 on empty", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"categories":[]`) {
		t.Errorf("empty faqs should be [], got %s", rec.Body.String())
	}
}

func TestAPI_ConfigEmptyCategories(t *testing.T) {
	rec := do(newAPI(fakeEngine{}, fakeFAQs{}), http.MethodGet, "/api/v1/config", "goodkey", "")
	if !strings.Contains(rec.Body.String(), `"categories":[]`) {
		t.Errorf("config categories should be [], got %s", rec.Body.String())
	}
}

func TestAPI_Ask(t *testing.T) {
	h := newAPI(fakeEngine{reply: core.Reply{Text: "Yes, we deliver.", Answered: true}}, fakeFAQs{})
	rec := do(h, http.MethodPost, "/api/v1/ask", "goodkey", `{"message":"do you deliver?","session_id":"s1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got askResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Answer != "Yes, we deliver." || !got.Answered {
		t.Errorf("unexpected ask response: %+v", got)
	}
}

func TestAPI_AskRateLimited(t *testing.T) {
	h := New(fakeEngine{reply: core.Reply{Text: "ok", Answered: true}}, fakeFAQs{},
		fakeBiz{valid: "goodkey", rate: 2}, fakeReplies{}, &fakeLeads{},
		Options{AllowedOrigins: []string{"*"}}, discardLog())
	body := `{"message":"q","session_id":"s"}`

	for i := 0; i < 2; i++ {
		if rec := do(h, http.MethodPost, "/api/v1/ask", "goodkey", body); rec.Code != http.StatusOK {
			t.Fatalf("call %d = %d, want 200", i+1, rec.Code)
		}
	}
	rec := do(h, http.MethodPost, "/api/v1/ask", "goodkey", body)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("3rd call = %d, want 429", rec.Code)
	}
}

func TestAPI_AskEmptyMessage(t *testing.T) {
	rec := do(newAPI(fakeEngine{}, fakeFAQs{}), http.MethodPost, "/api/v1/ask", "goodkey", `{"message":"  "}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAPI_Replies(t *testing.T) {
	h := New(fakeEngine{}, fakeFAQs{}, fakeBiz{valid: "goodkey"},
		fakeReplies{list: []store.WebReply{{ID: 3, Message: "Yes, we deliver."}}},
		&fakeLeads{}, Options{AllowedOrigins: []string{"*"}}, discardLog())

	rec := do(h, http.MethodGet, "/api/v1/replies?session_id=s1&since=0", "goodkey", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Yes, we deliver.") {
		t.Errorf("expected reply in body, got %s", rec.Body.String())
	}
}

func TestAPI_RepliesRequiresSession(t *testing.T) {
	rec := do(newAPI(fakeEngine{}, fakeFAQs{}), http.MethodGet, "/api/v1/replies", "goodkey", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 without session_id", rec.Code)
	}
}

func TestAPI_Lead(t *testing.T) {
	leads := &fakeLeads{}
	h := New(fakeEngine{}, fakeFAQs{}, fakeBiz{valid: "goodkey"}, fakeReplies{}, leads,
		Options{AllowedOrigins: []string{"*"}}, discardLog())

	rec := do(h, http.MethodPost, "/api/v1/lead", "goodkey", `{"session_id":"s1","email":"a@b.com"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !leads.called || leads.email != "a@b.com" || leads.session != "s1" {
		t.Errorf("lead not saved correctly: %+v", leads)
	}
}

func TestAPI_LeadRequiresContact(t *testing.T) {
	rec := do(newAPI(fakeEngine{}, fakeFAQs{}), http.MethodPost, "/api/v1/lead", "goodkey", `{"session_id":"s1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 without email/phone", rec.Code)
	}
}

func TestAPI_ConfigExposesContactAndSource(t *testing.T) {
	h := New(fakeEngine{}, fakeFAQs{}, fakeBiz{valid: "goodkey"}, fakeReplies{}, &fakeLeads{},
		Options{AllowedOrigins: []string{"*"}, SourceURL: "https://example.com/src", RequireContact: true}, discardLog())
	rec := do(h, http.MethodGet, "/api/v1/config", "goodkey", "")
	body := rec.Body.String()
	if !strings.Contains(body, `"require_contact":true`) {
		t.Errorf("config should expose require_contact: %s", body)
	}
	if !strings.Contains(body, `"source_url":"https://example.com/src"`) {
		t.Errorf("config should expose source_url: %s", body)
	}
}

func TestAPI_CORSPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/faqs", nil)
	req.Header.Set("Origin", "https://minipos.site")
	rec := httptest.NewRecorder()
	newAPI(fakeEngine{}, fakeFAQs{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://minipos.site" {
		t.Errorf("missing CORS origin header: %v", rec.Header())
	}
}
