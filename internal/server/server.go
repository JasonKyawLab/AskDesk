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
	mux *http.ServeMux
}

// New constructs a Server with the base routes registered. db may be nil.
func New(log *slog.Logger, db DB) *Server {
	s := &Server{log: log, db: db, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("GET /wake", s.handleWake)
	return s
}

// Mount registers an additional route (e.g. a channel webhook).
func (s *Server) Mount(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}

// Routes returns the handler wrapped in the middleware chain
// (outermost first: recover, then log).
func (s *Server) Routes() http.Handler {
	return s.recoverPanic(s.logRequests(s.mux))
}
