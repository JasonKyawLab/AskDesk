// Package server wires the HTTP web tier: routing and middleware.
package server

import (
	"context"
	"log/slog"
	"net/http"
)

// DB is the database dependency the server needs for readiness checks.
// A nil DB means the service runs without a database.
type DB interface {
	Ping(ctx context.Context) error
}

// Server holds the dependencies shared by all HTTP handlers.
type Server struct {
	log *slog.Logger
	db  DB
}

// New constructs a Server. db may be nil.
func New(log *slog.Logger, db DB) *Server {
	return &Server{log: log, db: db}
}

// Routes builds the service handler wrapped in the middleware chain
// (outermost first: recover, then log).
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)

	return s.recoverPanic(s.logRequests(mux))
}
