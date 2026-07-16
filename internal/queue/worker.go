package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/JasonKyawLab/AskDesk/internal/app"
)

// Processor handles queued customer messages by delegating to the shared
// dispatcher (the same pipeline the synchronous mode uses).
type Processor struct {
	dispatcher *app.Dispatcher
	log        *slog.Logger
}

// NewProcessor constructs a Processor.
func NewProcessor(dispatcher *app.Dispatcher, log *slog.Logger) *Processor {
	return &Processor{dispatcher: dispatcher, log: log}
}

// Handle processes one task. A returned error makes asynq retry with backoff
// (graceful degradation); a bad payload is non-retryable via asynq.SkipRetry.
func (p *Processor) Handle(ctx context.Context, t *asynq.Task) error {
	var payload CustomerMessagePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w: %w", err, asynq.SkipRetry)
	}
	return p.dispatcher.Dispatch(ctx, payload.message(), payload.ReplyTo)
}

// Server runs the asynq worker until it receives a termination signal.
type Server struct {
	srv *asynq.Server
	mux *asynq.ServeMux
}

// NewServer builds a worker server with the given Redis URL and concurrency.
func NewServer(redisURL string, concurrency int, processor *Processor) (*Server, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeCustomerMessage, processor.Handle)

	return &Server{
		srv: asynq.NewServer(opt, asynq.Config{Concurrency: concurrency}),
		mux: mux,
	}, nil
}

// Run starts the worker and blocks until shutdown (asynq handles signals).
func (s *Server) Run() error {
	if err := s.srv.Run(s.mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
		return err
	}
	return nil
}
