// Package editor serves the magic-link FAQ editor: a small, mobile-friendly web
// form an admin reaches through a signed short-lived link. A valid link is
// exchanged for a signed session cookie; the form then lists, adds, and deletes
// FAQs for that admin's business.
package editor

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JasonKyawLab/AskDesk/internal/auth"
	"github.com/JasonKyawLab/AskDesk/internal/core"
	"github.com/JasonKyawLab/AskDesk/internal/store"
)

const (
	sessionCookie   = "askdesk_editor"
	sessionDuration = 15 * time.Minute
)

// FAQStore is the FAQ data access the editor needs.
type FAQStore interface {
	List(ctx context.Context, businessID int64) ([]store.FAQ, error)
	Create(ctx context.Context, businessID int64, question, answer, category string) (int64, error)
	Delete(ctx context.Context, businessID, id int64) error
}

// SettingsStore is the business-settings access the editor needs.
type SettingsStore interface {
	RawSettings(ctx context.Context, businessID int64) (store.BusinessSettings, error)
	UpdateSettings(ctx context.Context, businessID int64, s store.BusinessSettings) error
}

// AdminStore is the pending-question access the editor's handoff section needs.
type AdminStore interface {
	PendingUnanswered(ctx context.Context, businessID int64, limit int) ([]store.PendingQuestion, error)
	GetUnanswered(ctx context.Context, businessID, id int64) (store.UnansweredTarget, error)
	ResolveUnanswered(ctx context.Context, businessID, id int64) error
}

// Deliverer sends an admin reply to the customer on their originating channel.
type Deliverer interface {
	Deliver(ctx context.Context, channel core.Channel, replyTo, text string) error
}

// Handler serves the editor endpoints.
type Handler struct {
	faqs      FAQStore
	settings  SettingsStore
	admin     AdminStore
	deliverer Deliverer
	signer    *auth.Signer
	secure    bool // set Secure on the session cookie (HTTPS deployments)
	log       *slog.Logger
	tmpl      *template.Template
}

// NewHandler builds the editor handler. secure should be true in production so
// the session cookie is only sent over HTTPS.
func NewHandler(faqs FAQStore, settings SettingsStore, admin AdminStore, deliverer Deliverer, signer *auth.Signer, secure bool, log *slog.Logger) *Handler {
	return &Handler{
		faqs:      faqs,
		settings:  settings,
		admin:     admin,
		deliverer: deliverer,
		signer:    signer,
		secure:    secure,
		log:       log,
		tmpl:      template.Must(template.New("page").Parse(pageTemplate)),
	}
}

// pageData is the editor page model.
type pageData struct {
	Settings store.BusinessSettings
	Pending  []store.PendingQuestion
	FAQs     []store.FAQ
}

// HandleEdit exchanges a magic-link token for a session, then renders the list.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	// A token in the URL means the admin just followed their magic link.
	if tok := r.URL.Query().Get("token"); tok != "" {
		claims, err := h.signer.Verify(tok)
		if err != nil {
			h.deny(w, "This link is invalid or has expired. Send /edit again for a new one.")
			return
		}
		h.setSession(w, claims)
		// Redirect to the clean URL so the token leaves the address bar/history.
		http.Redirect(w, r, "/edit", http.StatusSeeOther)
		return
	}

	claims, ok := h.requireSession(w, r)
	if !ok {
		return
	}

	faqs, err := h.faqs.List(r.Context(), claims.BusinessID)
	if err != nil {
		h.serverError(w, "load faqs", err)
		return
	}
	settings, err := h.settings.RawSettings(r.Context(), claims.BusinessID)
	if err != nil {
		h.serverError(w, "load settings", err)
		return
	}
	pending, err := h.admin.PendingUnanswered(r.Context(), claims.BusinessID, 20)
	if err != nil {
		h.serverError(w, "load pending", err)
		return
	}
	h.render(w, pageData{Settings: settings, Pending: pending, FAQs: faqs})
}

