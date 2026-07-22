package chatui

import (
	"context"
	"testing"

	"github.com/JasonKyawLab/AskDesk/internal/store"
)

type fakeSettings struct{ s store.BusinessSettings }

func (f fakeSettings) Settings(context.Context, int64) (store.BusinessSettings, error) {
	return f.s, nil
}

func TestIsGreeting(t *testing.T) {
	for _, g := range []string{"hi", "Hello", "  HEY ", "menu", "/start", "get started"} {
		if !IsGreeting(g) {
			t.Errorf("IsGreeting(%q) = false, want true", g)
		}
	}
	for _, q := range []string{"how much does it cost?", "pricing", ""} {
		if IsGreeting(q) {
			t.Errorf("IsGreeting(%q) = true, want false", q)
		}
	}
}

func TestWelcomeAndAsk_DefaultWhenUnset(t *testing.T) {
	if got := Welcome(context.Background(), nil, 1); got != DefaultWelcome {
		t.Errorf("Welcome(nil) = %q, want default", got)
	}
	if got := Ask(context.Background(), nil, 1); got != DefaultAsk {
		t.Errorf("Ask(nil) = %q, want default", got)
	}
}

func TestWelcomeAndAsk_BusinessOverride(t *testing.T) {
	s := fakeSettings{s: store.BusinessSettings{WelcomeMessage: "Hi from Bakery", AskPrompt: "Ask us anything"}}
	if got := Welcome(context.Background(), s, 1); got != "Hi from Bakery" {
		t.Errorf("Welcome override = %q", got)
	}
	if got := Ask(context.Background(), s, 1); got != "Ask us anything" {
		t.Errorf("Ask override = %q", got)
	}
}
