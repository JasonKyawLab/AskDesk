package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/JasonKyawLab/AskDesk/internal/auth"
	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

// AdminStore is the data access the admin panel needs (satisfied by store.Admins).
type AdminStore interface {
	IsAdmin(ctx context.Context, businessID int64, channel core.Channel, externalID string) (bool, error)
	TodayStats(ctx context.Context, businessID int64) (store.DailyStats, error)
	PendingUnanswered(ctx context.Context, businessID int64, limit int) ([]store.PendingQuestion, error)
	GetUnanswered(ctx context.Context, businessID, id int64) (store.UnansweredTarget, error)
	ResolveUnanswered(ctx context.Context, businessID, id int64) error
	SetPendingReply(ctx context.Context, businessID int64, channel core.Channel, externalID string, queueID int64) error
	ClearPendingReply(ctx context.Context, businessID int64, channel core.Channel, externalID string) error
	PendingReply(ctx context.Context, businessID int64, channel core.Channel, externalID string) (int64, bool, error)
}

// PanelClient is the Telegram surface the panel renders onto.
type PanelClient interface {
	MenuClient
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// Deliverer sends an admin's reply to the customer on their originating channel
// (Telegram chat or web poll). It makes tap-to-reply work across channels.
type Deliverer interface {
	Deliver(ctx context.Context, channel core.Channel, replyTo, text string) error
}

// AdminPanel is the button-driven admin UI: /admin opens it; stats, pending
// questions, tap-to-reply, dismiss, and the FAQ editor link are all buttons.
// Every action re-verifies the tapper's admin identity.
type AdminPanel struct {
	store      AdminStore
	client     PanelClient
	deliverer  Deliverer
	signer     *auth.Signer // nil hides the Edit FAQs button
	baseURL    string
	businessID int64
	log        *slog.Logger
}

// NewAdminPanel constructs the panel. signer/baseURL may be empty.
func NewAdminPanel(s AdminStore, client PanelClient, deliverer Deliverer, signer *auth.Signer, baseURL string, businessID int64, log *slog.Logger) *AdminPanel {
	return &AdminPanel{store: s, client: client, deliverer: deliverer, signer: signer, baseURL: baseURL, businessID: businessID, log: log}
}

// Admin callback data: "a" panel, "a:s" stats, "a:p" pending list,
// "a:q:<id>" detail, "a:r:<id>" start reply, "a:d:<id>" dismiss, "a:c" cancel.
const (
	cbPanel   = "a"
	cbStats   = "a:s"
	cbPending = "a:p"
	cbCancel  = "a:c"
)

func isPanelData(data string) bool {
	return data == cbPanel || strings.HasPrefix(data, "a:")
}

// ShowPanel handles the typed /admin command. Returns false for non-admins so
// the message falls through to the normal flow.
func (p *AdminPanel) ShowPanel(ctx context.Context, fromID, chatID int64) bool {
	if !p.isAdmin(ctx, fromID) {
		return false
	}
	text, kb := p.panelScreen()
	if err := p.client.SendMenu(ctx, chatID, text, kb); err != nil {
		p.log.Error("admin panel: send failed", "error", err)
	}
	return true
}

// HandleCallback routes an admin button tap. Returns false when the data is not
// panel data (so the customer menu can handle it).
func (p *AdminPanel) HandleCallback(ctx context.Context, cb *callbackQuery) bool {
	if !isPanelData(cb.Data) {
		return false
	}
	defer func() { _ = p.client.AnswerCallback(ctx, cb.ID) }()

	// Buttons can be forged; never act without re-verifying identity.
	if cb.Message == nil || !p.isAdmin(ctx, cb.From.ID) {
		return true
	}
	adminID := strconv.FormatInt(cb.From.ID, 10)

	var (
		text string
		kb   Keyboard
		err  error
	)
	switch data := cb.Data; {
	case data == cbStats:
		text, kb, err = p.statsScreen(ctx)
	case data == cbPending:
		text, kb, err = p.pendingScreen(ctx)
	case data == cbCancel:
		err = p.store.ClearPendingReply(ctx, p.businessID, core.ChannelTelegram, adminID)
		text, kb = p.panelScreen()
	case data == "a:e":
		text, kb = p.editScreen(adminID)
	case strings.HasPrefix(data, "a:q:"):
		text, kb, err = p.detailScreen(ctx, strings.TrimPrefix(data, "a:q:"))
	case strings.HasPrefix(data, "a:r:"):
		text, kb, err = p.startReply(ctx, adminID, strings.TrimPrefix(data, "a:r:"))
	case strings.HasPrefix(data, "a:d:"):
		text, kb, err = p.dismiss(ctx, strings.TrimPrefix(data, "a:d:"))
	default: // "a"
		text, kb = p.panelScreen()
	}
	if err != nil {
		p.log.Error("admin panel: screen failed", "error", err, "data", cb.Data)
		return true
	}

	if err := p.client.EditMenu(ctx, cb.Message.Chat.ID, cb.Message.MessageID, text, kb); err != nil {
		p.log.Error("admin panel: edit failed", "error", err)
	}
	return true
}

// InterceptReply consumes a plain admin message while a tap-to-reply is in
// progress: it relays the text to the customer, resolves the item, and clears
// the state. Returns false when there is nothing to intercept.
func (p *AdminPanel) InterceptReply(ctx context.Context, fromID, chatID int64, text string) bool {
	adminID := strconv.FormatInt(fromID, 10)
	queueID, ok, err := p.store.PendingReply(ctx, p.businessID, core.ChannelTelegram, adminID)
	if err != nil || !ok {
		return false
	}

	target, err := p.store.GetUnanswered(ctx, p.businessID, queueID)
	if err != nil {
		// Item vanished (answered elsewhere): clear state and inform.
		_ = p.store.ClearPendingReply(ctx, p.businessID, core.ChannelTelegram, adminID)
		p.send(ctx, chatID, fmt.Sprintf("Question #%d is no longer pending — reply cancelled.", queueID))
		return true
	}

	// Route to the customer's own channel (Telegram chat or web poll).
	if err := p.deliverer.Deliver(ctx, target.Channel, target.ReplyTo, text); err != nil {
		p.log.Error("admin panel: deliver reply failed", "error", err)
		p.send(ctx, chatID, "Couldn't deliver the reply — please try again.")
		return true
	}

	_ = p.store.ResolveUnanswered(ctx, p.businessID, queueID)
	_ = p.store.ClearPendingReply(ctx, p.businessID, core.ChannelTelegram, adminID)

	name := target.UserName
	if name == "" {
		name = "the customer"
	}
	p.send(ctx, chatID, fmt.Sprintf("✅ Sent to %s. #%d resolved.", name, queueID))
	return true
}

// --- screens ---

func (p *AdminPanel) panelScreen() (string, Keyboard) {
	kb := Keyboard{
		{{Text: "📊 Today's stats", Data: cbStats}},
		{{Text: "📥 Pending questions", Data: cbPending}},
	}
	if p.signer != nil && p.baseURL != "" {
		kb = append(kb, []Button{{Text: "✏️ Edit FAQs", Data: "a:e"}})
	}
	return "🛠 Admin panel — what would you like to do?", kb
}

func (p *AdminPanel) statsScreen(ctx context.Context) (string, Keyboard, error) {
	st, err := p.store.TodayStats(ctx, p.businessID)
	if err != nil {
		return "", nil, err
	}
	text := fmt.Sprintf("📊 Today so far:\n• Total: %d\n• Answered: %d\n• Unanswered: %d",
		st.Total, st.Answered, st.Unanswered)
	return text, backRow(), nil
}

func (p *AdminPanel) pendingScreen(ctx context.Context) (string, Keyboard, error) {
	items, err := p.store.PendingUnanswered(ctx, p.businessID, 10)
	if err != nil {
		return "", nil, err
	}
	if len(items) == 0 {
		return "📥 No pending questions. 🎉", backRow(), nil
	}

	var kb Keyboard
	for _, it := range items {
		name := it.UserName
		if name == "" {
			name = "customer"
		}
		label := truncLabel(fmt.Sprintf("#%d %s · %s: %s", it.ID, name, it.Ago(), it.Question))
		kb = append(kb, []Button{{Text: label, Data: "a:q:" + strconv.FormatInt(it.ID, 10)}})
	}
	kb = append(kb, backRow()...)
	return "📥 Pending questions — tap one to answer:", kb, nil
}

func (p *AdminPanel) detailScreen(ctx context.Context, idStr string) (string, Keyboard, error) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return "", nil, err
	}
	t, err := p.store.GetUnanswered(ctx, p.businessID, id)
	if err != nil {
		text := fmt.Sprintf("Question #%d is no longer pending.", id)
		return text, Keyboard{{{Text: "⬅️ Pending", Data: cbPending}}}, nil
	}

	name := t.UserName
	if name == "" {
		name = "customer"
	}
	text := fmt.Sprintf("📨 #%d — from %s · %s:\n\n%s", id, name, t.Ago(), t.Question)
	kb := Keyboard{
		{{Text: "✍️ Reply", Data: "a:r:" + idStr}, {Text: "🗑 Dismiss", Data: "a:d:" + idStr}},
		{{Text: "⬅️ Pending", Data: cbPending}},
	}
	return text, kb, nil
}

