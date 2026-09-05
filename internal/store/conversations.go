package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// Conversations implements core.ConversationStore backed by Postgres.
type Conversations struct {
	pool *pgxpool.Pool
}

// NewConversations constructs a Conversations store.
func NewConversations(pool *pgxpool.Pool) *Conversations {
	return &Conversations{pool: pool}
}

// PurgeOlderThan deletes conversations older than the given number of days and
// returns how many were removed. Related unanswered_queue rows cascade away via
// their foreign key. Used by the retention job to bound the database and honour
// data-retention limits. days <= 0 is a no-op.
func (c *Conversations) PurgeOlderThan(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	tag, err := c.pool.Exec(ctx,
		`DELETE FROM conversations WHERE created_at < now() - make_interval(days => $1)`, days)
	if err != nil {
		return 0, fmt.Errorf("purge conversations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// LogConversation inserts one interaction and returns its id.
func (c *Conversations) LogConversation(ctx context.Context, rec core.ConversationRecord) (int64, error) {
	const q = `
		INSERT INTO conversations
			(business_id, channel, external_user_id, external_user_name, question,
			 matched_faq_id, ai_answer, confidence_score, was_answered)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	var id int64
	err := c.pool.QueryRow(ctx, q,
		rec.BusinessID, string(rec.Channel), rec.UserID, nullIfEmpty(rec.UserName), rec.Question,
		rec.MatchedFAQID, rec.AIAnswer, rec.Confidence, rec.WasAnswered,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert conversation: %w", err)
	}
	return id, nil
}

// EnqueueUnanswered flags a low-confidence question for an admin.
func (c *Conversations) EnqueueUnanswered(ctx context.Context, conversationID int64, question string) error {
	const q = `INSERT INTO unanswered_queue (conversation_id, question) VALUES ($1, $2)`
	if _, err := c.pool.Exec(ctx, q, conversationID, question); err != nil {
		return fmt.Errorf("insert unanswered: %w", err)
	}
	return nil
}
