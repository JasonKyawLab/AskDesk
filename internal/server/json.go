package server

import (
	"encoding/json"
	"net/http"
)

// writeJSON serializes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// If encoding fails the status is already sent; log-and-ignore is the only
	// sensible option, and callers pass simple, encodable values.
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error body. The message is caller-controlled and must
// never contain internal details (stack traces, SQL, secrets).
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
