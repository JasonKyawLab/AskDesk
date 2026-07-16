package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// Engine is the reply engine the dispatcher runs.
type Engine interface {
	GenerateCustomerReply(ctx context.Context, msg core.Message) (core.Reply, error)
}

// AdminHandler handles in-chat admin commands. handled=false means the message
// is a normal customer question.
type AdminHandler interface {
	HandleCommand(ctx context.Context, businessID int64, channel core.Channel, userID, text string) (reply string, handled bool, err error)
}

// Deliverer sends a finished reply back over its originating channel.
type Deliverer interface {
	Deliver(ctx context.Context, channel core.Channel, replyTo, text string) error
}

// Dispatcher is the shared "handle one message" pipeline used by BOTH the async
// worker and the synchronous all-in-one mode: admin command or engine reply,
// then delivery. Keeping it in one place means both modes behave identically.
type Dispatcher struct {
	engine    Engine
	admin     AdminHandler
	deliverer Deliverer
	log       *slog.Logger
}

// NewDispatcher constructs a Dispatcher.
func NewDispatcher(engine Engine, admin AdminHandler, deliverer Deliverer, log *slog.Logger) *Dispatcher {
	return &Dispatcher{engine: engine, admin: admin, deliverer: deliverer, log: log}
}

// Dispatch processes a normalized message and delivers the reply.
func (d *Dispatcher) Dispatch(ctx context.Context, msg core.Message, replyTo string) error {
	if reply, handled, err := d.admin.HandleCommand(ctx, msg.BusinessID, msg.Channel, msg.UserID, msg.Text); err != nil {
		return fmt.Errorf("admin command: %w", err)
	} else if handled {
		return d.deliverer.Deliver(ctx, msg.Channel, replyTo, reply)
	}

	reply, err := d.engine.GenerateCustomerReply(ctx, msg)
	if err != nil {
		return fmt.Errorf("generate reply: %w", err)
	}
	return d.deliverer.Deliver(ctx, msg.Channel, replyTo, reply.Text)
}

// SyncSubmitter implements telegram.Submitter by dispatching inline, with no
// queue. This is the all-in-one free-tier mode: the webhook runs the engine and
// replies within the request instead of enqueuing.
type SyncSubmitter struct {
	dispatcher *Dispatcher
}

// NewSyncSubmitter constructs a SyncSubmitter.
func NewSyncSubmitter(d *Dispatcher) *SyncSubmitter {
	return &SyncSubmitter{dispatcher: d}
}

// Submit dispatches the message synchronously.
func (s *SyncSubmitter) Submit(ctx context.Context, msg core.Message, replyTo string) error {
	return s.dispatcher.Dispatch(ctx, msg, replyTo)
}
