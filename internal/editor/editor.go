// Package editor serves the magic-link FAQ editor: a small, mobile-friendly web
// form an admin reaches through a signed short-lived link. A valid link is
// exchanged for a signed session cookie; the form then lists, adds, and deletes
// FAQs for that admin's business.
package editor

import (
	"context"
	"fmt"
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
	Update(ctx context.Context, businessID, id int64, question, answer, category string) error
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

// LeadStore lists widget-captured contacts with the questions each asked (the
// CRM view). nil disables the leads section.
type LeadStore interface {
	ProfilesList(ctx context.Context, businessID int64, limit int) ([]store.LeadProfile, error)
}

// AnalyticsStore runs the dashboard aggregate queries (nil disables the section).
type AnalyticsStore interface {
	AnswerRate(ctx context.Context, businessID int64, days int) (store.AnswerStats, error)
	TopQuestions(ctx context.Context, businessID int64, days, limit int, onlyUnanswered bool) ([]store.QuestionCount, error)
	BusyHours(ctx context.Context, businessID int64, days int) ([]store.HourBucket, error)
	BusyDays(ctx context.Context, businessID int64, days int) ([]store.DayBucket, error)
}

// Handler serves the editor endpoints.
type Handler struct {
	faqs      FAQStore
	settings  SettingsStore
	admin     AdminStore
	leads     LeadStore
	analytics AnalyticsStore
	deliverer Deliverer
	signer    *auth.Signer
	secure    bool // set Secure on the session cookie (HTTPS deployments)
	log       *slog.Logger
	tmpl      *template.Template
}

// NewHandler builds the editor handler. secure should be true in production so
// the session cookie is only sent over HTTPS. leads and analytics may be nil.
func NewHandler(faqs FAQStore, settings SettingsStore, admin AdminStore, leads LeadStore, analytics AnalyticsStore, deliverer Deliverer, signer *auth.Signer, secure bool, log *slog.Logger) *Handler {
	return &Handler{
		faqs:      faqs,
		settings:  settings,
		admin:     admin,
		leads:     leads,
		analytics: analytics,
		deliverer: deliverer,
		signer:    signer,
		secure:    secure,
		log:       log,
		tmpl:      template.Must(template.New("page").Funcs(templateFuncs).Parse(pageTemplate)),
	}
}

// pageData is the editor page model.
type pageData struct {
	Settings  store.BusinessSettings
	Pending   []store.PendingQuestion
	Leads     []store.LeadProfile
	FAQs      []store.FAQ
	Analytics *analyticsView
}

// analyticsView is the /edit analytics section, pre-formatted for the template.
type analyticsView struct {
	Days        int
	Total       int
	Answered    int
	Unanswered  int
	AnsweredPct int
	Top         []store.QuestionCount
	Gaps        []store.QuestionCount
	BusyHours   []barRow // busiest hours, formatted "2 PM"
	BusyDays    []barRow // busiest weekdays, formatted "Mon"
}

// barRow is one labelled bar in a mini bar chart (count + width %).
type barRow struct {
	Label string
	Count int
	Pct   int // 0-100, relative to the busiest bucket
}

var weekdayNames = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

// templateFuncs are helpers available to the editor page template.
var templateFuncs = template.FuncMap{
	"shortdate": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format("Jan 2, 15:04")
	},
}

func hourLabel(h int) string {
	switch {
	case h == 0:
		return "12 AM"
	case h < 12:
		return fmt.Sprintf("%d AM", h)
	case h == 12:
		return "12 PM"
	default:
		return fmt.Sprintf("%d PM", h-12)
	}
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
	var leads []store.LeadProfile
	if h.leads != nil {
		if leads, err = h.leads.ProfilesList(r.Context(), claims.BusinessID, 100); err != nil {
			h.serverError(w, "load leads", err)
			return
		}
	}
	analytics, err := h.buildAnalytics(r.Context(), claims.BusinessID)
	if err != nil {
		h.serverError(w, "load analytics", err)
		return
	}
	h.render(w, pageData{Settings: settings, Pending: pending, Leads: leads, FAQs: faqs, Analytics: analytics})
}

// buildAnalytics loads and formats the dashboard aggregates (last 30 days).
// Returns nil (section hidden) when no analytics store is wired.
func (h *Handler) buildAnalytics(ctx context.Context, businessID int64) (*analyticsView, error) {
	if h.analytics == nil {
		return nil, nil
	}
	const days = 30
	rate, err := h.analytics.AnswerRate(ctx, businessID, days)
	if err != nil {
		return nil, err
	}
	top, err := h.analytics.TopQuestions(ctx, businessID, days, 8, false)
	if err != nil {
		return nil, err
	}
	gaps, err := h.analytics.TopQuestions(ctx, businessID, days, 8, true)
	if err != nil {
		return nil, err
	}
	hours, err := h.analytics.BusyHours(ctx, businessID, days)
	if err != nil {
		return nil, err
	}
	dayb, err := h.analytics.BusyDays(ctx, businessID, days)
	if err != nil {
		return nil, err
	}
	av := &analyticsView{
		Days: days, Total: rate.Total, Answered: rate.Answered,
		Unanswered: rate.Unanswered, AnsweredPct: rate.AnsweredPct(),
		Top: top, Gaps: gaps,
	}
	var maxH int
	for _, b := range hours {
		if b.Count > maxH {
			maxH = b.Count
		}
	}
	for _, b := range hours {
		av.BusyHours = append(av.BusyHours, barRow{Label: hourLabel(b.Hour), Count: b.Count, Pct: pctOf(b.Count, maxH)})
	}
	var maxD int
	for _, b := range dayb {
		if b.Count > maxD {
			maxD = b.Count
		}
	}
	for _, b := range dayb {
		name := "?"
		if b.Weekday >= 0 && b.Weekday < 7 {
			name = weekdayNames[b.Weekday]
		}
		av.BusyDays = append(av.BusyDays, barRow{Label: name, Count: b.Count, Pct: pctOf(b.Count, maxD)})
	}
	return av, nil
}

func pctOf(n, max int) int {
	if max <= 0 {
		return 0
	}
	return int(float64(n)*100/float64(max) + 0.5)
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
	rate, _ := strconv.Atoi(r.FormValue("ask_rate_per_min"))
	global, _ := strconv.Atoi(r.FormValue("ask_global_per_min"))
	s := store.BusinessSettings{
		DisplayName:     strings.TrimSpace(r.FormValue("display_name")),
		WelcomeMessage:  strings.TrimSpace(r.FormValue("welcome_message")),
		FallbackMessage: strings.TrimSpace(r.FormValue("fallback_message")),
		AskPrompt:       strings.TrimSpace(r.FormValue("ask_prompt")),
		AskRatePerMin:   rate,
		AskGlobalPerMin: global,
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

// HandleUpdate edits an existing FAQ (re-embeds the question).
func (h *Handler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	question := r.FormValue("question")
	answer := r.FormValue("answer")
	if err != nil || question == "" || answer == "" {
		http.Redirect(w, r, "/edit", http.StatusSeeOther)
		return
	}
	if err := h.faqs.Update(r.Context(), claims.BusinessID, id, question, answer, r.FormValue("category")); err != nil {
		h.serverError(w, "update faq", err)
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
