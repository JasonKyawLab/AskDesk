package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/JasonKyawLab/AskDesk/internal/store"
)

// MenuStore is the FAQ data the button menu is built from. The menu is fully
// data-driven: categories become the main menu, questions become buttons.
type MenuStore interface {
	Categories(ctx context.Context, businessID int64) ([]string, error)
	ListByCategory(ctx context.Context, businessID int64, category string) ([]store.FAQ, error)
	GetByID(ctx context.Context, businessID, id int64) (store.FAQ, error)
}

// MenuClient is the Telegram surface the menu renders onto.
type MenuClient interface {
	SendMenu(ctx context.Context, chatID int64, text string, kb Keyboard) error
	EditMenu(ctx context.Context, chatID, messageID int64, text string, kb Keyboard) error
	AnswerCallback(ctx context.Context, callbackID string) error
}

// Callback data (≤64 bytes): "m" main menu, "c:<category>" category,
// "q:<faqID>" answer, "ask" free-text prompt.
const (
	cbMain = "m"
	cbAsk  = "ask"
)

const welcomeText = "👋 Welcome! Pick a topic below, or just type your question."

// isGreeting reports whether text should open the menu instead of the AI.
func isGreeting(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "/start", "/menu", "start", "menu", "hi", "hello", "hey":
		return true
	}
	return false
}

func (h *Handler) menusEnabled() bool {
	return h.menu != nil && h.menuClient != nil
}

// showMainMenu sends a fresh menu message (used for greetings).
func (h *Handler) showMainMenu(ctx context.Context, chatID, fromID int64) {
	text, kb, err := h.mainMenu(ctx, fromID)
	if err != nil {
		h.log.Error("menu: build main failed", "error", err)
		return
	}
	if err := h.menuClient.SendMenu(ctx, chatID, text, kb); err != nil {
		h.log.Error("menu: send failed", "error", err)
	}
}

// handleCallback routes a button tap, edits the menu message in place, and
// acks the tap. Menu taps are pure DB reads — no AI is involved.
func (h *Handler) handleCallback(ctx context.Context, cb *callbackQuery) {
	if cb.Message == nil {
		_ = h.menuClient.AnswerCallback(ctx, cb.ID)
		return
	}

	var (
		text string
		kb   Keyboard
		err  error
	)
	data := cb.Data
	switch {
	case data == cbAsk:
		text, kb = h.askScreen()
	case strings.HasPrefix(data, "c:"):
		text, kb, err = h.categoryScreen(ctx, strings.TrimPrefix(data, "c:"))
	case strings.HasPrefix(data, "q:"):
		var id int64
		if id, err = strconv.ParseInt(strings.TrimPrefix(data, "q:"), 10, 64); err == nil {
			text, kb, err = h.answerScreen(ctx, id)
		}
	default: // cbMain and anything unknown
		text, kb, err = h.mainMenu(ctx, cb.From.ID)
	}
	if err != nil {
		h.log.Error("menu: build screen failed", "error", err, "data", data)
		_ = h.menuClient.AnswerCallback(ctx, cb.ID)
		return
	}

	if err := h.menuClient.EditMenu(ctx, cb.Message.Chat.ID, cb.Message.MessageID, text, kb); err != nil {
		h.log.Error("menu: edit failed", "error", err)
	}
	_ = h.menuClient.AnswerCallback(ctx, cb.ID)
}

// mainMenu lists categories (two per row) plus the free-text option. Admins
// also get an entry into their panel (identity-checked; customers never see it).
func (h *Handler) mainMenu(ctx context.Context, fromID int64) (string, Keyboard, error) {
	cats, err := h.menu.Categories(ctx, h.businessID)
	if err != nil {
		return "", nil, err
	}

	var kb Keyboard
	var row []Button
	for _, c := range cats {
		row = append(row, Button{Text: c, Data: truncData("c:" + c)})
		if len(row) == 2 {
			kb = append(kb, row)
			row = nil
		}
	}
	if len(row) > 0 {
		kb = append(kb, row)
	}
	kb = append(kb, []Button{{Text: "💬 Ask a question", Data: cbAsk}})
	if h.panel != nil && h.panel.isAdmin(ctx, fromID) {
		kb = append(kb, []Button{{Text: "🛠 Admin panel", Data: cbPanel}})
	}
	return welcomeText, kb, nil
}

// categoryScreen lists a category's questions, one per row.
func (h *Handler) categoryScreen(ctx context.Context, category string) (string, Keyboard, error) {
	faqs, err := h.menu.ListByCategory(ctx, h.businessID, category)
	if err != nil {
		return "", nil, err
	}

	var kb Keyboard
	for _, f := range faqs {
		kb = append(kb, []Button{{Text: truncLabel(f.Question), Data: "q:" + strconv.FormatInt(f.ID, 10)}})
	}
	kb = append(kb, []Button{{Text: "🏠 Main menu", Data: cbMain}})
	return fmt.Sprintf("📂 %s — pick a question:", category), kb, nil
}

// answerScreen shows one FAQ's answer with navigation back.
func (h *Handler) answerScreen(ctx context.Context, id int64) (string, Keyboard, error) {
	f, err := h.menu.GetByID(ctx, h.businessID, id)
	if err != nil {
		return "", nil, err
	}

	kb := Keyboard{
		{{Text: "⬅️ Back", Data: truncData("c:" + f.Category)}, {Text: "🏠 Main menu", Data: cbMain}},
		{{Text: "💬 Ask a question", Data: cbAsk}},
	}
	return fmt.Sprintf("❓ %s\n\n%s", f.Question, f.Answer), kb, nil
}

// askScreen invites free text (which flows to the AI as usual).
func (h *Handler) askScreen() (string, Keyboard) {
	text := "💬 Type your question below — I'll answer right away, and if I can't, our team will follow up here."
	return text, Keyboard{{{Text: "🏠 Main menu", Data: cbMain}}}
}

// truncLabel keeps button labels within Telegram's comfortable display width.
func truncLabel(s string) string {
	r := []rune(s)
	if len(r) <= 60 {
		return s
	}
	return string(r[:57]) + "…"
}

// truncData guards the 64-byte callback-data limit (long category names).
func truncData(s string) string {
	if len(s) <= 64 {
		return s
	}
	return s[:64]
}
