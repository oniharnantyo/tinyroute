// Package auth implements inbound API key management: generation, storage,
// verification, and expiry. Each key persists its plaintext secret (so
// it can be re-embedded into downstream client configs from the dashboard)
// alongside a sha256 digest used for verification on the request path. The keys
// file is written atomically with 0600 permissions; secrets must never be
// logged or returned in error responses.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// keyPrefix is prepended to every generated key so that secret scanners
// can recognize tinyroute credentials.
const keyPrefix = "tr_live_"

// keyRandomBytes is the number of cryptographically random bytes encoded
// into each generated key (before base64url encoding).
const keyRandomBytes = 32

// Key is a stored API key record. It holds the plaintext credential (Secret)
// so it can be re-embedded into downstream client configs, plus a sha256
// digest used for verification without re-exposing the secret on the hot path.
type Key struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Prefix   string     `json:"prefix"`           // first 4 chars after tr_live_, for display only
	Digest   string     `json:"digest"`           // sha256 hex digest of the full plaintext key
	Secret   string     `json:"secret,omitempty"` // plaintext credential, for re-embedding into client configs
	Created  time.Time  `json:"created"`
	Expires  *time.Time `json:"expires,omitempty"`
	Disabled bool       `json:"disabled,omitempty"`
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

// KeyOpt configures optional settings when generating a new key.
type KeyOpt func(*Key) error

// WithExpires sets the expiration time for the generated key.
// If expires is non-nil, it must be in the future.
func WithExpires(expires *time.Time) KeyOpt {
	return func(k *Key) error {
		if expires != nil {
			if !expires.After(time.Now()) {
				return fmt.Errorf("auth: expiry must be in the future")
			}
			k.Expires = expires
		}
		return nil
	}
}

// WithRate sets the rate limit spec for the generated key.
// If rate is non-nil, requests must be positive and interval must parse with time.ParseDuration.
func WithRate(rate *RateSpec) KeyOpt {
	return func(k *Key) error {
		if rate != nil {
			if err := validateRateSpec(*rate); err != nil {
				return err
			}
			k.Rate = rate
		}
		return nil
	}
}

func validateRateSpec(r RateSpec) error {
	if r.Requests <= 0 {
		return fmt.Errorf("auth: rate requests must be positive")
	}
	d, err := time.ParseDuration(r.Interval)
	if err != nil || d <= 0 {
		return fmt.Errorf("auth: invalid rate interval: %q", r.Interval)
	}
	return nil
}

// KeyUpdate specifies changes to apply to an existing key.
type KeyUpdate struct {
	Name         *string
	Expires      *time.Time
	ClearExpires bool
	Rate         *RateSpec
	ClearRate    bool
}

// GenerateKey creates a new key with the given display name and optional configurations.
// It returns the plaintext credential (to show the user once) along with the Key record to
// store in keys.json. The plaintext is also persisted in Key.Secret so the key
// can be re-embedded into downstream client configs from the dashboard.
func GenerateKey(name string, opts ...KeyOpt) (plaintext string, key Key, err error) {
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
		Secret:  plaintext,
		Created: time.Now().UTC(),
	}

	for _, opt := range opts {
		if err := opt(&key); err != nil {
			return "", Key{}, err
		}
	}

	return plaintext, key, nil
}

// UpdateKey loads the key file at filePath, applies name/expiry/rate changes to the key with id,
// and writes the updated file back atomically using WriteKeyFile. The secret and digest are left untouched.
func UpdateKey(filePath, id string, upd KeyUpdate) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("auth: read keys file: %w", err)
	}
	var kf KeyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return fmt.Errorf("auth: parse keys file: %w", err)
	}

	found := false
	for i := range kf.Keys {
		if kf.Keys[i].ID == id {
			found = true
			if upd.Name != nil {
				if strings.TrimSpace(*upd.Name) == "" {
					return fmt.Errorf("auth: key name cannot be empty")
				}
				kf.Keys[i].Name = *upd.Name
			}
			if upd.ClearExpires {
				kf.Keys[i].Expires = nil
			} else if upd.Expires != nil {
				if !upd.Expires.After(time.Now()) {
					return fmt.Errorf("auth: expiry must be in the future")
				}
				kf.Keys[i].Expires = upd.Expires
			}
			if upd.ClearRate {
				kf.Keys[i].Rate = nil
			} else if upd.Rate != nil {
				if err := validateRateSpec(*upd.Rate); err != nil {
					return err
				}
				kf.Keys[i].Rate = upd.Rate
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("auth: key %q not found", id)
	}

	return WriteKeyFile(filePath, kf)
}

// RevokeKey loads the key file at filePath, marks the key with id as Disabled=true,
// and writes the updated file back atomically using WriteKeyFile.
// Returns an error if the key does not exist or is already disabled.
func RevokeKey(filePath, id string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("auth: read keys file: %w", err)
	}
	var kf KeyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return fmt.Errorf("auth: parse keys file: %w", err)
	}

	found := false
	for i := range kf.Keys {
		if kf.Keys[i].ID == id {
			found = true
			if kf.Keys[i].Disabled {
				return fmt.Errorf("auth: key %q is already disabled", id)
			}
			kf.Keys[i].Disabled = true
			break
		}
	}
	if !found {
		return fmt.Errorf("auth: key %q not found", id)
	}

	return WriteKeyFile(filePath, kf)
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

// Verify checks a bearer credential against the stored keys.
// It returns the key's identifier on success.
//
// Verify never mutates the KeyStore and performs no I/O, so expiry is
// checked purely against the current time without requiring a file change.
func (ks *KeyStore) Verify(token string) (string, error) {
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

// LookupByToken checks if token matches any stored key's digest or secret.
func (ks *KeyStore) LookupByToken(token string) (Key, bool) {
	if ks == nil || token == "" {
		return Key{}, false
	}
	digest := digestOf(token)
	if k, ok := ks.byDigest[digest]; ok {
		return *k, true
	}
	for _, k := range ks.keys {
		if k.Secret == token {
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
