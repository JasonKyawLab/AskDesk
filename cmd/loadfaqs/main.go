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
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("file", "", "path to a JSON file of FAQs")
	reset := flag.Bool("reset", false, "delete the business's existing FAQs before loading")
	delay := flag.Duration("delay", 6*time.Second, "pause between inserts (to respect AI rate limits)")
	flag.Parse()

	if *path == "" {
		return fmt.Errorf("usage: loadfaqs -file faqs.json [-reset] [-delay 6s]")
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
		if _, err := pool.Exec(ctx, "DELETE FROM faqs WHERE business_id = $1", cfg.BusinessID); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
		fmt.Printf("Deleted existing FAQs for business %d.\n", cfg.BusinessID)
	}

	_, embedder := app.BuildAI(cfg, log)
	faqStore := store.NewFAQs(pool, embedder)

	fmt.Printf("Loading %d FAQs for business %d (%.0fs between each)...\n", len(faqs), cfg.BusinessID, delay.Seconds())
	for i, f := range faqs {
		if f.Question == "" || f.Answer == "" {
			fmt.Printf("  [%d/%d] skipped (empty)\n", i+1, len(faqs))
			continue
		}
		id, err := faqStore.Create(ctx, cfg.BusinessID, f.Question, f.Answer, f.Category)
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
