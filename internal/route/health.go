package route

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
)

// HealthStore tracks provider and per-model cooldown state.
type HealthStore struct {
	mu        sync.RWMutex
	clock     core.Clock
	path      string // state.json path
	strikes   map[string]int
	cooldowns map[string]time.Time // provider or provider#model -> cooldown end time
}

// NewHealthStore creates a health store with the given clock and state file path.
func NewHealthStore(clock core.Clock, path string) *HealthStore {
	return &HealthStore{
		clock:     clock,
		path:      path,
		strikes:   make(map[string]int),
		cooldowns: make(map[string]time.Time),
	}
}

// MemoryAffinity implements core.Affinity in-memory (non-persisted).
type MemoryAffinity struct {
	mu     sync.RWMutex
	counts map[string]int
}

// NewMemoryAffinity constructs a new MemoryAffinity instance.
func NewMemoryAffinity() *MemoryAffinity {
	return &MemoryAffinity{counts: make(map[string]int)}
}

func (m *MemoryAffinity) Count(key string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.counts[key]
}

func (m *MemoryAffinity) Touch(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counts[key]++
	return m.counts[key]
}

func (m *MemoryAffinity) Reset(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.counts, key)
}

// Available returns true if the provider/account key is not in an active cooldown window.
func (h *HealthStore) Available(provider string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	end, ok := h.cooldowns[provider]
	if !ok {
		return true
	}
	return h.clock.Now().After(end)
}

// AvailableModel returns true if neither the per-model composite key nor the provider/account key is in cooldown.
func (h *HealthStore) AvailableModel(key, model string) bool {
	if model != "" {
		if !h.Available(key + "#" + model) {
			return false
		}
	}
	return h.Available(key)
}

// Penalize records a failure and applies a cooldown.
// For connection/5xx errors, strikes escalate the cooldown duration.
func (h *HealthStore) Penalize(provider string, duration time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Escalate: each consecutive strike doubles the duration, capped at 5min
	h.strikes[provider]++
	strikes := h.strikes[provider]
	escalated := duration
	for i := 1; i < strikes && i < 5; i++ {
		escalated *= 2
	}
	if escalated > 5*time.Minute {
		escalated = 5 * time.Minute
	}

	h.cooldowns[provider] = h.clock.Now().Add(escalated)
}

// PenalizeModel applies a cooldown to a specific model key (if model != ""), or account-wide (if model == "").
func (h *HealthStore) PenalizeModel(key, model string, duration time.Duration) {
	if model != "" {
		h.Penalize(key+"#"+model, duration)
		return
	}
	h.Penalize(key, duration)
}

// CooldownEnd returns when the cooldown expires for a provider or model key, or zero time if available.
func (h *HealthStore) CooldownEnd(provider string) time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	end, ok := h.cooldowns[provider]
	if !ok || h.clock.Now().After(end) {
		return time.Time{}
	}
	return end
}

// ClearStrikes resets the strike counter for a provider (call on success).
func (h *HealthStore) ClearStrikes(provider string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.strikes, provider)
}

// stateEntry is the JSON representation of a cooldown for persistence.
type stateEntry struct {
	Provider string    `json:"provider"`
	Model    string    `json:"model,omitempty"`
	Until    time.Time `json:"until"`
	Strikes  int       `json:"strikes"`
}

type stateFile struct {
	Cooldowns []stateEntry `json:"cooldowns"`
}

// Save persists cooldown state to state.json.
func (h *HealthStore) Save() error {
	h.mu.RLock()
	now := h.clock.Now()
	var entries []stateEntry
	for prov, end := range h.cooldowns {
		if end.After(now) { // only persist active cooldowns
			pName := prov
			mName := ""
			if idx := strings.IndexByte(prov, '#'); idx != -1 {
				pName = prov[:idx]
				mName = prov[idx+1:]
			}
			entries = append(entries, stateEntry{
				Provider: pName,
				Model:    mName,
				Until:    end,
				Strikes:  h.strikes[prov],
			})
		}
	}
	h.mu.RUnlock()

	sf := stateFile{Cooldowns: entries}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// Atomic write
	tmp := h.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, h.path)
}

// Load restores cooldown state from state.json, discarding expired entries.
func (h *HealthStore) Load() error {
	data, err := os.ReadFile(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no state file, start fresh
		}
		return err
	}

	var sf stateFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.clock.Now()
	for _, entry := range sf.Cooldowns {
		if entry.Until.After(now) { // only restore active cooldowns
			key := entry.Provider
			if entry.Model != "" {
				key = entry.Provider + "#" + entry.Model
			}
			h.cooldowns[key] = entry.Until
			h.strikes[key] = entry.Strikes
		}
	}
	return nil
}

// ActiveCooldowns returns a snapshot of all currently active cooldowns.
// Used by the status command.
func (h *HealthStore) ActiveCooldowns() map[string]time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	now := h.clock.Now()
	result := make(map[string]time.Time)
	for prov, end := range h.cooldowns {
		if end.After(now) {
			result[prov] = end
		}
	}
	return result
}
