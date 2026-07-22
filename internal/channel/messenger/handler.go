package messenger

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// Submitter accepts a normalized message plus the channel reply address and
// hands it off (inline dispatch or the task queue). Structurally identical to
// the Telegram submitter, so the same all-in-one/queue submitter satisfies both.
type Submitter interface {
	Submit(ctx context.Context, msg core.Message, replyTo string) error
}

// Handler is the Messenger webhook endpoint. GET requests are Facebook's
// subscription verification handshake; POST requests carry messaging events,
// which are signature-verified, normalized, and submitted to the engine.
type Handler struct {
	submitter   Submitter
	businessID  int64
	appSecret   string // verifies X-Hub-Signature-256; empty disables verification (dev only)
	verifyToken string // echoed challenge token for the GET handshake
	log         *slog.Logger
}

// NewHandler builds the webhook handler. An empty appSecret disables signature
// verification (development only).
func NewHandler(submitter Submitter, businessID int64, appSecret, verifyToken string, log *slog.Logger) *Handler {
	return &Handler{
		submitter:   submitter,
		businessID:  businessID,
		appSecret:   appSecret,
		verifyToken: verifyToken,
		log:         log,
	}
}

// webhookEvent is the subset of a Messenger webhook payload we care about.
type webhookEvent struct {
	Object string `json:"object"`
	Entry  []struct {
		Messaging []struct {
			Sender struct {
				ID string `json:"id"`
			} `json:"sender"`
			Message *struct {
				Text   string `json:"text"`
				IsEcho bool   `json:"is_echo"`
			} `json:"message"`
		} `json:"messaging"`
	} `json:"entry"`
}

// ServeHTTP routes the GET verification handshake and POST messaging events.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.verify(w, r)
	case http.MethodPost:
		h.receive(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// verify answers Facebook's subscription handshake: echo hub.challenge when the
// mode and verify token match what we configured.
func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("hub.mode") == "subscribe" && q.Get("hub.verify_token") == h.verifyToken && h.verifyToken != "" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(q.Get("hub.challenge")))
		return
	}
	h.log.Warn("messenger webhook: verification failed")
	w.WriteHeader(http.StatusForbidden)
}

// receive verifies the signature, normalizes each text message, and submits it.
// It always acks 200 after auth so Facebook does not retry-storm on a transient
// engine error.
func (h *Handler) receive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !h.validSignature(r, body) {
		h.log.Warn("messenger webhook: invalid signature")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var ev webhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		h.log.Warn("messenger webhook: bad payload", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, entry := range ev.Entry {
		for _, m := range entry.Messaging {
			// Skip non-text events and our own echoed messages.
			if m.Message == nil || m.Message.IsEcho || m.Message.Text == "" || m.Sender.ID == "" {
				continue
			}
			msg := core.Message{
				BusinessID: h.businessID,
				Channel:    core.ChannelMessenger,
				UserID:     m.Sender.ID,
				Text:       m.Message.Text,
			}
			if err := h.submitter.Submit(r.Context(), msg, m.Sender.ID); err != nil {
				h.log.Error("messenger webhook: submit failed", "error", err, "business_id", h.businessID)
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

// validSignature checks the X-Hub-Signature-256 HMAC over the raw body.
func (h *Handler) validSignature(r *http.Request, body []byte) bool {
	if h.appSecret == "" {
		return true
	}
	got := r.Header.Get("X-Hub-Signature-256")
	const prefix = "sha256="
	if len(got) <= len(prefix) {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.appSecret))
	mac.Write(body)
	want := prefix + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(got), []byte(want))
}
