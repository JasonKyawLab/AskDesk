// Package widget serves the embeddable web chat widget: a single self-contained
// script any website drops in with one <script> tag. It renders the chat bubble
// and talks to the existing /api/v1 endpoints — no build step, no dependencies.
package widget

import (
	"bytes"
	_ "embed"
	"net/http"
	"time"
)

//go:embed widget.js
var widgetJS []byte

//go:embed demo.html
var demoHTML []byte

var start = time.Now()

// Handler serves the widget script and a demo page.
type Handler struct{}

// New builds the widget handler.
func New() *Handler { return &Handler{} }

// ServeScript serves widget.js with long-lived caching and permissive CORS (it's
// public, non-secret JavaScript embedded on customer sites).
func (h *Handler) ServeScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=120")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.ServeContent(w, r, "widget.js", start, bytes.NewReader(widgetJS))
}

// ServeDemo serves a self-contained page that embeds the widget for testing.
func (h *Handler) ServeDemo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(demoHTML)
}
