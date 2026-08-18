// Package agent provides a registry and implementations for managing
// downstream coding agent configurations (Claude Code, Codex, Cline, etc.).
package clients

import (
	"sort"
	"sync"
)

// Client defines the interface implemented by each coding agent adapter.
type Client interface {
	// ID returns the unique machine-readable identifier of the agent (e.g. "claude").
	ID() string
	// Name returns the human-readable display name of the agent (e.g. "Claude Code").
	Name() string
	// Dialect returns the primary API dialect required by this agent (e.g. "anthropic", "openai").
	Dialect() string
	// NeedsModel reports whether the agent configuration requires selecting a model.
	NeedsModel() bool
	// ModelSlots returns the declared model selection slots for this agent.
	ModelSlots() []ModelSlot
	// Detect returns the current installation and configuration status of the agent.
	Detect() (Status, error)
	// Apply applies the tinyroute configuration to the agent, creating/updating config files.
	Apply(input ApplyInput) (Result, error)
	// Reset removes tinyroute configuration fields from the agent's config files.
	Reset() error
}

var (
	mu   sync.RWMutex
	byID = map[string]Client{}
)

// Register registers a Client adapter in the global registry.
func Register(c Client) {
	mu.Lock()
	defer mu.Unlock()
	byID[c.ID()] = c
}

// Get returns the registered Client for the given ID, if found.
func Get(id string) (Client, bool) {
	mu.RLock()
	defer mu.RUnlock()
	a, ok := byID[id]
	return a, ok
}

// All returns all registered Client adapters sorted by ID.
func All() []Client {
	mu.RLock()
	defer mu.RUnlock()

	res := make([]Client, 0, len(byID))
	for _, a := range byID {
		res = append(res, a)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].ID() < res[j].ID()
	})
	return res
}