func (p *AdminPanel) startReply(ctx context.Context, adminID, idStr string) (string, Keyboard, error) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return "", nil, err
	}
	if err := p.store.SetPendingReply(ctx, p.businessID, core.ChannelTelegram, adminID, id); err != nil {
		return "", nil, err
	}
	text := fmt.Sprintf("✍️ Type your reply to #%d now — your next message will be sent straight to the customer.", id)
	return text, Keyboard{{{Text: "❌ Cancel", Data: cbCancel}}}, nil
}

func (p *AdminPanel) dismiss(ctx context.Context, idStr string) (string, Keyboard, error) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return "", nil, err
	}
	if err := p.store.ResolveUnanswered(ctx, p.businessID, id); err != nil {
		return "", nil, err
	}
	text, kb, err := p.pendingScreen(ctx)
	if err != nil {
		return "", nil, err
	}
	return "🗑 Dismissed.\n\n" + text, kb, nil
}

func (p *AdminPanel) editScreen(adminID string) (string, Keyboard) {
	link, err := p.signer.MagicLink(p.baseURL, auth.Claims{
		BusinessID: p.businessID,
		AdminID:    adminID,
		Channel:    string(core.ChannelTelegram),
		ExpiresAt:  time.Now().Add(10 * time.Minute).Unix(),
	})
	if err != nil {
		p.log.Error("admin panel: magic link failed", "error", err)
		return "Couldn't create an editor link — please try again.", backRow()
	}
	return "✏️ Your FAQ editor (expires in 10 min):\n" + link, backRow()
}

func backRow() Keyboard {
	return Keyboard{{{Text: "⬅️ Admin panel", Data: cbPanel}}}
}

func (p *AdminPanel) isAdmin(ctx context.Context, fromID int64) bool {
	ok, err := p.store.IsAdmin(ctx, p.businessID, core.ChannelTelegram, strconv.FormatInt(fromID, 10))
	if err != nil {
		p.log.Error("admin panel: identity check failed", "error", err)
		return false
	}
	return ok
}

func (p *AdminPanel) send(ctx context.Context, chatID int64, text string) {
	if err := p.client.SendMessage(ctx, chatID, text); err != nil {
		p.log.Error("admin panel: send failed", "error", err)
	}
}
