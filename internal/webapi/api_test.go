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

type fakeBiz struct{ valid string }

func (f fakeBiz) IDByAPIKey(_ context.Context, key string) (int64, error) {
	if key == f.valid {
		return 1, nil
	}
	return 0, store.ErrUnknownAPIKey
}
func (fakeBiz) Settings(context.Context, int64) (store.BusinessSettings, error) {
	return store.BusinessSettings{DisplayName: "MiniPOS", WelcomeMessage: "hi", AskPrompt: "ask"}, nil
}

func newAPI(engine Engine, faqs FAQStore) *Handler {
	return New(engine, faqs, fakeBiz{valid: "goodkey"}, []string{"*"},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestAPI_AskEmptyMessage(t *testing.T) {
	rec := do(newAPI(fakeEngine{}, fakeFAQs{}), http.MethodPost, "/api/v1/ask", "goodkey", `{"message":"  "}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
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
