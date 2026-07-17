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

// Button is one inline-keyboard button; Data is the callback payload (≤64 bytes).
type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

// Keyboard is rows of inline buttons.
type Keyboard [][]Button

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

// SendMessage sends plain text to a chat.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	return c.api(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": text})
}

// SendMenu sends text with an inline keyboard.
func (c *Client) SendMenu(ctx context.Context, chatID int64, text string, kb Keyboard) error {
	return c.api(ctx, "sendMessage", map[string]any{
		"chat_id":      chatID,
		"text":         text,
		"reply_markup": map[string]any{"inline_keyboard": kb},
	})
}

// EditMenu replaces a previously sent menu message in place (smooth navigation).
func (c *Client) EditMenu(ctx context.Context, chatID, messageID int64, text string, kb Keyboard) error {
	return c.api(ctx, "editMessageText", map[string]any{
		"chat_id":      chatID,
		"message_id":   messageID,
		"text":         text,
		"reply_markup": map[string]any{"inline_keyboard": kb},
	})
}

// AnswerCallback acks a button tap so Telegram stops the loading spinner.
func (c *Client) AnswerCallback(ctx context.Context, callbackID string) error {
	return c.api(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": callbackID})
}

// api POSTs a JSON payload to a Bot API method.
func (c *Client) api(ctx context.Context, method string, payload map[string]any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshal %s: %w", method, err)
	}

	url := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("telegram: build %s: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: %s failed: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("telegram: %s status %d: %s", method, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}
