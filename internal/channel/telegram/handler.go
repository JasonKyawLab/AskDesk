package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// Submitter accepts a normalized message plus the channel reply address and
// hands it off (e.g. to the task queue). The web tier stays thin: it validates,
// normalizes, and submits — the worker does the slow engine work.
type Submitter interface {
	Submit(ctx context.Context, msg core.Message, replyTo string) error
}

// Handler is the Telegram webhook endpoint. In Phase 1 it serves a single
// business; multi-tenant routing (by bot token) plugs in here later.
type Handler struct {
	submitter  Submitter
	businessID int64
	secret     string
	log        *slog.Logger
}

// NewHandler builds the webhook handler. secret is the Telegram webhook secret
// token; an empty secret disables verification (development only).
func NewHandler(submitter Submitter, businessID int64, secret string, log *slog.Logger) *Handler {
	return &Handler{submitter: submitter, businessID: businessID, secret: secret, log: log}
}

// update is the subset of a Telegram update we care about.
type update struct {
	Message *struct {
		Text string `json:"text"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"from"`
	} `json:"message"`
}

// ServeHTTP verifies the secret, normalizes the update, and submits it. It acks
// 200 after auth so Telegram does not retry-storm on a transient error.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.validSecret(r) {
		h.log.Warn("telegram webhook: invalid secret token")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var upd update
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		h.log.Warn("telegram webhook: bad payload", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Ignore non-text updates (joins, stickers, edits, ...).
	if upd.Message == nil || upd.Message.Text == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	msg := core.Message{
		BusinessID: h.businessID,
		Channel:    core.ChannelTelegram,
		UserID:     strconv.FormatInt(upd.Message.From.ID, 10),
		UserName:   displayName(upd.Message.From.FirstName, upd.Message.From.Username),
		Text:       upd.Message.Text,
	}
	replyTo := strconv.FormatInt(upd.Message.Chat.ID, 10)

	if err := h.submitter.Submit(r.Context(), msg, replyTo); err != nil {
		h.log.Error("telegram webhook: submit failed", "error", err, "business_id", h.businessID)
	}
	w.WriteHeader(http.StatusOK)
}

// displayName builds a readable customer name from Telegram's first name and
// username, e.g. "Aung (@aungshop)".
func displayName(firstName, username string) string {
	switch {
	case firstName != "" && username != "":
		return firstName + " (@" + username + ")"
	case firstName != "":
		return firstName
	case username != "":
		return "@" + username
	default:
		return ""
	}
}

// validSecret compares the request's secret-token header in constant time.
func (h *Handler) validSecret(r *http.Request) bool {
	if h.secret == "" {
		return true
	}
	got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.secret)) == 1
}
