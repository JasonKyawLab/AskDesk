// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the service.
type Config struct {
	Env         string // "development" or "production"
	HTTPPort    int
	LogLevel    string // debug, info, warn, error
	DatabaseURL string // postgres DSN; empty runs without a database
	RedisURL    string // redis URL for the task queue (web enqueues, worker consumes)

	// Phase 1 single-tenant + channel settings.
	BusinessID            int64  // the business this instance serves
	GeminiAPIKey          string // empty falls back to a static dev provider
	GeminiGenModel        string // generation model name (override if deprecated)
	GeminiEmbedModel      string // embedding model name (override if deprecated)
	TelegramBotToken      string // empty disables the Telegram webhook
	TelegramWebhookSecret string // verified on every Telegram webhook request
	TelegramAPIURL        string // override Bot API base URL (empty = Telegram's)

	// Magic-link FAQ editor.
	MagicLinkSecret string // HMAC key for signing edit links; empty disables the editor
	PublicURL       string // public base URL used to build magic links

	// FallbackMessage is sent to a customer when the AI is unavailable.
	FallbackMessage string
}

// Load reads configuration from the environment, applying development-friendly
// defaults so a bare `go run` works without setup.
func Load() (*Config, error) {
	cfg := &Config{
		Env:                   getEnv("ASKDESK_ENV", "development"),
		LogLevel:              getEnv("ASKDESK_LOG_LEVEL", "info"),
		DatabaseURL:           getEnv("ASKDESK_DATABASE_URL", ""),
		RedisURL:              getEnv("ASKDESK_REDIS_URL", ""),
		GeminiAPIKey:          getEnv("ASKDESK_GEMINI_API_KEY", ""),
		GeminiGenModel:        getEnv("ASKDESK_GEMINI_GEN_MODEL", ""),
		GeminiEmbedModel:      getEnv("ASKDESK_GEMINI_EMBED_MODEL", ""),
		TelegramBotToken:      getEnv("ASKDESK_TELEGRAM_BOT_TOKEN", ""),
		TelegramWebhookSecret: getEnv("ASKDESK_TELEGRAM_WEBHOOK_SECRET", ""),
		TelegramAPIURL:        getEnv("ASKDESK_TELEGRAM_API_URL", ""),
		MagicLinkSecret:       getEnv("ASKDESK_MAGIC_LINK_SECRET", ""),
		FallbackMessage:       getEnv("ASKDESK_FALLBACK_MESSAGE", ""),
		// PublicURL falls back to Render's auto-injected RENDER_EXTERNAL_URL.
		PublicURL: getEnv("ASKDESK_PUBLIC_URL", getEnv("RENDER_EXTERNAL_URL", "")),
	}

	// Port falls back to PORT (which Render and many PaaS hosts inject).
	port, err := strconv.Atoi(getEnv("ASKDESK_HTTP_PORT", getEnv("PORT", "8080")))
	if err != nil {
		return nil, fmt.Errorf("ASKDESK_HTTP_PORT must be a number: %w", err)
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("ASKDESK_HTTP_PORT out of range: %d", port)
	}
	cfg.HTTPPort = port

	if v := getEnv("ASKDESK_BUSINESS_ID", ""); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("ASKDESK_BUSINESS_ID must be a number: %w", err)
		}
		cfg.BusinessID = id
	}

	return cfg, nil
}

// IsProduction reports whether the service is running in production mode.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
