// Package auth implements inbound API key management: generation, hashed
// storage, verification, scoping, and expiry. Keys are never stored in
// plaintext — only a sha256 digest and a short display prefix are persisted.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"time"
)

// keyPrefix is prepended to every generated key so that secret scanners
// can recognize tinyroute credentials.
const keyPrefix = "tr_live_"

// keyRandomBytes is the number of cryptographically random bytes encoded
// into each generated key (before base64url encoding).
const keyRandomBytes = 32

// Key is a stored API key record. It never holds the plaintext credential —
// only a sha256 digest and a short, non-secret display prefix.
type Key struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Prefix   string     `json:"prefix"` // first 4 chars after tr_live_, for display only
	Digest   string     `json:"digest"` // sha256 hex digest of the full plaintext key
	Created  time.Time  `json:"created"`
	Expires  *time.Time `json:"expires,omitempty"`
	Disabled bool       `json:"disabled,omitempty"`
	// Allow is a list of "surface:model-glob" patterns. When non-empty, a
	// request is permitted only if it matches at least one pattern. When
	// empty, the key permits any configured route.
	Allow []string `json:"allow,omitempty"`
	// Rate is an optional request-rate limit for this key. The keystore
	// itself does not enforce it; it is data consulted by the rate limiter.
	Rate *RateSpec `json:"rate,omitempty"`
}

// RateSpec describes a request-count-per-interval limit for a key.
type RateSpec struct {
	Requests int    `json:"requests"`
	Interval string `json:"interval"` // e.g. "1m", "1h", parsed with time.ParseDuration
}

// KeyFile is the on-disk structure of keys.json.
type KeyFile struct {
	Keys []Key `json:"keys"`
}

// GenerateKey creates a new key with the given display name. It returns the
// plaintext credential (which the caller MUST show to the user exactly once
// and never persist) along with the Key record to store in keys.json.
func GenerateKey(name string) (plaintext string, key Key, err error) {
	raw := make([]byte, keyRandomBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", Key{}, fmt.Errorf("auth: generate key: %w", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(raw)
	plaintext = keyPrefix + encoded

	digest := digestOf(plaintext)
	id := "k_" + digest[:8]

	prefix := encoded
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}

	key = Key{
		ID:      id,
		Name:    name,
		Prefix:  prefix,
		Digest:  digest,
		Created: time.Now().UTC(),
	}
	return plaintext, key, nil
}

// digestOf returns the sha256 hex digest of s.
func digestOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// KeyStore is an immutable, parsed snapshot of keys.json used for
// verification. A new KeyStore is produced on every reload; callers should
// treat values as read-only.
type KeyStore struct {
	keys     []Key
	byDigest map[string]*Key
}

// ParseKeyFile parses keys.json content into a KeyStore. This is the parse
// function to hand to config.Watcher for hot-reload:
//
//	w, err := config.NewWatcher(path, auth.ParseKeyFile)
func ParseKeyFile(data []byte) (KeyStore, error) {
	var kf KeyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return KeyStore{}, fmt.Errorf("auth: parse keys.json: %w", err)
	}

	store := KeyStore{
		keys:     kf.Keys,
		byDigest: make(map[string]*Key, len(kf.Keys)),
	}
	for i := range store.keys {
		store.byDigest[store.keys[i].Digest] = &store.keys[i]
	}
	return store, nil
}

// Verify checks a bearer credential against the stored keys and, if valid,
// that it is scoped to permit the given surface and model. It returns the
// key's identifier on success.
//
// Verify never mutates the KeyStore and performs no I/O, so expiry is
// checked purely against the current time without requiring a file change.
func (ks *KeyStore) Verify(token string, surface string, model string) (string, error) {
	if ks == nil {
		return "", fmt.Errorf("authentication required")
	}
	if token == "" {
		return "", fmt.Errorf("authentication required")
	}

	digest := digestOf(token)
	key, ok := ks.byDigest[digest]
	if !ok {
		return "", fmt.Errorf("invalid API key")
	}

	if key.Disabled {
		return "", fmt.Errorf("API key %q is disabled", key.ID)
	}

	if key.Expires != nil && time.Now().After(*key.Expires) {
		return "", fmt.Errorf("API key %q has expired", key.ID)
	}

	if len(key.Allow) > 0 && !matchesScope(key.Allow, surface, model) {
		return "", fmt.Errorf("API key %q does not permit %s:%s", key.ID, surface, model)
	}

	return key.ID, nil
}

// Lookup returns the key with the given identifier, if present.
func (ks *KeyStore) Lookup(id string) (Key, bool) {
	if ks == nil {
		return Key{}, false
	}
	for _, k := range ks.keys {
		if k.ID == id {
			return k, true
		}
	}
	return Key{}, false
}

// Keys returns all stored keys, e.g. for `tinyroute keys list`. The
// returned slice must not be mutated.
func (ks *KeyStore) Keys() []Key {
	if ks == nil {
		return nil
	}
	return ks.keys
}

// matchesScope reports whether surface and model match at least one
// "surface:model-glob" pattern in allow. Surface matches exactly or via "*".
// Model matches via path.Match glob semantics.
func matchesScope(allow []string, surface, model string) bool {
	for _, pattern := range allow {
		pSurface, pModel, ok := strings.Cut(pattern, ":")
		if !ok {
			continue
		}

		if pSurface != "*" && pSurface != surface {
			continue
		}

		if matched, _ := path.Match(pModel, model); matched {
			return true
		}
	}
	return false
}

// WriteKeyFile atomically writes kf to filePath using canonical
// marshalling (2-space indent) so that diffs stay readable. It must be
// called only in response to a key mutation (create, revoke, disable,
// update) — never on the request path — so that keys.json's modification
// time changes only when keys actually change, which is what the
// hot-reload watcher relies on.
func WriteKeyFile(filePath string, kf KeyFile) error {
	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return fmt.Errorf("auth: marshal keys.json: %w", err)
	}
	data = append(data, '\n')

	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("auth: write temp key file: %w", err)
	}
	if err := os.Rename(tmp, filePath); err != nil {
		return fmt.Errorf("auth: rename temp key file: %w", err)
	}
	return nil
}
