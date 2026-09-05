package webapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// AdminLeadStore lists captured widget leads and builds a single lead's CRM
// profile (nil returns an empty list and 404s profiles).
type AdminLeadStore interface {
	List(ctx context.Context, businessID int64, limit int) ([]store.Lead, error)
	Profile(ctx context.Context, businessID int64, sessionID string) (store.LeadProfile, error)
}

// AdminAnalyticsStore runs the dashboard aggregate queries (nil disables the
// analytics endpoint).
type AdminAnalyticsStore interface {
	AnswerRate(ctx context.Context, businessID int64, days int) (store.AnswerStats, error)
	TopQuestions(ctx context.Context, businessID int64, days, limit int, onlyUnanswered bool) ([]store.QuestionCount, error)
	BusyHours(ctx context.Context, businessID int64, days int) ([]store.HourBucket, error)
	BusyDays(ctx context.Context, businessID int64, days int) ([]store.DayBucket, error)
}

// AdminHandler serves the privileged /api/v1/admin endpoints so a frontend can
// build its own support inbox. It is authenticated by an X-Admin-Key header
// (separate from the public api_key) and intentionally sends NO CORS headers —
// call it from a backend, never directly from a browser.
type AdminHandler struct {
	store     AdminStore
	leads     AdminLeadStore
	analytics AdminAnalyticsStore
	deliverer Deliverer
	auth      AdminAuth
	log       *slog.Logger
	mux       *http.ServeMux
}

// NewAdmin builds the admin API handler. leads and analytics may be nil.
func NewAdmin(s AdminStore, leads AdminLeadStore, analytics AdminAnalyticsStore, deliverer Deliverer, auth AdminAuth, log *slog.Logger) *AdminHandler {
	h := &AdminHandler{store: s, leads: leads, analytics: analytics, deliverer: deliverer, auth: auth, log: log, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /api/v1/admin/stats", h.handleStats)
	h.mux.HandleFunc("GET /api/v1/admin/pending", h.handlePending)
	h.mux.HandleFunc("GET /api/v1/admin/leads", h.handleLeads)
	h.mux.HandleFunc("GET /api/v1/admin/lead", h.handleLeadProfile)
	h.mux.HandleFunc("GET /api/v1/admin/analytics", h.handleAnalytics)
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
	ID        int64  `json:"id"`
	Question  string `json:"question"`
	Customer  string `json:"customer"`
	Channel   string `json:"channel"`    // "telegram" | "widget" | ...
	CreatedAt string `json:"created_at"` // RFC3339 UTC; "" if unknown
	SessionID string `json:"session_id"` // widget session; links to a lead
	Email     string `json:"email"`      // captured contact for this session, "" if none
	Phone     string `json:"phone"`      // captured contact for this session, "" if none
}

func (h *AdminHandler) handlePending(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.PendingUnanswered(r.Context(), businessID(r.Context()), 50)
	if err != nil {
		h.serverError(w, "pending", err)
		return
	}
	out := make([]pendingItem, 0, len(items))
	for _, it := range items {
		created := ""
		if !it.CreatedAt.IsZero() {
			created = it.CreatedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, pendingItem{ID: it.ID, Question: it.Question, Customer: it.UserName, Channel: string(it.Channel), CreatedAt: created, SessionID: it.SessionID, Email: it.Email, Phone: it.Phone})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pending": out})
}

type leadItem struct {
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
}

// handleLeads returns the widget-captured contacts so a frontend's own admin UI
// can show a CRM/leads list.
func (h *AdminHandler) handleLeads(w http.ResponseWriter, r *http.Request) {
	out := []leadItem{}
	if h.leads != nil {
		items, err := h.leads.List(r.Context(), businessID(r.Context()), 200)
		if err != nil {
			h.serverError(w, "leads", err)
			return
		}
		for _, it := range items {
			out = append(out, leadItem{SessionID: it.SessionID, Name: it.Name, Email: it.Email, Phone: it.Phone})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"leads": out})
}

// handleLeadProfile returns one lead plus the questions their session asked.
// GET /api/v1/admin/lead?session=<session_id>
func (h *AdminHandler) handleLeadProfile(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSpace(r.URL.Query().Get("session"))
	if session == "" {
		writeError(w, http.StatusBadRequest, "session is required")
		return
	}
	if h.leads == nil {
		writeError(w, http.StatusNotFound, "no such lead")
		return
	}
	p, err := h.leads.Profile(r.Context(), businessID(r.Context()), session)
	if err != nil {
		writeError(w, http.StatusNotFound, "no such lead")
		return
	}
	msgs := make([]map[string]any, 0, len(p.Messages))
	for _, m := range p.Messages {
		created := ""
		if !m.CreatedAt.IsZero() {
			created = m.CreatedAt.UTC().Format(time.RFC3339)
		}
		msgs = append(msgs, map[string]any{
			"question": m.Question, "answered": m.Answered, "channel": m.Channel, "created_at": created,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": p.SessionID, "name": p.Name, "email": p.Email, "phone": p.Phone,
		"messages": msgs,
	})
}

// handleAnalytics returns the dashboard aggregates over a window (?days=30,
// default 30; 0 = all time). GET /api/v1/admin/analytics
func (h *AdminHandler) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	if h.analytics == nil {
		writeError(w, http.StatusNotFound, "analytics unavailable")
		return
	}
	days := 30
	if v := strings.TrimSpace(r.URL.Query().Get("days")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			days = n
		}
	}
	id := businessID(r.Context())
	rate, err := h.analytics.AnswerRate(r.Context(), id, days)
	if err != nil {
		h.serverError(w, "analytics rate", err)
		return
	}
	top, err := h.analytics.TopQuestions(r.Context(), id, days, 10, false)
	if err != nil {
		h.serverError(w, "analytics top", err)
		return
	}
	gaps, err := h.analytics.TopQuestions(r.Context(), id, days, 10, true)
	if err != nil {
		h.serverError(w, "analytics gaps", err)
		return
	}
	hours, err := h.analytics.BusyHours(r.Context(), id, days)
	if err != nil {
		h.serverError(w, "analytics hours", err)
		return
	}
	dayb, err := h.analytics.BusyDays(r.Context(), id, days)
	if err != nil {
		h.serverError(w, "analytics days", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days": days,
		"answer_rate": map[string]any{
			"total": rate.Total, "answered": rate.Answered,
			"unanswered": rate.Unanswered, "answered_pct": rate.AnsweredPct(),
		},
		"top_questions":  questionCounts(top),
		"top_unanswered": questionCounts(gaps),
		"busy_hours":     hourBuckets(hours),
		"busy_days":      dayBuckets(dayb),
	})
}

func questionCounts(in []store.QuestionCount) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, q := range in {
		out = append(out, map[string]any{"question": q.Question, "count": q.Count, "unanswered": q.Unanswered})
	}
	return out
}

func hourBuckets(in []store.HourBucket) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, b := range in {
		out = append(out, map[string]any{"hour": b.Hour, "count": b.Count})
	}
	return out
}

func dayBuckets(in []store.DayBucket) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, b := range in {
		out = append(out, map[string]any{"weekday": b.Weekday, "count": b.Count})
	}
	return out
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
