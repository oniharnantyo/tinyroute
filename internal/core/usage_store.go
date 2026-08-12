package core

import (
	"sync"
	"time"
)

type usageRecordEntry struct {
	Timestamp time.Time
	Tokens    int64
}

// UsageStore tracks rolling-window usage per account/provider key.
type UsageStore struct {
	mu      sync.RWMutex
	clock   Clock
	entries map[string][]usageRecordEntry // key -> slice of entries
}

// NewUsageStore constructs a UsageStore with the given clock.
func NewUsageStore(clock Clock) *UsageStore {
	return &UsageStore{
		clock:   clock,
		entries: make(map[string][]usageRecordEntry),
	}
}

func (s *UsageStore) now() time.Time {
	if s != nil && s.clock != nil {
		return s.clock.Now()
	}
	return time.Now()
}

// Record adds a usage event to the key ("provider/account" or "provider").
func (s *UsageStore) Record(key string, usage Usage) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	tokens := usage.InputTokens + usage.OutputTokens

	s.entries[key] = append(s.entries[key], usageRecordEntry{
		Timestamp: now,
		Tokens:    tokens,
	})
}

// Exhausted checks if the key has exceeded the QuotaConfig limits within its rolling window.
func (s *UsageStore) Exhausted(key string, quota *QuotaConfig) bool {
	if s == nil || quota == nil || key == "" {
		return false
	}
	snap := s.Snapshot(key, quota)
	if quota.Tokens > 0 && snap.UsedTokens >= quota.Tokens {
		return true
	}
	if quota.Requests > 0 && snap.UsedRequests >= quota.Requests {
		return true
	}
	return false
}

// Snapshot returns the current usage snapshot vs quota limits for the key.
func (s *UsageStore) Snapshot(key string, quota *QuotaConfig) UsageSnapshot {
	if s == nil || key == "" || quota == nil {
		return UsageSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	window := quota.Window
	if window <= 0 {
		window = 24 * time.Hour
	}
	cutoff := now.Add(-window)

	raw := s.entries[key]
	var valid []usageRecordEntry
	var totalTokens int64
	var totalRequests int
	var oldestInWindow time.Time

	for _, entry := range raw {
		if entry.Timestamp.After(cutoff) {
			valid = append(valid, entry)
			totalTokens += entry.Tokens
			totalRequests++
			if oldestInWindow.IsZero() || entry.Timestamp.Before(oldestInWindow) {
				oldestInWindow = entry.Timestamp
			}
		}
	}
	s.entries[key] = valid

	resetAt := now.Add(window)
	if !oldestInWindow.IsZero() {
		resetAt = oldestInWindow.Add(window)
	}

	return UsageSnapshot{
		Window:        window,
		UsedTokens:    totalTokens,
		UsedRequests:  totalRequests,
		LimitTokens:   quota.Tokens,
		LimitRequests: quota.Requests,
		ResetAt:       resetAt,
	}
}
