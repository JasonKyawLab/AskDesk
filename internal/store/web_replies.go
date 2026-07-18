package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WebReply is an admin reply waiting for a web customer to poll for it.
type WebReply struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

// WebReplies stores replies destined for web/widget customers.
type WebReplies struct {
	pool *pgxpool.Pool
}

// NewWebReplies constructs a WebReplies store.
func NewWebReplies(pool *pgxpool.Pool) *WebReplies {
	return &WebReplies{pool: pool}
}

// Add queues a reply for a web session.
func (w *WebReplies) Add(ctx context.Context, businessID int64, sessionID, message string) error {
	const q = `INSERT INTO web_replies (business_id, session_id, message) VALUES ($1, $2, $3)`
	if _, err := w.pool.Exec(ctx, q, businessID, sessionID, message); err != nil {
		return fmt.Errorf("add web reply: %w", err)
	}
	return nil
}

// Since returns replies for a session with id greater than sinceID (a cursor the
// client advances), so polling is idempotent.
func (w *WebReplies) Since(ctx context.Context, businessID int64, sessionID string, sinceID int64) ([]WebReply, error) {
	const q = `
		SELECT id, message FROM web_replies
		WHERE business_id = $1 AND session_id = $2 AND id > $3
		ORDER BY id
		LIMIT 50`

	rows, err := w.pool.Query(ctx, q, businessID, sessionID, sinceID)
	if err != nil {
		return nil, fmt.Errorf("web replies since: %w", err)
	}
	defer rows.Close()

	var out []WebReply
	for rows.Next() {
		var r WebReply
		if err := rows.Scan(&r.ID, &r.Message); err != nil {
			return nil, fmt.Errorf("scan web reply: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
