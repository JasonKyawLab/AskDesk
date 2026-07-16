// Package telegram is the Telegram channel adapter: it translates Telegram
// webhook updates into normalized core.Messages and sends replies back. The
// core engine never knows anything Telegram-specific.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client sends messages via the Telegram Bot API. Base URL and HTTP client are
// injectable for testing.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides the API base URL (used in tests).
func WithBaseURL(u string) ClientOption { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient overrides the HTTP client (used in tests).
func WithHTTPClient(h *http.Client) ClientOption { return func(c *Client) { c.httpClient = h } }

// NewClient builds a Telegram API client for the given bot token.
func NewClient(token string, opts ...ClientOption) *Client {
	c := &Client{
		token:      token,
		baseURL:    "https://api.telegram.org",
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// SendMessage sends text to a chat.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	payload, err := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	if err != nil {
		return fmt.Errorf("telegram: marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", c.baseURL, c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("telegram: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram: status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}
