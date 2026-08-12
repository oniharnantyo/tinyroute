// Package dialect provides a registry of core.Dialect implementations,
// keyed by the inbound HTTP paths each dialect owns.
//
// Concrete dialects register themselves via init(),
// so adding a dialect costs one package plus one import line in main —
// this registry and the proxy never need to change.
package dialect

import (
	"sync"

	"github.com/oniharnantyo/tinyroute/internal/core"
)

var (
	mu          sync.RWMutex
	byMountPath = map[string]core.Dialect{}
	byName      = map[string]core.Dialect{}
)

// Register adds a dialect to the registry, indexing it by every path it
// declares via MountPaths() and by its Name(). Intended to be called from a
// dialect package's init() function.
func Register(d core.Dialect) {
	mu.Lock()
	defer mu.Unlock()

	byName[d.Name()] = d
	for _, p := range d.MountPaths() {
		byMountPath[p] = d
	}
}

// ByName returns the dialect registered under the given name, and whether
// one was found.
func ByName(name string) (core.Dialect, bool) {
	mu.RLock()
	defer mu.RUnlock()

	d, ok := byName[name]
	return d, ok
}

// Names returns the names of all registered dialects.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	return names
}

// All returns all registered dialects.
func All() []core.Dialect {
	mu.RLock()
	defer mu.RUnlock()

	all := make([]core.Dialect, 0, len(byName))
	for _, d := range byName {
		all = append(all, d)
	}
	return all
}
