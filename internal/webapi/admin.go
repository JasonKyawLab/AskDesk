package webapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

// AdminStore is the privileged data access the admin API needs.
type AdminStore interface {
	TodayStats(ctx context.Context, businessID int64) (store.DailyStats, error)
	PendingUnanswered(ctx context.Context, businessID int64, limit int) ([]store.PendingQuestion, error)
	GetUnanswered(ctx context.Context, businessID, id int64) (store.UnansweredTarget, error)
	ResolveUnanswered(ctx context.Context, businessID, id int64) error
}

// Deliverer sends an admin reply to the customer's originating channel.
type Deliverer interface {
	Deliver(ctx context.Context, channel core.Channel, replyTo, text string) error
}

// AdminAuth resolves a privileged admin key to a business id.
type AdminAuth interface {
	IDByAdminKey(ctx context.Context, adminKey string) (int64, error)
}

// AdminHandler serves the privileged /api/v1/admin endpoints so a frontend can
// build its own support inbox. It is authenticated by an X-Admin-Key header
// (separate from the public api_key) and intentionally sends NO CORS headers —
// call it from a backend, never directly from a browser.
type AdminHandler struct {
	store     AdminStore
	deliverer Deliverer
	auth      AdminAuth
	log       *slog.Logger
	mux       *http.ServeMux
}

// NewAdmin builds the admin API handler.
func NewAdmin(s AdminStore, deliverer Deliverer, auth AdminAuth, log *slog.Logger) *AdminHandler {
	h := &AdminHandler{store: s, deliverer: deliverer, auth: auth, log: log, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /api/v1/admin/stats", h.handleStats)
	h.mux.HandleFunc("GET /api/v1/admin/pending", h.handlePending)
	h.mux.HandleFunc("POST /api/v1/admin/reply", h.handleReply)
	h.mux.HandleFunc("POST /api/v1/admin/dismiss", h.handleDismiss)
	return h
}

// ServeHTTP authenticates the admin key, then routes. No CORS (backend-only).
func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := h.auth.IDByAdminKey(r.Context(), r.Header.Get("X-Admin-Key"))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or missing X-Admin-Key")
		return
	}
	ctx := context.WithValue(r.Context(), businessKey{}, id)
	h.mux.ServeHTTP(w, r.WithContext(ctx))
}

func (h *AdminHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.TodayStats(r.Context(), businessID(r.Context()))
	if err != nil {
		h.serverError(w, "stats", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"total": st.Total, "answered": st.Answered, "unanswered": st.Unanswered,
	})
}

type pendingItem struct {
	ID       int64  `json:"id"`
	Question string `json:"question"`
	Customer string `json:"customer"`
}

func (h *AdminHandler) handlePending(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.PendingUnanswered(r.Context(), businessID(r.Context()), 50)
	if err != nil {
		h.serverError(w, "pending", err)
		return
	}
	out := make([]pendingItem, 0, len(items))
	for _, it := range items {
		out = append(out, pendingItem{ID: it.ID, Question: it.Question, Customer: it.UserName})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pending": out})
}

type replyRequest struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

func (h *AdminHandler) handleReply(w http.ResponseWriter, r *http.Request) {
	var req replyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.ID == 0 || strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "id and message are required")
		return
	}

	id := businessID(r.Context())
	target, err := h.store.GetUnanswered(r.Context(), id, req.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no such pending question")
		return
	}
	if err := h.deliverer.Deliver(r.Context(), target.Channel, target.ReplyTo, req.Message); err != nil {
		h.serverError(w, "deliver reply", err)
		return
	}
	if err := h.store.ResolveUnanswered(r.Context(), id, req.ID); err != nil {
		h.log.Error("admin api: resolve failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type dismissRequest struct {
	ID int64 `json:"id"`
}

func (h *AdminHandler) handleDismiss(w http.ResponseWriter, r *http.Request) {
	var req dismissRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || req.ID == 0 {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := h.store.ResolveUnanswered(r.Context(), businessID(r.Context()), req.ID); err != nil {
		h.serverError(w, "dismiss", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *AdminHandler) serverError(w http.ResponseWriter, what string, err error) {
	h.log.Error("admin api: "+what+" failed", "error", err)
	writeError(w, http.StatusInternalServerError, "something went wrong")
}
