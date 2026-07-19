package webapi

import (
	"fmt"
	"sync"
	"time"
)

// rateLimiter is a simple in-memory fixed-window counter (no Redis needed, so it
// works on the free tier). Limits are read per call, so they can be changed at
// runtime via business settings without restarting.
type rateLimiter struct {
	mu        sync.Mutex
	windows   map[string]*rlWindow
	lastSweep time.Time
	now       func() time.Time
}

type rlWindow struct {
	resetAt time.Time
	count   int
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{windows: make(map[string]*rlWindow), now: time.Now}
}

// allow reports whether key is under limit for the current minute, counting the
// hit. A limit <= 0 means unlimited.
func (l *rateLimiter) allow(key string, limit int) bool {
	if limit <= 0 {
		return true
	}
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now)

	w := l.windows[key]
	if w == nil || now.After(w.resetAt) {
		w = &rlWindow{resetAt: now.Add(time.Minute)}
		l.windows[key] = w
	}
	if w.count >= limit {
		return false
	}
	w.count++
	return true
}

// sweep drops expired windows at most once a minute so the map stays small.
func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for k, w := range l.windows {
		if now.After(w.resetAt) {
			delete(l.windows, k)
		}
	}
}

func globalKey(businessID int64) string { return fmt.Sprintf("g:%d", businessID) }

func userKey(businessID int64, session string) string {
	return fmt.Sprintf("u:%d:%s", businessID, session)
}
