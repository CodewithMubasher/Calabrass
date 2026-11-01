package httpapi

import (
	"sync"
	"time"
)

// attemptLimiter bounds how often something may be tried: a fixed window per key.
// It is what stands between the one endpoint a stranger on the network can reach,
// claiming a pairing code, and someone guessing codes.
type attemptLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	seen   map[string]*attempts
	now    func() time.Time
}

type attempts struct {
	count int
	start time.Time
}

func newAttemptLimiter(limit int, window time.Duration) *attemptLimiter {
	return &attemptLimiter{limit: limit, window: window, seen: map[string]*attempts{}, now: time.Now}
}

// Allow records an attempt and reports whether it is within the limit.
func (l *attemptLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	// Forget keys whose window is over, so the map cannot grow without bound.
	if len(l.seen) > 1024 {
		for k, a := range l.seen {
			if now.Sub(a.start) >= l.window {
				delete(l.seen, k)
			}
		}
	}
	a := l.seen[key]
	if a == nil || now.Sub(a.start) >= l.window {
		l.seen[key] = &attempts{count: 1, start: now}
		return true
	}
	a.count++
	return a.count <= l.limit
}
