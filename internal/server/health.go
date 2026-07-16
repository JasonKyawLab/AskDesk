package server

import (
	"context"
	"net/http"
	"time"
)

// handleHealth is the liveness probe: the process is up and serving.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleWake is a lightweight keep-alive endpoint for an external uptime/cron
// service to ping. It touches no dependencies so it always responds instantly
// and wakes a sleeping free-tier instance. Returns the current time so the
// caller can log when the wake happened.
func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "awake",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// handleReady is the readiness probe: reports ready only when the database
// (if configured) is reachable, so traffic is held back during an outage.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.db.Ping(ctx); err != nil {
			s.log.Error("readiness: database unreachable", "error", err)
			writeError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
