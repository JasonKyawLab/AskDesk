package server

import "net/http"

// handleHealth is the liveness probe: the process is up and serving.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady is the readiness probe: the service is ready for traffic.
// Once Postgres and Redis are wired in, this checks them before reporting ready.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
