package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

// Submitter accepts a normalized message plus the channel reply address and
// hands it off (e.g. to the task queue). The web tier stays thin: it validates,
// normalizes, and submits — the worker does the slow engine work.
type Submitter interface {
	Submit(ctx context.Context, msg core.Message, replyTo string) error
}

// Handler is the Telegram webhook endpoint. In Phase 1 it serves a single
// business; multi-tenant routing (by bot token) plugs in here later.
//
// Button-menu taps and greetings are handled here directly (fast DB reads, no
// AI); free-typed questions flow through the Submitter to the engine.
type Handler struct {
	submitter  Submitter
	menu       MenuStore     // nil disables the button menu
	menuClient MenuClient    // nil disables the button menu
	panel      *AdminPanel   // nil disables the admin panel
	settings   SettingsStore // nil uses built-in default copy
	businessID int64
	secret     string
	log        *slog.Logger
}

// SettingsStore provides per-business presentation strings (welcome, ask prompt).
type SettingsStore interface {
	Settings(ctx context.Context, businessID int64) (store.BusinessSettings, error)
}

// NewHandler builds the webhook handler. secret is the Telegram webhook secret
// token; an empty secret disables verification (development only). menu,
// menuClient, panel, and settings may be nil, which disables those features.
func NewHandler(submitter Submitter, menu MenuStore, menuClient MenuClient, panel *AdminPanel, settings SettingsStore, businessID int64, secret string, log *slog.Logger) *Handler {
	return &Handler{
		submitter:  submitter,
		menu:       menu,
		menuClient: menuClient,
		panel:      panel,
		settings:   settings,
		businessID: businessID,
		secret:     secret,
		log:        log,
	}
}

// callbackQuery is a button tap on an inline keyboard.
type callbackQuery struct {
	ID   string `json:"id"`
	Data string `json:"data"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Message *struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
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
	CallbackQuery *callbackQuery `json:"callback_query"`
}

// ServeHTTP verifies the secret, then routes: button taps and greetings to the
// menu, text to the submitter. It acks 200 after auth so Telegram does not
// retry-storm on a transient error.
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

	// Button tap: handled inline (DB reads only, no AI). Admin-panel data is
	// routed first; everything else is the customer menu.
	if upd.CallbackQuery != nil {
		handled := h.panel != nil && h.panel.HandleCallback(r.Context(), upd.CallbackQuery)
		if !handled && h.menusEnabled() {
			h.handleCallback(r.Context(), upd.CallbackQuery)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	// Ignore non-text updates (joins, stickers, edits, ...).
	if upd.Message == nil || upd.Message.Text == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	text := upd.Message.Text

	// An admin mid tap-to-reply: their plain message goes to the customer.
	if h.panel != nil && !strings.HasPrefix(strings.TrimSpace(text), "/") {
		if h.panel.InterceptReply(r.Context(), upd.Message.From.ID, upd.Message.Chat.ID, text) {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// /admin opens the button panel (admins only; others fall through).
	if h.panel != nil && strings.TrimSpace(strings.ToLower(text)) == "/admin" {
		if h.panel.ShowPanel(r.Context(), upd.Message.From.ID, upd.Message.Chat.ID) {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Greetings open the menu instead of spending an AI call.
	if h.menusEnabled() && isGreeting(text) {
		h.showMainMenu(r.Context(), upd.Message.Chat.ID, upd.Message.From.ID)
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
