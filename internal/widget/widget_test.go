package widget

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeScript(t *testing.T) {
	rec := httptest.NewRecorder()
	New().ServeScript(rec, httptest.NewRequest(http.MethodGet, "/widget.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("content-type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "AskDesk") || !strings.Contains(body, "/api/v1/config") {
		t.Errorf("widget.js missing expected content")
	}
}

func TestServeDemo(t *testing.T) {
	rec := httptest.NewRecorder()
	New().ServeDemo(rec, httptest.NewRequest(http.MethodGet, "/widget/demo", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "widget.js") {
		t.Errorf("demo page not served correctly (status %d)", rec.Code)
	}
}
