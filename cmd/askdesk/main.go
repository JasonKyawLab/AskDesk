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
	"github.com/JasonKyawLab/AskDesk/internal/channel/messenger"
	"github.com/JasonKyawLab/AskDesk/internal/channel/telegram"
	"github.com/JasonKyawLab/AskDesk/internal/config"
	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/editor"
	"github.com/JasonKyawLab/AskDesk/internal/logging"
	"github.com/JasonKyawLab/AskDesk/internal/queue"
	"github.com/JasonKyawLab/AskDesk/internal/server"
	"github.com/JasonKyawLab/AskDesk/internal/store"
	"github.com/JasonKyawLab/AskDesk/internal/webapi"
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

	var pool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		pool, err = store.NewPool(context.Background(), cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		defer pool.Close()
		if err := store.Migrate(cfg.DatabaseURL); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}

	srv := server.New(log, pool)

	if pool != nil {
		// Shared components — one engine, reused by every channel.
		genProvider, embedder := app.BuildAI(cfg, log)
		faqStore := store.NewFAQs(pool, embedder)
		bizStore := store.NewBusinesses(pool)
		adminStore := store.NewAdmins(pool)
		webReplies := store.NewWebReplies(pool)
		engine := core.NewEngine(faqStore, genProvider, store.NewConversations(pool), bizStore, log)
		deliverer := app.NewChannelDeliverer(cfg, webReplies)

		var signer *auth.Signer
		if cfg.MagicLinkSecret != "" {
			signer = auth.NewSigner(cfg.MagicLinkSecret)
		}

		// Web API (JSON channel) — available whenever the database is present.
		// Public (customer) endpoints under /api/v1, admin endpoints under
		// /api/v1/admin (separate X-Admin-Key, no CORS — backend only).
		srv.Mount("/api/v1/", webapi.New(engine, faqStore, bizStore, webReplies, cfg.CORSOrigins, log))
		srv.Mount("/api/v1/admin/", webapi.NewAdmin(adminStore, deliverer, bizStore, log))
		log.Info("web api enabled")

		// Chat channels (Telegram, Messenger) share one submitter: run the engine
		// inline (all-in-one) or enqueue to the worker (queue mode).
		if cfg.TelegramBotToken != "" || cfg.MessengerPageToken != "" {
			var submitter telegram.Submitter
			if cfg.RedisURL == "" {
				adminSvc := admin.NewService(adminStore, deliverer, signer, cfg.PublicURL)
				dispatcher := app.NewDispatcher(engine, adminSvc, deliverer, log)
				submitter = app.NewSyncSubmitter(dispatcher)
				log.Info("chat channels: all-in-one mode")
			} else {
				enq, err := queue.NewEnqueuer(cfg.RedisURL)
				if err != nil {
					return fmt.Errorf("connect redis: %w", err)
				}
				defer enq.Close()
				submitter = enq
				log.Info("chat channels: queue mode")
			}

			// Telegram channel.
			if cfg.TelegramBotToken != "" {
				var clientOpts []telegram.ClientOption
				if cfg.TelegramAPIURL != "" {
					clientOpts = append(clientOpts, telegram.WithBaseURL(cfg.TelegramAPIURL))
				}
				client := telegram.NewClient(cfg.TelegramBotToken, clientOpts...)
				panel := telegram.NewAdminPanel(adminStore, client, deliverer, signer, cfg.PublicURL, cfg.BusinessID, log)

				srv.Mount("POST /webhook/telegram",
					telegram.NewHandler(submitter, faqStore, client, panel, bizStore, cfg.BusinessID, cfg.TelegramWebhookSecret, log))
				log.Info("telegram webhook enabled", "business_id", cfg.BusinessID)
				if cfg.TelegramWebhookSecret == "" {
					log.Warn("telegram webhook secret is empty; requests are not verified")
				}
			}

			// Messenger channel.
			if cfg.MessengerPageToken != "" {
				var msgOpts []messenger.ClientOption
				if cfg.MessengerAPIURL != "" {
					msgOpts = append(msgOpts, messenger.WithBaseURL(cfg.MessengerAPIURL))
				}
				mClient := messenger.NewClient(cfg.MessengerPageToken, msgOpts...)
				// Best-effort: install the Get Started button + persistent menu so
				// the bot has a Telegram-style "Browse topics" entry point.
				if err := mClient.SetupProfile(context.Background(), "MENU", "📋 Browse topics", "💬 Ask a question", "ASK"); err != nil {
					log.Warn("messenger: profile setup failed", "error", err)
				}

				srv.Mount("/webhook/messenger",
					messenger.NewHandler(submitter, faqStore, mClient, bizStore, mClient, cfg.BusinessID, cfg.MessengerAppSecret, cfg.MessengerVerifyToken, log))
				log.Info("messenger webhook enabled", "business_id", cfg.BusinessID)
				if cfg.MessengerAppSecret == "" {
					log.Warn("messenger app secret is empty; requests are not verified")
				}
			}
		}

		// Magic-link web admin: FAQs, settings, and pending-question handoff.
		if cfg.MagicLinkSecret != "" {
			ed := editor.NewHandler(faqStore, bizStore, adminStore, deliverer, signer,
				cfg.IsProduction() || strings.HasPrefix(cfg.PublicURL, "https"), log)
			srv.Mount("GET /edit", http.HandlerFunc(ed.HandleEdit))
			srv.Mount("POST /edit/faqs", http.HandlerFunc(ed.HandleCreate))
			srv.Mount("POST /edit/faqs/update", http.HandlerFunc(ed.HandleUpdate))
			srv.Mount("POST /edit/faqs/delete", http.HandlerFunc(ed.HandleDelete))
			srv.Mount("POST /edit/settings", http.HandlerFunc(ed.HandleSettings))
			srv.Mount("POST /edit/reply", http.HandlerFunc(ed.HandleReply))
			srv.Mount("POST /edit/dismiss", http.HandlerFunc(ed.HandleDismiss))
			log.Info("web admin (editor + handoff) enabled")
		}
	} else if cfg.TelegramBotToken != "" && cfg.RedisURL != "" {
		// Thin web tier without a database: enqueue only; the worker runs the engine.
		enq, err := queue.NewEnqueuer(cfg.RedisURL)
		if err != nil {
			return fmt.Errorf("connect redis: %w", err)
		}
		defer enq.Close()
		srv.Mount("POST /webhook/telegram",
			telegram.NewHandler(enq, nil, nil, nil, nil, cfg.BusinessID, cfg.TelegramWebhookSecret, log))
		log.Info("telegram webhook enabled (thin web tier, queue mode)")
	}

	return serve(cfg, srv, log)
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
