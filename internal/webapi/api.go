// Package webapi is the HTTP/JSON channel adapter: a small REST API a web
// frontend (e.g. minipos.site) calls to browse FAQs and ask questions. It
// reuses the same core engine and FAQ store as every other channel.
//
// Unlike a chat webhook, this is synchronous request/response: /ask runs the
// engine and returns the answer in the HTTP body.
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

// Engine answers a free-text question.
type Engine interface {
	GenerateCustomerReply(ctx context.Context, msg core.Message) (core.Reply, error)
}

// FAQStore provides the browsable knowledge base.
type FAQStore interface {
	Categories(ctx context.Context, businessID int64) ([]string, error)
	List(ctx context.Context, businessID int64) ([]store.FAQ, error)
}

// BusinessStore authenticates API keys and provides presentation settings.
type BusinessStore interface {
	IDByAPIKey(ctx context.Context, apiKey string) (int64, error)
	Settings(ctx context.Context, businessID int64) (store.BusinessSettings, error)
}

// Handler serves the /api/v1 endpoints.
type Handler struct {
	engine  Engine
	faqs    FAQStore
	biz     BusinessStore
	origins []string // CORS allowlist; "*" allows any origin
	log     *slog.Logger
	mux     *http.ServeMux
}

// New builds the API handler. allowedOrigins is the CORS allowlist (["*"] allows
// any origin — fine here since auth is a header API key, not a cookie).
func New(engine Engine, faqs FAQStore, biz BusinessStore, allowedOrigins []string, log *slog.Logger) *Handler {
	h := &Handler{engine: engine, faqs: faqs, biz: biz, origins: allowedOrigins, log: log, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /api/v1/config", h.handleConfig)
	h.mux.HandleFunc("GET /api/v1/faqs", h.handleFAQs)
	h.mux.HandleFunc("POST /api/v1/ask", h.handleAsk)
	return h
}

type businessKey struct{}

// ServeHTTP applies CORS, authenticates the API key, then routes.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.setCORS(w, r)
	if r.Method == http.MethodOptions { // CORS preflight
		w.WriteHeader(http.StatusNoContent)
		return
	}

	id, err := h.biz.IDByAPIKey(r.Context(), r.Header.Get("X-API-Key"))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or missing X-API-Key")
		return
	}

	ctx := context.WithValue(r.Context(), businessKey{}, id)
	h.mux.ServeHTTP(w, r.WithContext(ctx))
}

func businessID(ctx context.Context) int64 {
	id, _ := ctx.Value(businessKey{}).(int64)
	return id
}

// --- endpoints ---

type configResponse struct {
	BusinessName string   `json:"business_name"`
	Welcome      string   `json:"welcome"`
	AskPrompt    string   `json:"ask_prompt"`
	Categories   []string `json:"categories"`
}

func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	id := businessID(r.Context())
	settings, err := h.biz.Settings(r.Context(), id)
	if err != nil {
		h.serverError(w, "settings", err)
		return
	}
	cats, err := h.faqs.Categories(r.Context(), id)
	if err != nil {
		h.serverError(w, "categories", err)
		return
	}
	writeJSON(w, http.StatusOK, configResponse{
		BusinessName: settings.DisplayName,
		Welcome:      settings.WelcomeMessage,
		AskPrompt:    settings.AskPrompt,
		Categories:   emptyIfNil(cats),
	})
}

type faqItem struct {
	ID       int64  `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type faqCategory struct {
	Name string    `json:"name"`
	FAQs []faqItem `json:"faqs"`
}

func (h *Handler) handleFAQs(w http.ResponseWriter, r *http.Request) {
	id := businessID(r.Context())
	cats, err := h.faqs.Categories(r.Context(), id)
	if err != nil {
		h.serverError(w, "categories", err)
		return
	}
	all, err := h.faqs.List(r.Context(), id)
	if err != nil {
		h.serverError(w, "faqs", err)
		return
	}

	byCat := map[string][]faqItem{}
	for _, f := range all {
		byCat[f.Category] = append(byCat[f.Category], faqItem{ID: f.ID, Question: f.Question, Answer: f.Answer})
	}
	out := make([]faqCategory, 0, len(cats))
	for _, c := range cats {
		out = append(out, faqCategory{Name: c, FAQs: byCat[c]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": out})
}

type askRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
}

type askResponse struct {
	Answer   string `json:"answer"`
	Answered bool   `json:"answered"`
}

func (h *Handler) handleAsk(w http.ResponseWriter, r *http.Request) {
	var req askRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	reply, err := h.engine.GenerateCustomerReply(r.Context(), core.Message{
		BusinessID: businessID(r.Context()),
		Channel:    core.ChannelWidget,
		UserID:     sessionOrAnon(req.SessionID),
		Text:       req.Message,
	})
	if err != nil {
		h.serverError(w, "generate reply", err)
		return
	}
	writeJSON(w, http.StatusOK, askResponse{Answer: reply.Text, Answered: reply.Answered})
}

// --- helpers ---

func (h *Handler) setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" || !h.originAllowed(origin) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func (h *Handler) originAllowed(origin string) bool {
	for _, o := range h.origins {
		if o == "*" || strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

func sessionOrAnon(s string) string {
	if strings.TrimSpace(s) == "" {
		return "anon"
	}
	return s
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (h *Handler) serverError(w http.ResponseWriter, what string, err error) {
	h.log.Error("webapi: "+what+" failed", "error", err)
	writeError(w, http.StatusInternalServerError, "something went wrong")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
