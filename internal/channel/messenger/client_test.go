package messenger

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_SendMessage(t *testing.T) {
	var gotPath, gotToken string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.URL.Query().Get("access_token")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"recipient_id":"123","message_id":"m1"}`))
	}))
	defer srv.Close()

	c := NewClient("PAGE_TOKEN", WithBaseURL(srv.URL))
	if err := c.SendMessage(context.Background(), "123", "Yes, we deliver."); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if gotPath != "/me/messages" {
		t.Errorf("path = %q, want /me/messages", gotPath)
	}
	if gotToken != "PAGE_TOKEN" {
		t.Errorf("access_token = %q, want PAGE_TOKEN", gotToken)
	}
	recipient, _ := gotBody["recipient"].(map[string]any)
	if recipient["id"] != "123" {
		t.Errorf("recipient.id = %v, want 123", recipient["id"])
	}
	msg, _ := gotBody["message"].(map[string]any)
	if msg["text"] != "Yes, we deliver." {
		t.Errorf("message.text = %v", msg["text"])
	}
}

func TestClient_SendMessage_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid OAuth token"}}`))
	}))
	defer srv.Close()

	c := NewClient("bad", WithBaseURL(srv.URL))
	if err := c.SendMessage(context.Background(), "123", "hi"); err == nil {
		t.Fatal("expected error on non-200 response")
	}
}
