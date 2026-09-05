package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoLead is returned when no lead exists for a given session.
var ErrNoLead = errors.New("no lead for session")

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

// LeadMessage is one question a lead's session asked, for the CRM profile view.
type LeadMessage struct {
	Question  string
	Answered  bool
	Channel   string
	CreatedAt time.Time
}

// LeadProfile is a captured contact plus the history of questions their session
// asked — the CRM view of a single lead.
type LeadProfile struct {
	Lead
	Messages []LeadMessage
}

// Profile returns a lead and the questions their session asked (newest first),
// joined by session. Returns ErrNoLead if no lead exists for that session.
func (l *Leads) Profile(ctx context.Context, businessID int64, sessionID string) (LeadProfile, error) {
	var p LeadProfile
	const lq = `
		SELECT id, session_id, coalesce(name,''), coalesce(email,''), coalesce(phone,'')
		FROM leads WHERE business_id = $1 AND session_id = $2`
	err := l.pool.QueryRow(ctx, lq, businessID, sessionID).
		Scan(&p.ID, &p.SessionID, &p.Name, &p.Email, &p.Phone)
	if err != nil {
		return LeadProfile{}, ErrNoLead
	}

	const cq = `
		SELECT question, was_answered, channel, created_at
		FROM conversations
		WHERE business_id = $1 AND external_user_id = $2
		ORDER BY created_at DESC LIMIT 200`
	rows, err := l.pool.Query(ctx, cq, businessID, sessionID)
	if err != nil {
		return LeadProfile{}, fmt.Errorf("lead history: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m LeadMessage
		if err := rows.Scan(&m.Question, &m.Answered, &m.Channel, &m.CreatedAt); err != nil {
			return LeadProfile{}, fmt.Errorf("scan lead message: %w", err)
		}
		p.Messages = append(p.Messages, m)
	}
	return p, rows.Err()
}

// Delete removes a lead by session. The visitor's conversation history is left
// intact (it feeds analytics); only the captured contact is dropped.
func (l *Leads) Delete(ctx context.Context, businessID int64, sessionID string) error {
	if _, err := l.pool.Exec(ctx,
		`DELETE FROM leads WHERE business_id = $1 AND session_id = $2`, businessID, sessionID); err != nil {
		return fmt.Errorf("delete lead: %w", err)
	}
	return nil
}

// ProfilesList returns up to `limit` leads (newest first) each with the
// questions their session asked — the CRM view, in a single query.
func (l *Leads) ProfilesList(ctx context.Context, businessID int64, limit int) ([]LeadProfile, error) {
	const q = `
		SELECT le.session_id, coalesce(le.name,''), coalesce(le.email,''), coalesce(le.phone,''),
		       c.question, c.was_answered, coalesce(c.channel,''), c.created_at
		FROM (SELECT session_id, name, email, phone, created_at
		      FROM leads WHERE business_id = $1
		      ORDER BY created_at DESC LIMIT $2) le
		LEFT JOIN conversations c
		  ON c.business_id = $1 AND c.external_user_id = le.session_id
		ORDER BY le.created_at DESC, c.created_at DESC`
	rows, err := l.pool.Query(ctx, q, businessID, limit)
	if err != nil {
		return nil, fmt.Errorf("lead profiles: %w", err)
	}
	defer rows.Close()

	var out []LeadProfile
	idx := map[string]int{}
	for rows.Next() {
		var session, name, email, phone string
		var question, channel *string
		var answered *bool
		var created *time.Time
		if err := rows.Scan(&session, &name, &email, &phone, &question, &answered, &channel, &created); err != nil {
			return nil, fmt.Errorf("scan lead profile: %w", err)
		}
		i, ok := idx[session]
		if !ok {
			out = append(out, LeadProfile{Lead: Lead{SessionID: session, Name: name, Email: email, Phone: phone}})
			i = len(out) - 1
			idx[session] = i
		}
		if question != nil { // LEFT JOIN: leads with no messages yield a NULL row
			m := LeadMessage{Question: *question}
			if answered != nil {
				m.Answered = *answered
			}
			if channel != nil {
				m.Channel = *channel
			}
			if created != nil {
				m.CreatedAt = *created
			}
			out[i].Messages = append(out[i].Messages, m)
		}
	}
	return out, rows.Err()
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
