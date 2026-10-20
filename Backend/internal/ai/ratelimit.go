package ai

import (
	"context"
	"sync"
	"time"
)

// RateLimiter is a simple token bucket rate limiter per provider.
type RateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*tokenBucket
	defaultRPS   float64
	defaultBurst float64
}

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
	rps        float64
	burst      float64
}

func (tb *tokenBucket) take(ctx context.Context, tokens int) error {
	fTokens := float64(tokens)
	for {
		tb.refill()
		if tb.tokens >= fTokens {
			tb.tokens -= fTokens
			return nil
		}
		// Wait for next token
		waitTime := time.Duration((fTokens-tb.tokens)/tb.rps * float64(time.Second))
		if waitTime < time.Millisecond {
			waitTime = time.Millisecond
		}
		timer := time.NewTimer(waitTime)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			timer.Stop()
		}
	}
}

func (tb *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = min(tb.burst, tb.tokens+elapsed*tb.rps)
	tb.lastRefill = now
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// NewRateLimiter creates a new rate limiter with default RPS and burst per provider.
func NewRateLimiter(defaultRPS, defaultBurst float64) *RateLimiter {
	return &RateLimiter{
		buckets:      make(map[string]*tokenBucket),
		defaultRPS:   defaultRPS,
		defaultBurst: defaultBurst,
	}
}

// Take reserves tokens for a provider, blocking until available.
func (rl *RateLimiter) Take(ctx context.Context, provider string, tokens int) error {
	rl.mu.Lock()
	bucket, ok := rl.buckets[provider]
	if !ok {
		bucket = &tokenBucket{
			tokens:     rl.defaultBurst,
			lastRefill: time.Now(),
			rps:        rl.defaultRPS,
			burst:      rl.defaultBurst,
		}
		rl.buckets[provider] = bucket
	}
	rl.mu.Unlock()
	return bucket.take(ctx, tokens)
}

// SetLimit configures the rate limit for a specific provider.
func (rl *RateLimiter) SetLimit(provider string, rps, burst float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.buckets[provider] = &tokenBucket{
		tokens:     burst,
		lastRefill: time.Now(),
		rps:        rps,
		burst:      burst,
	}
}