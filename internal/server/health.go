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
