// Package server wires the HTTP web tier: routing and middleware.
package server

import (
	"log/slog"
	"net/http"
)

// Server holds the dependencies shared by all HTTP handlers.
type Server struct {
	log *slog.Logger
}

// New constructs a Server.
func New(log *slog.Logger) *Server {
	return &Server{log: log}
}

// Routes builds the service handler wrapped in the middleware chain
// (outermost first: recover, then log).
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)

	return s.recoverPanic(s.logRequests(mux))
}
