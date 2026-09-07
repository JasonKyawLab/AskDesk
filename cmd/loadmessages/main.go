// Command loadmessages writes per-language greeting / fallback / ask-prompt
// messages into a business's settings from a JSON file, so greetings are data
// (matching each language's tone) rather than hardcoded.
//
//	loadmessages -file messages.json
//
// The JSON keys each message by language, e.g.:
//
//	{
//	  "welcome":  {"en": "…", "my": "…", "zh": "…"},
//	  "fallback": {"en": "…", "my": "…", "zh": "…"},
//	  "ask":      {"en": "…", "my": "…", "zh": "…"}
//	}
//
// Requires ASKDESK_DATABASE_URL and ASKDESK_BUSINESS_ID. No AI calls.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/JasonKyawLab/AskDesk/internal/config"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

// messagesFile is the on-disk shape. welcome/fallback/ask are keyed by language;
// ui is keyed by language then label (so adding a language is pure data).
type messagesFile struct {
	Welcome  map[string]string            `json:"welcome"`
	Fallback map[string]string            `json:"fallback"`
	Ask      map[string]string            `json:"ask"`
	UI       map[string]map[string]string `json:"ui"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("file", "", "path to a JSON file of per-language messages")
	flag.Parse()
	if *path == "" {
		return fmt.Errorf("usage: loadmessages -file messages.json")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("ASKDESK_DATABASE_URL is required")
	}

	raw, err := os.ReadFile(*path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	var mf messagesFile
	if err := json.Unmarshal(raw, &mf); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}

	// Collect every language mentioned across the three message maps.
	byLang := map[string]store.LocalizedStrings{}
	set := func(m map[string]string, apply func(*store.LocalizedStrings, string)) {
		for lang, text := range m {
			s := byLang[lang]
			apply(&s, text)
			byLang[lang] = s
		}
	}
	set(mf.Welcome, func(s *store.LocalizedStrings, t string) { s.WelcomeMessage = t })
	set(mf.Fallback, func(s *store.LocalizedStrings, t string) { s.FallbackMessage = t })
	set(mf.Ask, func(s *store.LocalizedStrings, t string) { s.AskPrompt = t })

	if len(byLang) == 0 && len(mf.UI) == 0 {
		return fmt.Errorf("no messages or ui found in %s", *path)
	}

	ctx := context.Background()
	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}

	biz := store.NewBusinesses(pool, cfg.DefaultLanguage)
	if len(byLang) > 0 {
		if err := biz.SetLocalized(ctx, cfg.BusinessID, byLang); err != nil {
			return fmt.Errorf("save messages: %w", err)
		}
	}
	if len(mf.UI) > 0 {
		if err := biz.SetUILabels(ctx, cfg.BusinessID, mf.UI); err != nil {
			return fmt.Errorf("save ui labels: %w", err)
		}
	}

	seen := map[string]bool{}
	for l := range byLang {
		seen[l] = true
	}
	for l := range mf.UI {
		seen[l] = true
	}
	langs := make([]string, 0, len(seen))
	for l := range seen {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	fmt.Printf("Saved messages for business %d in %d language(s): %v\n", cfg.BusinessID, len(langs), langs)
	return nil
}
