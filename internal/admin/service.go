// Package admin handles in-chat admin commands (/stats, /unanswered). Commands
// are authorized by sender identity against the admins table — no passwords.
package admin

import (
	"context"
	"fmt"
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
}

// Service resolves admin commands into reply text.
type Service struct {
	store   Store
	signer  *auth.Signer // nil disables the /edit magic link
	baseURL string       // public base URL for magic links
}

// NewService constructs an admin Service. signer/baseURL may be empty, which
// disables the /edit command.
func NewService(s Store, signer *auth.Signer, baseURL string) *Service {
	return &Service{store: s, signer: signer, baseURL: baseURL}
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
	case "/edit":
		return s.editLink(businessID, channel, userID)
	default:
		return "Unknown command. Try /stats, /unanswered, or /edit.", true, nil
	}
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
		fmt.Fprintf(&b, "• %s\n", it.Question)
	}
	return strings.TrimRight(b.String(), "\n"), true, nil
}
