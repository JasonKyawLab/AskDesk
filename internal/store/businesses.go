package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BusinessSettings are per-business, runtime-editable presentation strings and
// limits. Empty/zero fields fall back to defaults; "{name}" becomes the shop name.
type BusinessSettings struct {
	DisplayName     string `json:"display_name"`
	WelcomeMessage  string `json:"welcome_message"`
	FallbackMessage string `json:"fallback_message"`
	AskPrompt       string `json:"ask_prompt"`
	// AskRatePerMin caps a single user's questions per minute (anti-spam).
	AskRatePerMin int `json:"ask_rate_per_min"`
	// AskGlobalPerMin caps total questions per minute (protects the AI quota
	// during a traffic spike — the knob to lower when you peak out).
	AskGlobalPerMin int `json:"ask_global_per_min"`
	// Localized holds per-language overrides of the presentation strings, keyed
	// by language code (e.g. "my", "zh"). Authored as data (via load-messages or
	// the admin API), never hardcoded — so greetings match the business's tone.
	Localized map[string]LocalizedStrings `json:"localized,omitempty"`
}

// LocalizedStrings are the per-language presentation strings for one language.
type LocalizedStrings struct {
	WelcomeMessage  string `json:"welcome_message,omitempty"`
	FallbackMessage string `json:"fallback_message,omitempty"`
	AskPrompt       string `json:"ask_prompt,omitempty"`
}

// Defaults (used when neither a per-language override nor base text is set). The
// only strings baked into the code are English — every other language is data.
const (
	DefaultWelcome         = "👋 Welcome to {name} support! Pick a topic below, or just type your question."
	DefaultFallback        = "Thanks for your message! I couldn't answer that one myself, so I've passed it to our team — we'll follow up here soon."
	DefaultAsk             = "💬 Type your question below — I'll answer right away, and if I can't, our team will follow up here."
	DefaultAskRatePerMin   = 10
	DefaultAskGlobalPerMin = 60
)

func firstPositive(vals ...int) int {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

// ErrUnknownAPIKey means no business matched the presented API key.
var ErrUnknownAPIKey = errors.New("unknown api key")

// Businesses reads and writes business rows and their settings.
type Businesses struct {
	pool        *pgxpool.Pool
	defaultLang string // deployment default language; base custom text belongs to it
}

// NewBusinesses constructs a Businesses store. defaultLang is the deployment's
// default FAQ language (empty = "en") — the language the business's custom
// welcome/fallback/ask text is assumed to be written in.
func NewBusinesses(pool *pgxpool.Pool, defaultLang string) *Businesses {
	if defaultLang == "" {
		defaultLang = "en"
	}
	return &Businesses{pool: pool, defaultLang: strings.ToLower(defaultLang)}
}

// localize resolves settings for one language. Each string is taken from, in
// order: the per-language override for that language (data the business set) →
// the base text if this is the default language → the English default. Nothing
// is hardcoded per non-English language. {name} is substituted.
func (b *Businesses) localize(name string, raw BusinessSettings, lang string) BusinessSettings {
	lang = normLang(lang)
	display := firstNonEmpty(raw.DisplayName, name)
	loc := raw.Localized[lang]
	pick := func(override, base, def string) string {
		baseForLang := ""
		if lang == b.defaultLang {
			baseForLang = base
		}
		return subName(firstNonEmpty(override, baseForLang, def), display)
	}
	return BusinessSettings{
		DisplayName:     display,
		WelcomeMessage:  pick(loc.WelcomeMessage, raw.WelcomeMessage, DefaultWelcome),
		FallbackMessage: pick(loc.FallbackMessage, raw.FallbackMessage, DefaultFallback),
		AskPrompt:       pick(loc.AskPrompt, raw.AskPrompt, DefaultAsk),
		AskRatePerMin:   firstPositive(raw.AskRatePerMin, DefaultAskRatePerMin),
		AskGlobalPerMin: firstPositive(raw.AskGlobalPerMin, DefaultAskGlobalPerMin),
	}
}

// IDByAPIKey resolves a public API key to its business id (for the web API).
func (b *Businesses) IDByAPIKey(ctx context.Context, apiKey string) (int64, error) {
	return b.idByKey(ctx, "api_key", apiKey)
}

// IDByAdminKey resolves a privileged admin API key to its business id.
func (b *Businesses) IDByAdminKey(ctx context.Context, adminKey string) (int64, error) {
	return b.idByKey(ctx, "admin_api_key", adminKey)
}

func (b *Businesses) idByKey(ctx context.Context, column, key string) (int64, error) {
	if strings.TrimSpace(key) == "" {
		return 0, ErrUnknownAPIKey
	}
	var id int64
	// column is a fixed literal (never user input), so this is not injectable.
	err := b.pool.QueryRow(ctx, "SELECT id FROM businesses WHERE "+column+" = $1", key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrUnknownAPIKey
	}
	if err != nil {
		return 0, fmt.Errorf("business by %s: %w", column, err)
	}
	return id, nil
}

// Settings returns fully resolved settings in the deployment's default language.
func (b *Businesses) Settings(ctx context.Context, businessID int64) (BusinessSettings, error) {
	return b.SettingsFor(ctx, businessID, b.defaultLang)
}

// SettingsFor returns fully resolved settings localized for lang (welcome,
// fallback, and ask prompt in that language), with {name} filled in.
func (b *Businesses) SettingsFor(ctx context.Context, businessID int64, lang string) (BusinessSettings, error) {
	name, raw, err := b.load(ctx, businessID)
	if err != nil {
		return BusinessSettings{}, err
	}
	return b.localize(name, raw, lang), nil
}

// RawSettings returns the stored (unresolved) settings — what the edit form
// shows so an admin edits their own text, not the filled-in defaults.
func (b *Businesses) RawSettings(ctx context.Context, businessID int64) (BusinessSettings, error) {
	_, raw, err := b.load(ctx, businessID)
	return raw, err
}

// UpdateSettings persists the settings for a business.
func (b *Businesses) UpdateSettings(ctx context.Context, businessID int64, s BusinessSettings) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if _, err := b.pool.Exec(ctx, "UPDATE businesses SET settings = $2 WHERE id = $1", businessID, data); err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}

