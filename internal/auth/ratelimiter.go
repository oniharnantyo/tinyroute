package auth

import (
	"sync"
	"time"
)

// RateLimiter implements per-key in-memory rate limiting using token buckets.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	getSpec func(keyID string) *RateSpec
}

type bucket struct {
	tokens    float64
	max       float64
	rate      float64 // tokens per second
	lastCheck time.Time
}

// NewRateLimiter creates a rate limiter.
// getSpec returns the RateSpec for a key ID, or nil if no limit.
func NewRateLimiter(getSpec func(keyID string) *RateSpec) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*bucket),
		getSpec: getSpec,
	}
}

// Allow checks if a request from the given key is within its rate limit.
// Returns true if allowed, false with a retry-after duration if rate limited.
func (rl *RateLimiter) Allow(keyID string) (bool, time.Duration) {
	spec := rl.getSpec(keyID)
	if spec == nil {
		return true, 0 // no rate limit configured
	}

	interval, err := time.ParseDuration(spec.Interval)
	if err != nil || interval <= 0 || spec.Requests <= 0 {
		return true, 0 // malformed spec, allow
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[keyID]
	if !ok {
		// Create new bucket
		max := float64(spec.Requests)
		b = &bucket{
			tokens:    max,
			max:       max,
			rate:      max / interval.Seconds(),
			lastCheck: time.Now(),
		}
		rl.buckets[keyID] = b
	}

	// Replenish tokens
	now := time.Now()
	elapsed := now.Sub(b.lastCheck)
	b.tokens += elapsed.Seconds() * b.rate
	if b.tokens > b.max {
		b.tokens = b.max
	}
	b.lastCheck = now

	// Check
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	// Compute retry-after
	deficit := 1 - b.tokens
	retryAfter := time.Duration(deficit/b.rate*1000) * time.Millisecond
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter
}
