package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Lead is a captured web-widget contact.
type Lead struct {
	ID        int64
	SessionID string
	Name      string
	Email     string
	Phone     string
}

// Leads stores contact details captured by the web widget's contact-gate.
type Leads struct {
	pool *pgxpool.Pool
}

// NewLeads constructs a Leads store.
func NewLeads(pool *pgxpool.Pool) *Leads {
	return &Leads{pool: pool}
}

// Upsert records (or updates) a visitor's contact details for a session. Blank
// fields don't overwrite existing values, so a later message that adds a phone
// keeps the earlier email.
func (l *Leads) Upsert(ctx context.Context, businessID int64, sessionID, name, email, phone string) error {
	const q = `
		INSERT INTO leads (business_id, session_id, name, email, phone)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (business_id, session_id) DO UPDATE SET
			name       = coalesce(nullif(excluded.name,  ''), leads.name),
			email      = coalesce(nullif(excluded.email, ''), leads.email),
			phone      = coalesce(nullif(excluded.phone, ''), leads.phone),
			updated_at = now()`
	if _, err := l.pool.Exec(ctx, q, businessID, sessionID,
		nullIfEmpty(name), nullIfEmpty(email), nullIfEmpty(phone)); err != nil {
		return fmt.Errorf("upsert lead: %w", err)
	}
	return nil
}

// List returns a business's captured leads, newest first (for an admin view).
func (l *Leads) List(ctx context.Context, businessID int64, limit int) ([]Lead, error) {
	const q = `
		SELECT id, session_id, coalesce(name,''), coalesce(email,''), coalesce(phone,'')
		FROM leads WHERE business_id = $1
		ORDER BY created_at DESC LIMIT $2`
	rows, err := l.pool.Query(ctx, q, businessID, limit)
	if err != nil {
		return nil, fmt.Errorf("list leads: %w", err)
	}
	defer rows.Close()

	var out []Lead
	for rows.Next() {
		var it Lead
		if err := rows.Scan(&it.ID, &it.SessionID, &it.Name, &it.Email, &it.Phone); err != nil {
			return nil, fmt.Errorf("scan lead: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
