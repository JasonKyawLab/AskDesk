package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// Enqueuer submits customer messages to the queue. It is the web tier's only
// dependency on the engine: the webhook enqueues and returns immediately.
type Enqueuer struct {
	client *asynq.Client
}

// NewEnqueuer connects an Enqueuer to Redis.
func NewEnqueuer(redisURL string) (*Enqueuer, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return &Enqueuer{client: asynq.NewClient(opt)}, nil
}

// Submit enqueues a normalized message with its channel reply address.
func (e *Enqueuer) Submit(ctx context.Context, msg core.Message, replyTo string) error {
	data, err := json.Marshal(CustomerMessagePayload{
		BusinessID: msg.BusinessID,
		Channel:    string(msg.Channel),
		UserID:     msg.UserID,
		Text:       msg.Text,
		ReplyTo:    replyTo,
	})
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	if _, err := e.client.EnqueueContext(ctx, asynq.NewTask(TypeCustomerMessage, data)); err != nil {
		return fmt.Errorf("enqueue: %w", err)
	}
	return nil
}

// Close releases the Redis connection.
func (e *Enqueuer) Close() error { return e.client.Close() }