// Fallback returns the resolved fallback message in the given language, or the
// localized default if settings can't be loaded. Implements core.FallbackProvider.
func (b *Businesses) Fallback(ctx context.Context, businessID int64, lang string) string {
	s, err := b.SettingsFor(ctx, businessID, lang)
	if err != nil {
		return DefaultFallback
	}
	return s.FallbackMessage
}

// SetLocalized writes per-language message overrides into a business's settings,
// merging with what's stored (other settings and other languages are preserved).
// A language whose fields are all blank is removed. Used by the load-messages CLI.
func (b *Businesses) SetLocalized(ctx context.Context, businessID int64, byLang map[string]LocalizedStrings) error {
	_, raw, err := b.load(ctx, businessID)
	if err != nil {
		return err
	}
	if raw.Localized == nil {
		raw.Localized = map[string]LocalizedStrings{}
	}
	for lang, m := range byLang {
		lang = normLang(lang)
		if strings.TrimSpace(m.WelcomeMessage) == "" && strings.TrimSpace(m.FallbackMessage) == "" && strings.TrimSpace(m.AskPrompt) == "" {
			delete(raw.Localized, lang)
			continue
		}
		raw.Localized[lang] = m
	}
	return b.UpdateSettings(ctx, businessID, raw)
}

func (b *Businesses) load(ctx context.Context, businessID int64) (string, BusinessSettings, error) {
	const q = `SELECT name, coalesce(settings, '{}') FROM businesses WHERE id = $1`
	var name string
	var raw []byte
	err := b.pool.QueryRow(ctx, q, businessID).Scan(&name, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", BusinessSettings{}, nil
	}
	if err != nil {
		return "", BusinessSettings{}, fmt.Errorf("load business: %w", err)
	}
	var s BusinessSettings
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", BusinessSettings{}, fmt.Errorf("parse settings: %w", err)
		}
	}
	return name, s, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func subName(s, name string) string {
	return strings.ReplaceAll(s, "{name}", name)
}
