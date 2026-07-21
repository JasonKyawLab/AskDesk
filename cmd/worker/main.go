// Command worker runs the AskDesk worker tier: it consumes queued customer
// messages, runs the reply engine (RAG + AI), and delivers replies back over
// the originating channel.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/JasonKyawLab/AskDesk/internal/admin"
	"github.com/JasonKyawLab/AskDesk/internal/app"
	"github.com/JasonKyawLab/AskDesk/internal/auth"
	"github.com/JasonKyawLab/AskDesk/internal/config"
	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/logging"
	"github.com/JasonKyawLab/AskDesk/internal/queue"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

const workerConcurrency = 10

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "startup error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logging.New(cfg.IsProduction(), cfg.LogLevel)
	log.Info("starting AskDesk worker", "env", cfg.Env)

	if cfg.RedisURL == "" {
		return fmt.Errorf("ASKDESK_REDIS_URL is required")
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("ASKDESK_DATABASE_URL is required")
	}

	pool, err := store.NewPool(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	genProvider, embedder := app.BuildAI(cfg, log)
	engine := core.NewEngine(
		store.NewFAQs(pool, embedder),
		genProvider,
		store.NewConversations(pool),
		store.NewBusinesses(pool),
		log,
		core.WithGenerationFloor(cfg.GenerationFloor),
	)

	var signer *auth.Signer
	if cfg.MagicLinkSecret != "" {
		signer = auth.NewSigner(cfg.MagicLinkSecret)
	}
	deliverer := app.NewChannelDeliverer(cfg, store.NewWebReplies(pool))
	adminSvc := admin.NewService(store.NewAdmins(pool), deliverer, signer, cfg.PublicURL)

	dispatcher := app.NewDispatcher(engine, adminSvc, deliverer, log)
	srv, err := queue.NewServer(cfg.RedisURL, workerConcurrency, queue.NewProcessor(dispatcher, log))
	if err != nil {
		return fmt.Errorf("start worker server: %w", err)
	}

	log.Info("worker ready, consuming tasks", "concurrency", workerConcurrency)
	return srv.Run()
}
