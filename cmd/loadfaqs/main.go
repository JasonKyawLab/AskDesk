// Command loadfaqs bulk-loads FAQs from a JSON file into a business's knowledge
// base, embedding each one. Use it instead of adding dozens of FAQs by hand.
//
//	loadfaqs -file faqs.json [-reset] [-delay 6s]
//
// The JSON is an array of {category, question, answer}. Requires
// ASKDESK_DATABASE_URL, ASKDESK_GEMINI_API_KEY, and ASKDESK_BUSINESS_ID.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JasonKyawLab/AskDesk/internal/app"
	"github.com/JasonKyawLab/AskDesk/internal/config"
	"github.com/JasonKyawLab/AskDesk/internal/logging"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

type faqInput struct {
	Category string `json:"category"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Language string `json:"language"` // optional per-FAQ override; falls back to -lang
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("file", "", "path to a JSON file of FAQs")
	reset := flag.Bool("reset", false, "delete existing FAQs (for -lang only) before loading")
	lang := flag.String("lang", "en", "language code for FAQs without a per-FAQ \"language\" field (e.g. en, my, zh)")
	delay := flag.Duration("delay", 6*time.Second, "pause between inserts (to respect AI rate limits)")
	retries := flag.Int("retries", 5, "max attempts per FAQ when the AI returns a transient error")
	flag.Parse()

	if *path == "" {
		return fmt.Errorf("usage: loadfaqs -file faqs.json [-lang en] [-reset] [-delay 6s]")
	}
	*lang = strings.ToLower(strings.TrimSpace(*lang))
	if *lang == "" {
		*lang = "en"
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("ASKDESK_DATABASE_URL is required")
	}
	if cfg.GeminiAPIKey == "" {
		return fmt.Errorf("ASKDESK_GEMINI_API_KEY is required (FAQs are embedded on load)")
	}

	raw, err := os.ReadFile(*path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	var faqs []faqInput
	if err := json.Unmarshal(raw, &faqs); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}

	ctx := context.Background()
	log := logging.New(false, cfg.LogLevel)

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}

	if *reset {
		// Reset only the language being loaded, so loading Myanmar FAQs doesn't
		// wipe the English set (and vice-versa).
		if _, err := pool.Exec(ctx, "DELETE FROM faqs WHERE business_id = $1 AND language = $2", cfg.BusinessID, *lang); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
		fmt.Printf("Deleted existing %q FAQs for business %d.\n", *lang, cfg.BusinessID)
	}

	_, embedder := app.BuildAI(cfg, log)
	faqStore := store.NewFAQs(pool, embedder)

	fmt.Printf("Loading %d FAQs for business %d (%.0fs between each)...\n", len(faqs), cfg.BusinessID, delay.Seconds())
	for i, f := range faqs {
		if f.Question == "" || f.Answer == "" {
			fmt.Printf("  [%d/%d] skipped (empty)\n", i+1, len(faqs))
			continue
		}
		if f.Language == "" {
			f.Language = *lang
		}
		id, err := createWithRetry(ctx, faqStore, cfg.BusinessID, f, *retries)
		if err != nil {
			return fmt.Errorf("faq %d (%q): %w", i+1, f.Question, err)
		}
		fmt.Printf("  [%d/%d] #%d  %s\n", i+1, len(faqs), id, f.Question)
		if i < len(faqs)-1 {
			time.Sleep(*delay)
		}
	}
	fmt.Println("Done.")
	return nil
}

// createWithRetry embeds and inserts one FAQ, retrying with backoff when the AI
// returns a transient error (e.g. a 503/overload). A single Gemini hiccup used
// to abort the whole batch mid-load; now it just costs a short wait.
func createWithRetry(ctx context.Context, faqs *store.FAQs, businessID int64, f faqInput, attempts int) (int64, error) {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		id, err := faqs.Create(ctx, businessID, f.Question, f.Answer, f.Category, f.Language)
		if err == nil {
			return id, nil
		}
		lastErr = err
		if attempt == attempts || !isTransient(err) {
			break
		}
		wait := time.Duration(attempt) * 5 * time.Second // 5s, 10s, 15s, ...
		fmt.Printf("      transient AI error (attempt %d/%d): %v\n      retrying in %s...\n", attempt, attempts, err, wait)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(wait):
		}
	}
	return 0, lastErr
}

// isTransient reports whether an error is worth retrying (a temporary AI/network
// hiccup) rather than a permanent failure (bad request, auth).
func isTransient(err error) bool {
	s := strings.ToLower(err.Error())
	for _, m := range []string{
		"503", "500", "502", "504", "429", "unavailable", "overloaded",
		"timeout", "deadline", "temporarily", "connection reset", "eof",
	} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
