// Command askdesk runs the AskDesk HTTP tier.
//
// It has two modes, chosen by whether Redis is configured:
//   - queue mode (ASKDESK_REDIS_URL set): a thin web tier that validates and
//     enqueues; a separate `worker` process runs the engine.
//   - all-in-one mode (no Redis): the webhook runs the engine synchronously and
//     replies within the request. This is the free single-service deployment.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JasonKyawLab/AskDesk/internal/admin"
	"github.com/JasonKyawLab/AskDesk/internal/app"
	"github.com/JasonKyawLab/AskDesk/internal/auth"
	"github.com/JasonKyawLab/AskDesk/internal/channel/telegram"
	"github.com/JasonKyawLab/AskDesk/internal/config"
	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/editor"
	"github.com/JasonKyawLab/AskDesk/internal/logging"
	"github.com/JasonKyawLab/AskDesk/internal/queue"
	"github.com/JasonKyawLab/AskDesk/internal/server"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

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
	log.Info("starting AskDesk", "env", cfg.Env, "port", cfg.HTTPPort)

	// All-in-one mode: no Redis, so the webhook runs the engine inline.
	// The editor and the button menu need the database too.
	syncMode := cfg.TelegramBotToken != "" && cfg.RedisURL == ""
	needDB := syncMode || cfg.MagicLinkSecret != "" ||
		(cfg.TelegramBotToken != "" && cfg.DatabaseURL != "")

	var (
		pool        *pgxpool.Pool
		genProvider core.AIProvider
		embedder    store.Embedder
	)
	if needDB {
		if cfg.DatabaseURL == "" {
			return errors.New("this configuration requires ASKDESK_DATABASE_URL")
		}
		pool, err = store.NewPool(context.Background(), cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		defer pool.Close()
		if err := store.Migrate(cfg.DatabaseURL); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
		genProvider, embedder = app.BuildAI(cfg, log)
	}

	srv := server.New(log, pool)

	// Telegram webhook.
	if cfg.TelegramBotToken != "" {
		submitter, cleanup, err := buildSubmitter(cfg, log, pool, genProvider, embedder, syncMode)
		if err != nil {
			return err
		}
		if cleanup != nil {
			defer cleanup()
		}

		// Button menu + admin panel (data-driven) need the database.
		var (
			menuStore  telegram.MenuStore
			menuClient telegram.MenuClient
			panel      *telegram.AdminPanel
		)
		if pool != nil {
			var clientOpts []telegram.ClientOption
			if cfg.TelegramAPIURL != "" {
				clientOpts = append(clientOpts, telegram.WithBaseURL(cfg.TelegramAPIURL))
			}
			client := telegram.NewClient(cfg.TelegramBotToken, clientOpts...)
			menuStore = store.NewFAQs(pool, embedder)
			menuClient = client

			var signer *auth.Signer
			if cfg.MagicLinkSecret != "" {
				signer = auth.NewSigner(cfg.MagicLinkSecret)
			}
			panel = telegram.NewAdminPanel(store.NewAdmins(pool), client, signer, cfg.PublicURL, cfg.BusinessID, log)
			log.Info("telegram button menu + admin panel enabled")
		}

		srv.Mount("POST /webhook/telegram",
			telegram.NewHandler(submitter, menuStore, menuClient, panel, cfg.BusinessID, cfg.TelegramWebhookSecret, log))
		log.Info("telegram webhook enabled", "business_id", cfg.BusinessID, "mode", modeName(syncMode))
		if cfg.TelegramWebhookSecret == "" {
			log.Warn("telegram webhook secret is empty; requests are not verified")
		}
	}

	// Magic-link FAQ editor (needs DB + embedder).
	if cfg.MagicLinkSecret != "" {
		ed := editor.NewHandler(
			store.NewFAQs(pool, embedder),
			auth.NewSigner(cfg.MagicLinkSecret),
			cfg.IsProduction() || strings.HasPrefix(cfg.PublicURL, "https"),
			log,
		)
		srv.Mount("GET /edit", http.HandlerFunc(ed.HandleEdit))
		srv.Mount("POST /edit/faqs", http.HandlerFunc(ed.HandleCreate))
		srv.Mount("POST /edit/faqs/delete", http.HandlerFunc(ed.HandleDelete))
		log.Info("faq editor enabled")
	}

	return serve(cfg, srv, log)
}

// buildSubmitter returns the telegram.Submitter for the active mode: an inline
// dispatcher (all-in-one) or a Redis enqueuer (queue mode). cleanup, if
// non-nil, must be deferred by the caller.
func buildSubmitter(cfg *config.Config, log *slog.Logger, pool *pgxpool.Pool, genProvider core.AIProvider, embedder store.Embedder, syncMode bool) (telegram.Submitter, func(), error) {
	if !syncMode {
		enq, err := queue.NewEnqueuer(cfg.RedisURL)
		if err != nil {
			return nil, nil, fmt.Errorf("connect redis: %w", err)
		}
		return enq, func() { _ = enq.Close() }, nil
	}

	engine := core.NewEngine(
		store.NewFAQs(pool, embedder),
		genProvider,
		store.NewConversations(pool),
		log,
		cfg.FallbackMessage,
	)
	var signer *auth.Signer
	if cfg.MagicLinkSecret != "" {
		signer = auth.NewSigner(cfg.MagicLinkSecret)
	}
	deliverer := app.NewChannelDeliverer(cfg)
	adminSvc := admin.NewService(store.NewAdmins(pool), deliverer, signer, cfg.PublicURL)
	dispatcher := app.NewDispatcher(engine, adminSvc, deliverer, log)
	return app.NewSyncSubmitter(dispatcher), nil, nil
}

func modeName(syncMode bool) string {
	if syncMode {
		return "all-in-one"
	}
	return "queue"
}

// serve runs the HTTP server with graceful shutdown.
func serve(cfg *config.Config, srv *server.Server, log *slog.Logger) error {
	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case sig := <-shutdown:
		log.Info("shutdown started", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
		log.Info("shutdown complete")
	}
	return nil
}
