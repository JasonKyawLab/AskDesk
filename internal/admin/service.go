// Package admin handles in-chat admin commands (/stats, /unanswered, /reply,
// /edit). Commands are authorized by sender identity against the admins table —
// no passwords.
package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JasonKyawLab/AskDesk/internal/auth"
	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

// Store is the data access the admin service needs.
type Store interface {
	IsAdmin(ctx context.Context, businessID int64, channel core.Channel, externalID string) (bool, error)
	TodayStats(ctx context.Context, businessID int64) (store.DailyStats, error)
	PendingUnanswered(ctx context.Context, businessID int64, limit int) ([]store.PendingQuestion, error)
	GetUnanswered(ctx context.Context, businessID, id int64) (store.UnansweredTarget, error)
	ResolveUnanswered(ctx context.Context, businessID, id int64) error
}

// Deliverer sends a reply back over a channel (used by /reply to reach the customer).
type Deliverer interface {
	Deliver(ctx context.Context, channel core.Channel, replyTo, text string) error
}

// Service resolves admin commands into reply text.
type Service struct {
	store     Store
	deliverer Deliverer
	signer    *auth.Signer // nil disables the /edit magic link
	baseURL   string       // public base URL for magic links
}

// NewService constructs an admin Service. signer/baseURL may be empty (disables
// /edit); deliverer may be nil (disables /reply).
func NewService(s Store, deliverer Deliverer, signer *auth.Signer, baseURL string) *Service {
	return &Service{store: s, deliverer: deliverer, signer: signer, baseURL: baseURL}
}

const (
	unansweredLimit = 10
	editLinkTTL     = 10 * time.Minute
)

// HandleCommand handles an admin command. It returns handled=false when the
// message is not a command, or the sender is not an admin — in which case the
// caller falls back to the normal customer reply flow.
func (s *Service) HandleCommand(ctx context.Context, businessID int64, channel core.Channel, userID, text string) (string, bool, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", false, nil
	}

	admin, err := s.store.IsAdmin(ctx, businessID, channel, userID)
	if err != nil {
		return "", false, err
	}
	if !admin {
		return "", false, nil
	}

	switch strings.Fields(text)[0] {
	case "/stats":
		return s.stats(ctx, businessID)
	case "/unanswered":
		return s.unanswered(ctx, businessID)
	case "/reply":
		return s.reply(ctx, businessID, text)
	case "/edit":
		return s.editLink(businessID, channel, userID)
	default:
		return "Commands: /admin (button panel), /stats, /unanswered, /reply <id> <message>, /edit", true, nil
	}
}

func (s *Service) stats(ctx context.Context, businessID int64) (string, bool, error) {
	st, err := s.store.TodayStats(ctx, businessID)
	if err != nil {
		return "", false, err
	}
	msg := fmt.Sprintf("Today so far:\n• Total: %d\n• Answered: %d\n• Unanswered: %d",
		st.Total, st.Answered, st.Unanswered)
	return msg, true, nil
}

func (s *Service) unanswered(ctx context.Context, businessID int64) (string, bool, error) {
	items, err := s.store.PendingUnanswered(ctx, businessID, unansweredLimit)
	if err != nil {
		return "", false, err
	}
	if len(items) == 0 {
		return "No pending questions. 🎉", true, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Pending questions (%d):\n", len(items))
	for _, it := range items {
		name := it.UserName
		if name == "" {
			name = "customer"
		}
		fmt.Fprintf(&b, "#%d — %s: %s\n", it.ID, name, it.Question)
	}
	b.WriteString("\nReply with:  /reply <id> <your message>")
	return b.String(), true, nil
}

// reply relays an admin's answer to the original customer and resolves the item.
func (s *Service) reply(ctx context.Context, businessID int64, text string) (string, bool, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(text, "/reply"))
	idStr, message, ok := strings.Cut(rest, " ")
	message = strings.TrimSpace(message)
	if !ok || message == "" {
		return "Usage: /reply <id> <message>", true, nil
	}
	id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil {
		return "Usage: /reply <id> <message>  (id must be a number)", true, nil
	}
	if s.deliverer == nil {
		return "Replying is not available in this configuration.", true, nil
	}

	target, err := s.store.GetUnanswered(ctx, businessID, id)
	if err != nil {
		return fmt.Sprintf("Couldn't find pending question #%d (already answered?).", id), true, nil
	}
	if err := s.deliverer.Deliver(ctx, target.Channel, target.ReplyTo, message); err != nil {
		return "", false, fmt.Errorf("deliver reply: %w", err)
	}
	if err := s.store.ResolveUnanswered(ctx, businessID, id); err != nil {
		return "✅ Sent to the customer (couldn't mark it resolved).", true, nil
	}
	return fmt.Sprintf("✅ Sent to the customer. #%d resolved.", id), true, nil
}

func (s *Service) editLink(businessID int64, channel core.Channel, userID string) (string, bool, error) {
	if s.signer == nil || s.baseURL == "" {
		return "The FAQ editor is not configured.", true, nil
	}
	link, err := s.signer.MagicLink(s.baseURL, auth.Claims{
		BusinessID: businessID,
		AdminID:    userID,
		Channel:    string(channel),
		ExpiresAt:  time.Now().Add(editLinkTTL).Unix(),
	})
	if err != nil {
		return "", false, err
	}
	return "Here's your FAQ editor (expires in 10 min):\n" + link, true, nil
}
