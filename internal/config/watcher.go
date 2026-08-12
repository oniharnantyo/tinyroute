package config

import (
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Watcher monitors a file by mtime and reloads it when changed.
// Parse must validate the content; if it returns an error, the previous
// snapshot continues to serve.
type Watcher[T any] struct {
	path    string
	parse   func([]byte) (T, error)
	current atomic.Pointer[T]
	mtime   time.Time
	mu      sync.Mutex
}

// NewWatcher creates a watcher for the given path. Initial load happens immediately.
// Returns an error only if the initial load fails AND there is no fallback.
func NewWatcher[T any](path string, parse func([]byte) (T, error)) (*Watcher[T], error) {
	w := &Watcher[T]{
		path:  path,
		parse: parse,
	}
	// Attempt initial load
	if err := w.reload(); err != nil {
		// If the file doesn't exist yet, store zero value
		var zero T
		w.current.Store(&zero)
		// Only return error if file exists but is unparseable
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return w, nil
}

// Get returns the current snapshot. It checks mtime and reloads if needed.
// The hot path (no change) is lock-free.
func (w *Watcher[T]) Get() *T {
	w.maybeReload()
	return w.current.Load()
}

func (w *Watcher[T]) maybeReload() {
	info, err := os.Stat(w.path)
	if err != nil {
		return // file missing or unreadable; keep current
	}
	if !info.ModTime().After(w.mtime) {
		return // no change
	}
	// mtime changed, take the lock and reload
	w.mu.Lock()
	defer w.mu.Unlock()
	// Double-check under lock
	info2, err := os.Stat(w.path)
	if err != nil || !info2.ModTime().After(w.mtime) {
		return
	}
	_ = w.reload()
}

func (w *Watcher[T]) reload() error {
	data, err := os.ReadFile(w.path)
	if err != nil {
		log.Printf("config: failed to read %s: %v", w.path, err)
		return err
	}

	parsed, err := w.parse(data)
	if err != nil {
		log.Printf("config: failed to parse %s: %v (keeping previous snapshot)", w.path, err)
		return err
	}

	w.current.Store(&parsed)
	// Update mtime AFTER successful store
	if info, e := os.Stat(w.path); e == nil {
		w.mtime = info.ModTime()
	}
	return nil
}
