// Package server wires the HTTP web tier: routing and cross-cutting middleware.
//
// The web tier stays deliberately thin. Its job is to accept requests (webhooks,
// widget calls, admin API) in milliseconds and hand real work to the core engine
// and background workers. Nothing slow belongs here.
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

// Routes builds the HTTP handler for the whole service, wrapped in the
// standard middleware chain (outermost first: recovery, then logging).
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Liveness and readiness probes for the container orchestrator / load balancer.
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)

	// Channel webhooks and the admin API attach here as they are built.

	return s.recoverPanic(s.logRequests(mux))
}
