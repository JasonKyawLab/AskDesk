package store

import (
	"strings"
	"testing"
)

// localize is pure (no DB): each string comes from the per-language override →
// the base text (default language only) → the English default. Nothing per
// non-English language is hardcoded; it's all data the business sets.
func TestLocalize_UsesPerLanguageOverrides(t *testing.T) {
	b := &Businesses{defaultLang: "en"}
	raw := BusinessSettings{
		DisplayName:    "MiniPOS",
		WelcomeMessage: "Custom EN welcome for {name}",
		Localized: map[string]LocalizedStrings{
			"my": {WelcomeMessage: "မင်္ဂလာပါ {name}"},
		},
	}

	// Default language uses the base custom text.
	if en := b.localize("MiniPOS", raw, "en"); en.WelcomeMessage != "Custom EN welcome for MiniPOS" {
		t.Errorf("en welcome = %q", en.WelcomeMessage)
	}
	// A language with an override uses it, with {name} filled.
	if my := b.localize("MiniPOS", raw, "my"); my.WelcomeMessage != "မင်္ဂလာပါ MiniPOS" {
		t.Errorf("my welcome should use the override, got %q", my.WelcomeMessage)
	}
	// A language with no override and no base falls back to the English default
	// (NOT the default-language custom text — that belongs to the default lang).
	zh := b.localize("MiniPOS", raw, "zh")
	if !strings.Contains(zh.WelcomeMessage, "Welcome") || strings.Contains(zh.WelcomeMessage, "Custom EN") {
		t.Errorf("zh welcome should be the English default, got %q", zh.WelcomeMessage)
	}
}

// When the deployment default is Myanmar, the base custom text is treated as the
// Myanmar text; English (no override) falls back to the English default.
func TestLocalize_NonEnglishDefault(t *testing.T) {
	b := &Businesses{defaultLang: "my"}
	raw := BusinessSettings{DisplayName: "Kantaw", WelcomeMessage: "ကိုယ်ပိုင် မြန်မာ နှုတ်ခွန်းဆက်"}

	if my := b.localize("Kantaw", raw, "my"); my.WelcomeMessage != "ကိုယ်ပိုင် မြန်မာ နှုတ်ခွန်းဆက်" {
		t.Errorf("default (my) should use the base custom text, got %q", my.WelcomeMessage)
	}
	if en := b.localize("Kantaw", raw, "en"); !strings.Contains(en.WelcomeMessage, "Welcome") {
		t.Errorf("en should use the English default, got %q", en.WelcomeMessage)
	}
}
