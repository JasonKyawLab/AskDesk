package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer() *Server {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
}

func TestHealthEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantBody string
	}{
		{name: "liveness", path: "/healthz", wantBody: `{"status":"ok"}`},
		{name: "readiness", path: "/readyz", wantBody: `{"status":"ready"}`},
	}

	handler := newTestServer().Routes()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			got := strings.TrimSpace(rec.Body.String())
			if got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}
