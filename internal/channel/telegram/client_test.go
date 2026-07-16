package telegram

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_SendMessage(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient("BOT123", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err := c.SendMessage(context.Background(), 42, "hello"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if gotPath != "/botBOT123/sendMessage" {
		t.Errorf("path = %q, want /botBOT123/sendMessage", gotPath)
	}
	if !strings.Contains(gotBody, `"chat_id":42`) || !strings.Contains(gotBody, `"text":"hello"`) {
		t.Errorf("body = %s, want chat_id and text", gotBody)
	}
}

func TestClient_SendMessage_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"ok":false}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewClient("BOT123", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err := c.SendMessage(context.Background(), 42, "hello"); err == nil {
		t.Fatal("expected error on non-200 status")
	}
}
