// Package ai provides the AI backends behind the reply engine: a single
// Provider interface, and a Chain that fails over between providers in cost
// order with a per-provider circuit breaker.
package ai

import (
	"context"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// Provider is one AI backend that answers a question grounded in FAQ context.
type Provider interface {
	Name() string
	GenerateReply(ctx context.Context, question string, faqs []core.Match) (string, error)
}
