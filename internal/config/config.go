// Package config loads runtime configuration from the environment.
//
// AskDesk is a 12-factor service: every deployment difference (host, port,
// credentials) comes from the environment, never from code. This keeps moving
// between the free tier and a paid host a config change, not a rewrite.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the service.
type Config struct {
	// Env is the deployment environment: "development" or "production".
	Env string
	// HTTPPort is the port the web tier listens on.
	HTTPPort int
	// LogLevel is one of: debug, info, warn, error.
	LogLevel string
}

// Load reads configuration from the environment, applying defaults suitable
// for local development. It returns an error only when a provided value is
// malformed, so a bare `go run` works out of the box.
func Load() (*Config, error) {
	cfg := &Config{
		Env:      getEnv("ASKDESK_ENV", "development"),
		LogLevel: getEnv("ASKDESK_LOG_LEVEL", "info"),
	}

	port, err := strconv.Atoi(getEnv("ASKDESK_HTTP_PORT", "8080"))
	if err != nil {
		return nil, fmt.Errorf("ASKDESK_HTTP_PORT must be a number: %w", err)
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("ASKDESK_HTTP_PORT out of range: %d", port)
	}
	cfg.HTTPPort = port

	return cfg, nil
}

// IsProduction reports whether the service is running in production mode.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

// getEnv returns the value of key, or fallback when unset or empty.
func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
