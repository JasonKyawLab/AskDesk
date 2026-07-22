// Package messenger is the Facebook Messenger channel adapter: it translates
// Messenger webhook events into normalized core.Messages and sends replies back
// via the Send API. The core engine never knows anything Messenger-specific.
package messenger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client sends messages via the Messenger Send API. Base URL and HTTP client
// are injectable for testing.
type Client struct {
	pageToken  string
	baseURL    string
	httpClient *http.Client
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides the Graph API base URL (used in tests).
func WithBaseURL(u string) ClientOption { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient overrides the HTTP client (used in tests).
func WithHTTPClient(h *http.Client) ClientOption { return func(c *Client) { c.httpClient = h } }

// NewClient builds a Messenger Send API client for the given page access token.
func NewClient(pageToken string, opts ...ClientOption) *Client {
	c := &Client{
		pageToken:  pageToken,
		baseURL:    "https://graph.facebook.com/v21.0",
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// SendMessage sends plain text to a recipient identified by their page-scoped
// id (PSID). messaging_type RESPONSE marks it as a reply to a user message.
func (c *Client) SendMessage(ctx context.Context, recipientID, text string) error {
	payload := map[string]any{
		"recipient":      map[string]string{"id": recipientID},
		"messaging_type": "RESPONSE",
		"message":        map[string]string{"text": text},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("messenger: marshal send: %w", err)
	}

	endpoint := fmt.Sprintf("%s/me/messages?access_token=%s", c.baseURL, url.QueryEscape(c.pageToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("messenger: build send: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("messenger: send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("messenger: send status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}
