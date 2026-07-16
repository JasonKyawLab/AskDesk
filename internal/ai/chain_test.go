package ai

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/JasonKyawLab/AskDesk/internal/core"
)

type fakeProvider struct {
	name   string
	answer string
	err    error
	calls  int
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) GenerateReply(context.Context, string, []core.Match) (string, error) {
	f.calls++
	return f.answer, f.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestChain_FirstProviderWins(t *testing.T) {
	p1 := &fakeProvider{name: "p1", answer: "from p1"}
	p2 := &fakeProvider{name: "p2", answer: "from p2"}
	c := NewChain(discardLogger(), p1, p2)

	ans, err := c.GenerateReply(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != "from p1" {
		t.Errorf("answer = %q, want %q", ans, "from p1")
	}
	if p2.calls != 0 {
		t.Errorf("p2 should not be called when p1 succeeds")
	}
}

func TestChain_FailsOverToNext(t *testing.T) {
	p1 := &fakeProvider{name: "p1", err: errors.New("quota exhausted")}
	p2 := &fakeProvider{name: "p2", answer: "from p2"}
	c := NewChain(discardLogger(), p1, p2)

	ans, err := c.GenerateReply(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != "from p2" {
		t.Errorf("answer = %q, want failover to %q", ans, "from p2")
	}
}

func TestChain_AllFail(t *testing.T) {
	p1 := &fakeProvider{name: "p1", err: errors.New("down")}
	p2 := &fakeProvider{name: "p2", err: errors.New("down")}
	c := NewChain(discardLogger(), p1, p2)

	if _, err := c.GenerateReply(context.Background(), "q", nil); !errors.Is(err, ErrAllProvidersFailed) {
		t.Fatalf("error = %v, want ErrAllProvidersFailed", err)
	}
}

func TestChain_CircuitBreakerOpensAndRecovers(t *testing.T) {
	p1 := &fakeProvider{name: "p1", err: errors.New("down")}
	p2 := &fakeProvider{name: "p2", answer: "from p2"}
	c := NewChain(discardLogger(), p1, p2)
	c.maxFailures = 2
	c.cooldown = time.Minute

	now := time.Unix(0, 0)
	c.now = func() time.Time { return now }

	// Two failures should open p1's breaker (p2 answers throughout).
	for i := 0; i < 2; i++ {
		if _, err := c.GenerateReply(context.Background(), "q", nil); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
	if p1.calls != 2 {
		t.Fatalf("p1 calls = %d, want 2 before breaker opens", p1.calls)
	}

	// Breaker open: p1 is skipped.
	_, _ = c.GenerateReply(context.Background(), "q", nil)
	if p1.calls != 2 {
		t.Errorf("p1 calls = %d, want 2 while circuit open (skipped)", p1.calls)
	}

	// After cooldown: p1 is tried again.
	now = now.Add(2 * time.Minute)
	_, _ = c.GenerateReply(context.Background(), "q", nil)
	if p1.calls != 3 {
		t.Errorf("p1 calls = %d, want 3 after cooldown expiry", p1.calls)
	}
}
