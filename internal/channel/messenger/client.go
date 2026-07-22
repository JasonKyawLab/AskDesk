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

// QuickReply is one tappable chip shown above the composer. Title ≤20 chars.
type QuickReply struct {
	Title   string
	Payload string
}

// CardButton is a button on a carousel card. Title ≤20 chars.
type CardButton struct {
	Title   string
	Payload string
}

// Card is one element of a generic-template carousel (≤10 per message).
type Card struct {
	Title    string // ≤80 chars
	Subtitle string // optional, ≤80 chars
	Buttons  []CardButton
}

// SendMessage sends plain text to a recipient identified by their page-scoped
// id (PSID). messaging_type RESPONSE marks it as a reply to a user message.
func (c *Client) SendMessage(ctx context.Context, recipientID, text string) error {
	return c.post(ctx, "me/messages", map[string]any{
		"recipient":      map[string]string{"id": recipientID},
		"messaging_type": "RESPONSE",
		"message":        map[string]string{"text": text},
	})
}

// SendQuickReplies sends text with tappable quick-reply chips (Messenger's
// equivalent of an inline keyboard). Titles are truncated to Messenger's limit.
func (c *Client) SendQuickReplies(ctx context.Context, recipientID, text string, replies []QuickReply) error {
	qrs := make([]map[string]any, 0, len(replies))
	for _, r := range replies {
		qrs = append(qrs, map[string]any{
			"content_type": "text",
			"title":        truncate(r.Title, 20),
			"payload":      r.Payload,
		})
	}
	return c.post(ctx, "me/messages", map[string]any{
		"recipient":      map[string]string{"id": recipientID},
		"messaging_type": "RESPONSE",
		"message":        map[string]any{"text": text, "quick_replies": qrs},
	})
}

// SendCarousel sends a generic-template carousel — one card per FAQ, each with
// a postback button. Facebook allows at most 10 cards; extras are dropped.
func (c *Client) SendCarousel(ctx context.Context, recipientID string, cards []Card) error {
	if len(cards) > 10 {
		cards = cards[:10]
	}
	elements := make([]map[string]any, 0, len(cards))
	for _, card := range cards {
		btns := make([]map[string]any, 0, len(card.Buttons))
		for _, b := range card.Buttons {
			btns = append(btns, map[string]any{
				"type":    "postback",
				"title":   truncate(b.Title, 20),
				"payload": b.Payload,
			})
		}
		el := map[string]any{"title": truncate(card.Title, 80), "buttons": btns}
		if card.Subtitle != "" {
			el["subtitle"] = truncate(card.Subtitle, 80)
		}
		elements = append(elements, el)
	}
	return c.post(ctx, "me/messages", map[string]any{
		"recipient":      map[string]string{"id": recipientID},
		"messaging_type": "RESPONSE",
		"message": map[string]any{
			"attachment": map[string]any{
				"type":    "template",
				"payload": map[string]any{"template_type": "generic", "elements": elements},
			},
		},
	})
}

// SetupProfile configures the Get Started button and persistent menu, so the
// bot presents a "Browse topics" entry point like the Telegram menu. Called
// best-effort at startup; getStartedPayload is delivered as a postback on tap.
func (c *Client) SetupProfile(ctx context.Context, getStartedPayload, browseTitle, askTitle, askPayload string) error {
	return c.post(ctx, "me/messenger_profile", map[string]any{
		"get_started": map[string]string{"payload": getStartedPayload},
		"persistent_menu": []map[string]any{{
			"locale":                  "default",
			"composer_input_disabled": false,
			"call_to_actions": []map[string]any{
				{"type": "postback", "title": truncate(browseTitle, 30), "payload": getStartedPayload},
				{"type": "postback", "title": truncate(askTitle, 30), "payload": askPayload},
			},
		}},
	})
}

// GetProfile fetches a user's display name by their page-scoped id (PSID).
// Messenger webhooks carry only the PSID, so this is how the inbox learns who
// sent a message. Returns "" (no error) when the name isn't available.
func (c *Client) GetProfile(ctx context.Context, psid string) (string, error) {
	endpoint := fmt.Sprintf("%s/%s?fields=first_name,last_name&access_token=%s",
		c.baseURL, url.PathEscape(psid), url.QueryEscape(c.pageToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("messenger: build profile: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("messenger: profile failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("messenger: profile status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var p struct {
		First string `json:"first_name"`
		Last  string `json:"last_name"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return "", fmt.Errorf("messenger: decode profile: %w", err)
	}
	return strings.TrimSpace(p.First + " " + p.Last), nil
}

// post sends a JSON payload to a Graph API path under the page token.
func (c *Client) post(ctx context.Context, path string, payload map[string]any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("messenger: marshal %s: %w", path, err)
	}

	endpoint := fmt.Sprintf("%s/%s?access_token=%s", c.baseURL, path, url.QueryEscape(c.pageToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("messenger: build %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("messenger: %s failed: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("messenger: %s status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

// truncate caps a UI string to n runes, appending an ellipsis when it cuts.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
