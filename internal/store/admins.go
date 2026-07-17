package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// Admins provides admin identity checks and the read-only stats/queue queries
// behind the in-chat admin commands.
type Admins struct {
	pool *pgxpool.Pool
}

// NewAdmins constructs an Admins store.
func NewAdmins(pool *pgxpool.Pool) *Admins {
	return &Admins{pool: pool}
}

// IsAdmin reports whether the sender is a registered admin for the business on
// this channel. This is the identity-as-auth check — no passwords in chat.
func (a *Admins) IsAdmin(ctx context.Context, businessID int64, channel core.Channel, externalID string) (bool, error) {
	const q = `SELECT EXISTS (
		SELECT 1 FROM admins WHERE business_id = $1 AND channel = $2 AND external_id = $3
	)`
	var ok bool
	if err := a.pool.QueryRow(ctx, q, businessID, string(channel), externalID).Scan(&ok); err != nil {
		return false, fmt.Errorf("is admin: %w", err)
	}
	return ok, nil
}

// DailyStats is today's conversation volume for a business.
type DailyStats struct {
	Total      int
	Answered   int
	Unanswered int
}

// TodayStats returns today's answered/unanswered/total conversation counts.
func (a *Admins) TodayStats(ctx context.Context, businessID int64) (DailyStats, error) {
	const q = `
		SELECT
			count(*)                                  AS total,
			count(*) FILTER (WHERE was_answered)      AS answered,
			count(*) FILTER (WHERE NOT was_answered)  AS unanswered
		FROM conversations
		WHERE business_id = $1 AND created_at >= date_trunc('day', now())`

	var s DailyStats
	if err := a.pool.QueryRow(ctx, q, businessID).Scan(&s.Total, &s.Answered, &s.Unanswered); err != nil {
		return DailyStats{}, fmt.Errorf("today stats: %w", err)
	}
	return s, nil
}

// PendingQuestion is one unanswered item awaiting an admin.
type PendingQuestion struct {
	ID       int64
	Question string
	UserName string // customer display name, may be empty
}

// PendingUnanswered returns up to limit pending questions for a business.
func (a *Admins) PendingUnanswered(ctx context.Context, businessID int64, limit int) ([]PendingQuestion, error) {
	const q = `
		SELECT u.id, u.question, coalesce(c.external_user_name, '')
		FROM unanswered_queue u
		JOIN conversations c ON c.id = u.conversation_id
		WHERE c.business_id = $1 AND u.status = 'pending'
		ORDER BY u.created_at DESC
		LIMIT $2`

	rows, err := a.pool.Query(ctx, q, businessID, limit)
	if err != nil {
		return nil, fmt.Errorf("pending unanswered: %w", err)
	}
	defer rows.Close()

	var out []PendingQuestion
	for rows.Next() {
		var p PendingQuestion
		if err := rows.Scan(&p.ID, &p.Question, &p.UserName); err != nil {
			return nil, fmt.Errorf("scan pending: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UnansweredTarget is who to reply to for a pending question.
type UnansweredTarget struct {
	Channel  core.Channel
	ReplyTo  string // channel reply address (e.g. Telegram chat id)
	Question string
}

// GetUnanswered returns the reply target for a pending queue item, scoped to the
// business so an admin can only reply within their own tenant.
func (a *Admins) GetUnanswered(ctx context.Context, businessID, id int64) (UnansweredTarget, error) {
	const q = `
		SELECT c.channel, c.external_user_id, u.question
		FROM unanswered_queue u
		JOIN conversations c ON c.id = u.conversation_id
		WHERE u.id = $1 AND c.business_id = $2 AND u.status = 'pending'`

	var t UnansweredTarget
	var channel string
	if err := a.pool.QueryRow(ctx, q, id, businessID).Scan(&channel, &t.ReplyTo, &t.Question); err != nil {
		return UnansweredTarget{}, fmt.Errorf("get unanswered: %w", err)
	}
	t.Channel = core.Channel(channel)
	return t, nil
}

// ResolveUnanswered marks a pending item resolved, scoped to the business.
func (a *Admins) ResolveUnanswered(ctx context.Context, businessID, id int64) error {
	const q = `
		UPDATE unanswered_queue u
		SET status = 'resolved'
		FROM conversations c
		WHERE u.conversation_id = c.id AND u.id = $1 AND c.business_id = $2`
	if _, err := a.pool.Exec(ctx, q, id, businessID); err != nil {
		return fmt.Errorf("resolve unanswered: %w", err)
	}
	return nil
}
