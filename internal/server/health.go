package server

import "net/http"

// handleHealth is the liveness probe: the process is up and serving.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady is the readiness probe: the service is ready to take traffic.
//
// Once dependencies (Postgres, Redis) are wired in, this checks them and
// reports "not ready" while any are unavailable, so the load balancer can hold
// traffic back during startup or an outage.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
