package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

func TestGemini_GenerateReply(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":generateContent") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"  Yes, we deliver.  "}]}}]}`))
	}))
	defer srv.Close()

	g := NewGemini("test-key", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	ans, err := g.GenerateReply(context.Background(), "do you deliver?",
		[]core.Match{{Question: "delivery", Answer: "1-2 days"}})
	if err != nil {
		t.Fatalf("GenerateReply: %v", err)
	}
	if ans != "Yes, we deliver." {
		t.Errorf("answer = %q, want trimmed %q", ans, "Yes, we deliver.")
	}
	if !strings.Contains(gotBody, "do you deliver?") {
		t.Errorf("request body should contain the question; got %s", gotBody)
	}
	if !strings.Contains(gotBody, "1-2 days") {
		t.Errorf("request body should contain the FAQ context")
	}
}

func TestGemini_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":embedContent") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embedding": map[string]any{"values": []float64{0.1, 0.2, 0.3}},
		})
	}))
	defer srv.Close()

	g := NewGemini("test-key", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	vec, err := g.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	want := []float32{0.1, 0.2, 0.3}
	if len(vec) != len(want) {
		t.Fatalf("len = %d, want %d", len(vec), len(want))
	}
	for i := range want {
		if vec[i] != want[i] {
			t.Errorf("vec[%d] = %f, want %f", i, vec[i], want[i])
		}
	}
}

func TestGemini_ErrorStatusFailsOver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"rate limit"}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	g := NewGemini("test-key", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if _, err := g.GenerateReply(context.Background(), "hi", nil); err == nil {
		t.Fatal("expected error on 429 status")
	}
}
