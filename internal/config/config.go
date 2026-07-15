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
}

// Load reads configuration from the environment, applying development-friendly
// defaults so a bare `go run` works without setup.
func Load() (*Config, error) {
	cfg := &Config{
		Env:         getEnv("ASKDESK_ENV", "development"),
		LogLevel:    getEnv("ASKDESK_LOG_LEVEL", "info"),
		DatabaseURL: getEnv("ASKDESK_DATABASE_URL", ""),
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

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
