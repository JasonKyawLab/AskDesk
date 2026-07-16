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

// LogConversation inserts one interaction and returns its id.
func (c *Conversations) LogConversation(ctx context.Context, rec core.ConversationRecord) (int64, error) {
	const q = `
		INSERT INTO conversations
			(business_id, channel, external_user_id, question,
			 matched_faq_id, ai_answer, confidence_score, was_answered)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`

	var id int64
	err := c.pool.QueryRow(ctx, q,
		rec.BusinessID, string(rec.Channel), rec.UserID, rec.Question,
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