// HandleReply relays an admin's answer to a pending question's customer (any
// channel) and resolves the item.
func (h *Handler) HandleReply(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	message := strings.TrimSpace(r.FormValue("message"))
	if err != nil || message == "" {
		http.Redirect(w, r, "/edit", http.StatusSeeOther)
		return
	}
	target, err := h.admin.GetUnanswered(r.Context(), claims.BusinessID, id)
	if err != nil {
		http.Redirect(w, r, "/edit", http.StatusSeeOther) // already answered
		return
	}
	if err := h.deliverer.Deliver(r.Context(), target.Channel, target.ReplyTo, message); err != nil {
		h.serverError(w, "deliver reply", err)
		return
	}
	if err := h.admin.ResolveUnanswered(r.Context(), claims.BusinessID, id); err != nil {
		h.log.Error("editor: resolve failed", "error", err)
	}
	http.Redirect(w, r, "/edit", http.StatusSeeOther)
}

// HandleDismiss resolves a pending question without replying.
func (h *Handler) HandleDismiss(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/edit", http.StatusSeeOther)
		return
	}
	if err := h.admin.ResolveUnanswered(r.Context(), claims.BusinessID, id); err != nil {
		h.serverError(w, "dismiss", err)
		return
	}
	http.Redirect(w, r, "/edit", http.StatusSeeOther)
}

// HandleSettings saves the business settings (name and messages).
func (h *Handler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	s := store.BusinessSettings{
		DisplayName:     strings.TrimSpace(r.FormValue("display_name")),
		WelcomeMessage:  strings.TrimSpace(r.FormValue("welcome_message")),
		FallbackMessage: strings.TrimSpace(r.FormValue("fallback_message")),
		AskPrompt:       strings.TrimSpace(r.FormValue("ask_prompt")),
	}
	if err := h.settings.UpdateSettings(r.Context(), claims.BusinessID, s); err != nil {
		h.serverError(w, "update settings", err)
		return
	}
	http.Redirect(w, r, "/edit", http.StatusSeeOther)
}

// HandleCreate adds a FAQ from the form.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	question := r.FormValue("question")
	answer := r.FormValue("answer")
	if question == "" || answer == "" {
		http.Redirect(w, r, "/edit", http.StatusSeeOther)
		return
	}
	if _, err := h.faqs.Create(r.Context(), claims.BusinessID, question, answer, r.FormValue("category")); err != nil {
		h.serverError(w, "create faq", err)
		return
	}
	http.Redirect(w, r, "/edit", http.StatusSeeOther)
}

// HandleDelete removes a FAQ by id.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/edit", http.StatusSeeOther)
		return
	}
	if err := h.faqs.Delete(r.Context(), claims.BusinessID, id); err != nil {
		h.serverError(w, "delete faq", err)
		return
	}
	http.Redirect(w, r, "/edit", http.StatusSeeOther)
}

func (h *Handler) setSession(w http.ResponseWriter, claims auth.Claims) {
	claims.ExpiresAt = time.Now().Add(sessionDuration).Unix()
	tok, err := h.signer.Sign(claims)
	if err != nil {
		h.serverError(w, "sign session", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/edit",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(claims.ExpiresAt, 0),
	})
}

func (h *Handler) requireSession(w http.ResponseWriter, r *http.Request) (auth.Claims, bool) {
	ck, err := r.Cookie(sessionCookie)
	if err != nil {
		h.deny(w, "Your session has expired. Send /edit again for a new link.")
		return auth.Claims{}, false
	}
	claims, err := h.signer.Verify(ck.Value)
	if err != nil {
		h.deny(w, "Your session has expired. Send /edit again for a new link.")
		return auth.Claims{}, false
	}
	return claims, true
}

func (h *Handler) render(w http.ResponseWriter, data pageData) {
	h.securityHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.Execute(w, data); err != nil {
		h.log.Error("editor: render failed", "error", err)
	}
}

func (h *Handler) deny(w http.ResponseWriter, msg string) {
	h.securityHeaders(w)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(msg))
}

func (h *Handler) serverError(w http.ResponseWriter, what string, err error) {
	h.log.Error("editor: "+what+" failed", "error", err)
	http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
}

func (h *Handler) securityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
