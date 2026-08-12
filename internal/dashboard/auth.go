package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

const (
	SessionCookieName = "tinyroute_session"
	DefaultPassword   = "123456"
)

type PasswordStore struct {
	mu         sync.RWMutex
	path       string
	BcryptHash string    `json:"bcrypt_hash"`
	UpdatedAt  time.Time `json:"updated_at"`
	IsDefault  bool      `json:"is_default"`
}

type authFile struct {
	BcryptHash string    `json:"bcrypt_hash"`
	UpdatedAt  time.Time `json:"updated_at"`
	IsDefault  bool      `json:"is_default"`
}

func NewPasswordStore(path string) (*PasswordStore, error) {
	ps := &PasswordStore{path: path}
	if err := ps.loadOrSeed(); err != nil {
		return nil, err
	}
	return ps, nil
}

func (ps *PasswordStore) loadOrSeed() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	data, err := os.ReadFile(ps.path)
	if err == nil {
		var af authFile
		if err := json.Unmarshal(data, &af); err == nil && af.BcryptHash != "" {
			ps.BcryptHash = af.BcryptHash
			ps.UpdatedAt = af.UpdatedAt
			ps.IsDefault = af.IsDefault
			return nil
		}
	}

	// Seed default password hash
	hash, err := bcrypt.GenerateFromPassword([]byte(DefaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("generate default password hash: %w", err)
	}

	ps.BcryptHash = string(hash)
	ps.UpdatedAt = time.Now()
	ps.IsDefault = true

	return ps.saveLocked()
}

func (ps *PasswordStore) VerifyPassword(password string) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	err := bcrypt.CompareHashAndPassword([]byte(ps.BcryptHash), []byte(password))
	return err == nil
}

func (ps *PasswordStore) SetPassword(newPassword string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	ps.BcryptHash = string(hash)
	ps.UpdatedAt = time.Now()
	ps.IsDefault = false

	return ps.saveLocked()
}

func (ps *PasswordStore) saveLocked() error {
	dir := filepath.Dir(ps.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	af := authFile{
		BcryptHash: ps.BcryptHash,
		UpdatedAt:  ps.UpdatedAt,
		IsDefault:  ps.IsDefault,
	}
	data, err := json.MarshalIndent(af, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth file: %w", err)
	}

	tmpFile := ps.path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("write temp auth file: %w", err)
	}

	if err := os.Rename(tmpFile, ps.path); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename auth file: %w", err)
	}

	return nil
}

func (ps *PasswordStore) IsDefaultPassword() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.IsDefault
}

// SessionStore handles session tokens.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]time.Time // token -> expiry
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]time.Time),
	}
}

func (ss *SessionStore) CreateSession(ttl time.Duration) string {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	ss.sessions[token] = time.Now().Add(ttl)
	return token
}

func (ss *SessionStore) ValidateSession(token string) bool {
	if token == "" {
		return false
	}
	ss.mu.RLock()
	expiry, exists := ss.sessions[token]
	ss.mu.RUnlock()

	if !exists {
		return false
	}
	if time.Now().After(expiry) {
		ss.mu.Lock()
		delete(ss.sessions, token)
		ss.mu.Unlock()
		return false
	}
	return true
}

func (ss *SessionStore) RevokeSession(token string) {
	if token == "" {
		return
	}
	ss.mu.Lock()
	delete(ss.sessions, token)
	ss.mu.Unlock()
}

// Guard logic for Host/Origin header checking on mutating requests
func IsLoopbackHost(hostHeader string) bool {
	host := hostHeader
	if h, _, err := net.SplitHostPort(hostHeader); err == nil {
		host = h
	}

	host = strings.Trim(host, "[]")
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func HostGuardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete || r.Method == http.MethodPatch {
			host := r.Host
			if !IsLoopbackHost(host) {
				http.Error(w, "Forbidden: Host must be loopback", http.StatusForbidden)
				return
			}

			origin := r.Header.Get("Origin")
			if origin != "" {
				// Strip protocol
				originHost := origin
				if idx := strings.Index(origin, "://"); idx != -1 {
					originHost = origin[idx+3:]
				}
				if !IsLoopbackHost(originHost) {
					http.Error(w, "Forbidden: Origin must be loopback", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// LoginLimiter wraps auth.RateLimiter for login attempts.
type LoginLimiter struct {
	limiter *auth.RateLimiter
}

func NewLoginLimiter() *LoginLimiter {
	limiter := auth.NewRateLimiter(func(clientIP string) *auth.RateSpec {
		return &auth.RateSpec{
			Requests: 5,
			Interval: "1m",
		}
	})
	return &LoginLimiter{limiter: limiter}
}

func (ll *LoginLimiter) Allow(clientIP string) (bool, time.Duration) {
	return ll.limiter.Allow(clientIP)
}
