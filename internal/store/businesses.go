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
}

// Defaults (used when a field is empty/zero).
const (
	DefaultWelcome         = "👋 Welcome to {name} support! Pick a topic below, or just type your question."
	DefaultFallback        = "Thanks for your message! I couldn't answer that one myself, so I've passed it to our team — we'll follow up here soon."
	DefaultAsk             = "💬 Type your question below — I'll answer right away, and if I can't, our team will follow up here."
	DefaultAskRatePerMin   = 10
	DefaultAskGlobalPerMin = 60
)

// resolve fills empty fields with defaults and substitutes {name}. businessName
// is the businesses.name column, used when DisplayName is unset.
func (s BusinessSettings) resolve(businessName string) BusinessSettings {
	name := firstNonEmpty(s.DisplayName, businessName)
	return BusinessSettings{
		DisplayName:     name,
		WelcomeMessage:  subName(firstNonEmpty(s.WelcomeMessage, DefaultWelcome), name),
		FallbackMessage: subName(firstNonEmpty(s.FallbackMessage, DefaultFallback), name),
		AskPrompt:       subName(firstNonEmpty(s.AskPrompt, DefaultAsk), name),
		AskRatePerMin:   firstPositive(s.AskRatePerMin, DefaultAskRatePerMin),
		AskGlobalPerMin: firstPositive(s.AskGlobalPerMin, DefaultAskGlobalPerMin),
	}
}

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
	pool *pgxpool.Pool
}

// NewBusinesses constructs a Businesses store.
func NewBusinesses(pool *pgxpool.Pool) *Businesses {
	return &Businesses{pool: pool}
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

// Settings returns fully resolved settings (defaults applied, {name} filled in)
// — what the bot renders.
func (b *Businesses) Settings(ctx context.Context, businessID int64) (BusinessSettings, error) {
	name, raw, err := b.load(ctx, businessID)
	if err != nil {
		return BusinessSettings{}, err
	}
	return raw.resolve(name), nil
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

// Fallback returns the resolved fallback message, or the plain default if
// settings can't be loaded. It implements core.FallbackProvider and never errors.
func (b *Businesses) Fallback(ctx context.Context, businessID int64) string {
	s, err := b.Settings(ctx, businessID)
	if err != nil {
		return DefaultFallback
	}
	return s.FallbackMessage
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
