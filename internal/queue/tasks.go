// Package queue is the Redis-backed task queue that decouples the web tier
// (which enqueues customer messages) from the worker tier (which runs the
// engine and delivers replies). This is what enables graceful degradation:
// a failed AI call returns an error and asynq retries the task later.
package queue

import "github.com/JasonKyawLab/AskDesk/internal/core"

// TypeCustomerMessage is the asynq task type for an inbound customer message.
const TypeCustomerMessage = "customer:message"

// CustomerMessagePayload is the enqueued work: a normalized message plus the
// channel-specific address to reply to (e.g. a Telegram chat id).
type CustomerMessagePayload struct {
	BusinessID int64  `json:"business_id"`
	Channel    string `json:"channel"`
	UserID     string `json:"user_id"`
	Text       string `json:"text"`
	ReplyTo    string `json:"reply_to"`
}

func (p CustomerMessagePayload) message() core.Message {
	return core.Message{
		BusinessID: p.BusinessID,
		Channel:    core.Channel(p.Channel),
		UserID:     p.UserID,
		Text:       p.Text,
	}
}
