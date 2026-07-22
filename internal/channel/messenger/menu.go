package messenger

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/JasonKyawLab/AskDesk/internal/channel/chatui"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

// MenuStore is the FAQ data the button menu is built from. Categories become
// the main-menu chips; questions become carousel cards. Same shape the Telegram
// menu uses, so both channels render from one source.
type MenuStore interface {
	Categories(ctx context.Context, businessID int64) ([]string, error)
	ListByCategory(ctx context.Context, businessID int64, category string) ([]store.FAQ, error)
	GetByID(ctx context.Context, businessID, id int64) (store.FAQ, error)
}

// MenuClient is the Messenger surface the menu renders onto.
type MenuClient interface {
	SendMessage(ctx context.Context, recipientID, text string) error
	SendQuickReplies(ctx context.Context, recipientID, text string, replies []QuickReply) error
	SendCarousel(ctx context.Context, recipientID string, cards []Card) error
}

// SettingsStore provides per-business presentation strings (welcome, ask prompt).
type SettingsStore interface {
	Settings(ctx context.Context, businessID int64) (store.BusinessSettings, error)
}

// Menu payloads, delivered as postbacks (Get Started / persistent menu / card
// buttons) or quick-reply payloads: "MENU" main menu, "ASK" free-text prompt,
// "CAT:<category>" a category's questions, "FAQ:<id>" one answer.
const (
	payloadMenu   = "MENU"
	payloadAsk    = "ASK"
	prefixCat     = "CAT:"
	prefixFAQ     = "FAQ:"
	maxCategories = 12 // quick replies allow 13; keep one slot for "Ask"
)

// menuEnabled reports whether the button menu is wired up.
func (h *Handler) menuEnabled() bool {
	return h.menu != nil && h.menuClient != nil
}

// handleMenu routes a menu payload (postback or quick reply) to the right screen.
func (h *Handler) handleMenu(ctx context.Context, recipientID, payload string) {
	switch {
	case payload == payloadAsk:
		h.send(ctx, recipientID, h.menuClient.SendMessage(ctx, recipientID, chatui.Ask(ctx, h.settings, h.businessID)))
	case strings.HasPrefix(payload, prefixCat):
		h.showCategory(ctx, recipientID, strings.TrimPrefix(payload, prefixCat))
	case strings.HasPrefix(payload, prefixFAQ):
		id, err := strconv.ParseInt(strings.TrimPrefix(payload, prefixFAQ), 10, 64)
		if err != nil {
			h.showMainMenu(ctx, recipientID)
			return
		}
		h.showAnswer(ctx, recipientID, id)
	default: // payloadMenu and anything unknown
		h.showMainMenu(ctx, recipientID)
	}
}

// showMainMenu lists categories as quick-reply chips, plus an "Ask" option.
func (h *Handler) showMainMenu(ctx context.Context, recipientID string) {
	cats, err := h.menu.Categories(ctx, h.businessID)
	if err != nil {
		h.log.Error("messenger menu: categories failed", "error", err)
		return
	}
	if len(cats) > maxCategories {
		cats = cats[:maxCategories]
	}
	replies := make([]QuickReply, 0, len(cats)+1)
	for _, c := range cats {
		replies = append(replies, QuickReply{Title: c, Payload: prefixCat + c})
	}
	replies = append(replies, QuickReply{Title: "💬 Ask a question", Payload: payloadAsk})
	h.send(ctx, recipientID, h.menuClient.SendQuickReplies(ctx, recipientID, chatui.Welcome(ctx, h.settings, h.businessID), replies))
}

// showCategory sends a carousel of the category's questions, each with a
// "See answer" button, then quick replies to navigate.
func (h *Handler) showCategory(ctx context.Context, recipientID, category string) {
	faqs, err := h.menu.ListByCategory(ctx, h.businessID, category)
	if err != nil {
		h.log.Error("messenger menu: list category failed", "error", err, "category", category)
		return
	}
	if len(faqs) == 0 {
		h.showMainMenu(ctx, recipientID)
		return
	}
	cards := make([]Card, 0, len(faqs))
	for _, f := range faqs {
		cards = append(cards, Card{
			Title:   f.Question,
			Buttons: []CardButton{{Title: "See answer", Payload: prefixFAQ + strconv.FormatInt(f.ID, 10)}},
		})
	}
	if err := h.menuClient.SendCarousel(ctx, recipientID, cards); err != nil {
		h.log.Error("messenger menu: carousel failed", "error", err)
		return
	}
	h.send(ctx, recipientID, h.menuClient.SendQuickReplies(ctx, recipientID,
		fmt.Sprintf("📂 %s — tap a question above, or:", category),
		[]QuickReply{{Title: "📋 All topics", Payload: payloadMenu}, {Title: "💬 Ask a question", Payload: payloadAsk}}))
}

// showAnswer sends one FAQ's answer with quick replies back to its category,
// the main menu, and the free-text prompt.
func (h *Handler) showAnswer(ctx context.Context, recipientID string, id int64) {
	f, err := h.menu.GetByID(ctx, h.businessID, id)
	if err != nil {
		h.log.Error("messenger menu: get faq failed", "error", err, "id", id)
		h.showMainMenu(ctx, recipientID)
		return
	}
	replies := []QuickReply{{Title: "📋 All topics", Payload: payloadMenu}, {Title: "💬 Ask a question", Payload: payloadAsk}}
	if f.Category != "" {
		replies = append([]QuickReply{{Title: "⬅️ " + f.Category, Payload: prefixCat + f.Category}}, replies...)
	}
	h.send(ctx, recipientID, h.menuClient.SendQuickReplies(ctx, recipientID,
		fmt.Sprintf("❓ %s\n\n%s", f.Question, f.Answer), replies))
}

// send logs a menu delivery failure without interrupting the webhook ack.
func (h *Handler) send(_ context.Context, _ string, err error) {
	if err != nil {
		h.log.Error("messenger menu: send failed", "error", err)
	}
}
