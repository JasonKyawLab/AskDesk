package ai

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

// ErrAllProvidersFailed means no provider in the chain could answer.
var ErrAllProvidersFailed = errors.New("all AI providers failed")

const (
	defaultMaxFailures = 3
	defaultCooldown    = 30 * time.Second
)

// Chain implements core.AIProvider by trying providers in order (cheapest
// first). It fails over on error and uses a circuit breaker so a repeatedly
// failing provider is skipped for a cooldown instead of retried every request.
type Chain struct {
	providers   []Provider
	breakers    []*breaker
	log         *slog.Logger
	now         func() time.Time
	maxFailures int
	cooldown    time.Duration
}

// NewChain builds a Chain from providers listed in cost order.
func NewChain(log *slog.Logger, providers ...Provider) *Chain {
	breakers := make([]*breaker, len(providers))
	for i := range breakers {
		breakers[i] = &breaker{}
	}
	return &Chain{
		providers:   providers,
		breakers:    breakers,
		log:         log,
		now:         time.Now,
		maxFailures: defaultMaxFailures,
		cooldown:    defaultCooldown,
	}
}

// GenerateReply returns the first successful provider's answer, or
// ErrAllProvidersFailed if every provider is unavailable or failing.
func (c *Chain) GenerateReply(ctx context.Context, question string, faqs []core.Match) (string, error) {
	for i, p := range c.providers {
		if !c.breakers[i].allow(c.now()) {
			c.log.Debug("provider skipped: circuit open", "provider", p.Name())
			continue
		}

		answer, err := p.GenerateReply(ctx, question, faqs)
		if err != nil {
			c.log.Warn("provider failed, trying next", "provider", p.Name(), "error", err)
			c.breakers[i].recordFailure(c.now(), c.maxFailures, c.cooldown)
			continue
		}

		c.breakers[i].recordSuccess()
		return answer, nil
	}
	return "", ErrAllProvidersFailed
}

// breaker is a per-provider circuit breaker. After maxFailures consecutive
// failures it opens for a cooldown, during which the provider is skipped.
type breaker struct {
	mu        sync.Mutex
	failures  int
	openUntil time.Time
}

func (b *breaker) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !now.Before(b.openUntil)
}

func (b *breaker) recordSuccess() {
	b.mu.Lock()
	b.failures = 0
	b.openUntil = time.Time{}
	b.mu.Unlock()
}

func (b *breaker) recordFailure(now time.Time, maxFailures int, cooldown time.Duration) {
	b.mu.Lock()
	b.failures++
	if b.failures >= maxFailures {
		b.openUntil = now.Add(cooldown)
		b.failures = 0
	}
	b.mu.Unlock()
}
