// Package chatui holds copy and helpers shared by every chat-channel adapter
// (Telegram, Messenger, …) so the menu wording and greeting logic live in one
// place instead of being duplicated per channel.
package chatui

import (
	"context"
	"strings"

	"github.com/JasonKyawLab/AskDesk/internal/store"
)

// Default customer-facing copy, used when a business hasn't set its own.
const (
	DefaultWelcome = "👋 Welcome! Pick a topic below, or just type your question."
	DefaultAsk     = "💬 Type your question below — I'll answer right away. If I can't, your message goes straight to our team and we'll follow up here."
)

// SettingsSource provides per-business presentation strings. It matches the
// store.Businesses Settings method, so adapters pass their settings store in.
type SettingsSource interface {
	Settings(ctx context.Context, businessID int64) (store.BusinessSettings, error)
}

// IsGreeting reports whether text is a greeting/menu command that should open
// the button menu instead of spending an AI call.
func IsGreeting(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "hi", "hello", "hey", "start", "menu", "get started", "/start", "/menu":
		return true
	}
	return false
}

// Welcome returns the business's configured welcome message, or the default.
func Welcome(ctx context.Context, s SettingsSource, businessID int64) string {
	if s != nil {
		if bs, err := s.Settings(ctx, businessID); err == nil && bs.WelcomeMessage != "" {
			return bs.WelcomeMessage
		}
	}
	return DefaultWelcome
}

// Ask returns the business's configured "ask a question" prompt, or the default.
func Ask(ctx context.Context, s SettingsSource, businessID int64) string {
	if s != nil {
		if bs, err := s.Settings(ctx, businessID); err == nil && bs.AskPrompt != "" {
			return bs.AskPrompt
		}
	}
	return DefaultAsk
}
