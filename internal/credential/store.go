package credential

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// OAuthRecord stores an OAuth credential entry on disk in credentials.json.
type OAuthRecord struct {
	Provider            string         `json:"provider"`
	Account             string         `json:"account,omitempty"`
	RefreshToken        string         `json:"refresh_token"`
	AccessToken         string         `json:"access_token,omitempty"`
	ExpiresAt           time.Time      `json:"expires_at,omitempty"`
	ClientID            string         `json:"client_id,omitempty"`
	ClientSecret        string         `json:"client_secret,omitempty"`
	TokenEndpoint       string         `json:"token_endpoint,omitempty"`
	Profile             RefreshProfile `json:"profile,omitempty"`
	Scopes              []string       `json:"scopes,omitempty"`
	DeviceID            string         `json:"device_id,omitempty"`
	DeviceHeaderProfile string         `json:"device_header_profile,omitempty"`
	UpdatedAt           time.Time      `json:"updated_at,omitempty"`
	IdentityHint        string         `json:"-"`
}

// CredentialsFile is the JSON root structure for credentials.json.
type CredentialsFile struct {
	Credentials map[string]OAuthRecord `json:"credentials"`
}

// ParseCredentialsFile parses credentials.json into a CredentialsFile structure,
// performing automatic migration of legacy provider-keyed records to provider/default.
func ParseCredentialsFile(data []byte) (CredentialsFile, error) {
	var cf CredentialsFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return CredentialsFile{}, fmt.Errorf("credential: parse credentials.json: %w", err)
	}
	if cf.Credentials == nil {
		cf.Credentials = make(map[string]OAuthRecord)
	}
	migrated := make(map[string]OAuthRecord, len(cf.Credentials))
	for k, rec := range cf.Credentials {
		if rec.Provider == "" {
			if idx := filepath.Base(k); idx != "" {
				if parts := splitKey(k); len(parts) == 2 {
					rec.Provider = parts[0]
					rec.Account = parts[1]
				} else {
					rec.Provider = k
				}
			}
		}
		if rec.Account == "" {
			if parts := splitKey(k); len(parts) == 2 {
				rec.Provider = parts[0]
				rec.Account = parts[1]
			} else {
				rec.Account = "default"
			}
		}
		targetKey := rec.Provider + "/" + rec.Account
		migrated[targetKey] = rec
	}
	cf.Credentials = migrated
	return cf, nil
}

func splitKey(k string) []string {
	for i := 0; i < len(k); i++ {
		if k[i] == '/' {
			return []string{k[:i], k[i+1:]}
		}
	}
	return []string{k}
}

type fileWatcher[T any] struct {
	path    string
	parse   func([]byte) (T, error)
	current atomic.Pointer[T]
	mtime   time.Time
	mu      sync.Mutex
}

func newFileWatcher[T any](path string, parse func([]byte) (T, error)) (*fileWatcher[T], error) {
	w := &fileWatcher[T]{
		path:  path,
		parse: parse,
	}
	if err := w.reload(); err != nil {
		var zero T
		w.current.Store(&zero)
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return w, nil
}

func (w *fileWatcher[T]) Get() *T {
	w.maybeReload()
	return w.current.Load()
}

func (w *fileWatcher[T]) maybeReload() {
	info, err := os.Stat(w.path)
	if err != nil {
		return
	}
	if info.ModTime().Equal(w.mtime) {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	info2, err := os.Stat(w.path)
	if err != nil || info2.ModTime().Equal(w.mtime) {
		return
	}
	_ = w.reload()
}

func (w *fileWatcher[T]) reload() error {
	data, err := os.ReadFile(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			var zero T
			w.current.Store(&zero)
			return nil
		}
		log.Printf("config: failed to read %s: %v", w.path, err)
		return err
	}
	parsed, err := w.parse(data)
	if err != nil {
		log.Printf("config: failed to parse %s: %v (keeping previous snapshot)", w.path, err)
		return err
	}
	w.current.Store(&parsed)
	if info, e := os.Stat(w.path); e == nil {
		w.mtime = info.ModTime()
	}
	return nil
}

// Store manages custodian storage for credentials.json with hot-reloading and mode 0600.
type Store struct {
	filePath string
	mu       sync.Mutex
	watcher  *fileWatcher[CredentialsFile]
}

// NewStore constructs a Store for filePath using fileWatcher for hot-reloading.
func NewStore(filePath string) (*Store, error) {
	w, err := newFileWatcher(filePath, ParseCredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("credential: new watcher: %w", err)
	}
	s := &Store{
		filePath: filePath,
		watcher:  w,
	}
	if cf := w.Get(); cf != nil && len(cf.Credentials) > 0 {
		_ = writeCredentialsFile(filePath, *cf)
	}
	return s, nil
}

// Get returns the stored OAuthRecord for the provider, if present.
// Get returns the stored OAuthRecord for key ("provider/account" or "provider"), if present.
func (s *Store) Get(key string) (OAuthRecord, bool) {
	cf := s.watcher.Get()
	if cf == nil || cf.Credentials == nil {
		return OAuthRecord{}, false
	}
	var rec OAuthRecord
	var ok bool
	if rec, ok = cf.Credentials[key]; !ok {
		if !strings.Contains(key, "/") {
			rec, ok = cf.Credentials[key+"/default"]
		}
	}
	if ok && (rec.Provider == "cline" || rec.Provider == "clinepass") && rec.AccessToken != "" && !strings.HasPrefix(rec.AccessToken, "workos:") {
		rec.AccessToken = "workos:" + rec.AccessToken
	}
	return rec, ok
}

// GetAccount returns the stored OAuthRecord for provider + account.
func (s *Store) GetAccount(provider, account string) (OAuthRecord, bool) {
	if account == "" {
		account = "default"
	}
	return s.Get(provider + "/" + account)
}

// Masked returns a copy of OAuthRecord with sensitive credentials (RefreshToken, AccessToken, ClientSecret) masked.
func (rec OAuthRecord) Masked() OAuthRecord {
	cp := rec
	if cp.RefreshToken != "" {
		cp.RefreshToken = MaskSecretToken(cp.RefreshToken)
	}
	if cp.AccessToken != "" {
		cp.AccessToken = MaskSecretToken(cp.AccessToken)
	}
	if cp.ClientSecret != "" {
		cp.ClientSecret = MaskSecretToken(cp.ClientSecret)
	}
	return cp
}

// MaskSecretToken masks a credential or token for safe display.
func MaskSecretToken(s string) string {
	if len(s) <= 8 {
		return "******"
	}
	return s[:4] + "..." + s[len(s)-4:]
}

// List returns all stored records containing raw plaintext credentials.
// WARNING: The returned OAuthRecord structures contain sensitive plaintext secrets.
// Callers MUST NOT print or log these records directly; use ListMasked() or rec.Masked()
// before any stdout/CLI rendering to prevent credential leakage.
func (s *Store) List() []OAuthRecord {
	cf := s.watcher.Get()
	if cf == nil || cf.Credentials == nil {
		return nil
	}
	records := make([]OAuthRecord, 0, len(cf.Credentials))
	for _, rec := range cf.Credentials {
		records = append(records, rec)
	}
	return records
}

// ListMasked returns all stored records with secrets (RefreshToken, AccessToken, ClientSecret) masked.
// Safe for CLI display or reporting.
func (s *Store) ListMasked() []OAuthRecord {
	records := s.List()
	masked := make([]OAuthRecord, len(records))
	for i, rec := range records {
		masked[i] = rec.Masked()
	}
	return masked
}

// Save atomically writes record to credentials.json with mode 0600 via tmp+rename.
func (s *Store) Save(record OAuthRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record.Account == "" {
		if parts := splitKey(record.Provider); len(parts) == 2 {
			record.Provider = parts[0]
			record.Account = parts[1]
		} else {
			record.Account = "default"
		}
	}

	key := record.Provider + "/" + record.Account

	var cf CredentialsFile
	if current := s.watcher.Get(); current != nil && current.Credentials != nil {
		cf.Credentials = make(map[string]OAuthRecord, len(current.Credentials)+1)
		for k, v := range current.Credentials {
			cf.Credentials[k] = v
		}
	} else {
		cf.Credentials = make(map[string]OAuthRecord)
	}

	record.UpdatedAt = time.Now().UTC()
	cf.Credentials[key] = record

	err := writeCredentialsFile(s.filePath, cf)
	if err == nil {
		_ = s.watcher.reload()
	}
	return err
}

// Delete removes provider record and atomically rewrites credentials.json.
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	targetKey := key
	targetDefault := key + "/default"

	var cf CredentialsFile
	if current := s.watcher.Get(); current != nil && current.Credentials != nil {
		cf.Credentials = make(map[string]OAuthRecord, len(current.Credentials))
		for k, v := range current.Credentials {
			if k != targetKey && k != targetDefault {
				cf.Credentials[k] = v
			}
		}
	} else {
		cf.Credentials = make(map[string]OAuthRecord)
	}

	err := writeCredentialsFile(s.filePath, cf)
	if err == nil {
		_ = s.watcher.reload()
	}
	return err
}

// DeleteProvider removes all credential records for the specified provider and atomically rewrites credentials.json.
func (s *Store) DeleteProvider(provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	provLower := strings.ToLower(provider)
	var cf CredentialsFile
	if current := s.watcher.Get(); current != nil && current.Credentials != nil {
		cf.Credentials = make(map[string]OAuthRecord, len(current.Credentials))
		for k, v := range current.Credentials {
			recProv := strings.ToLower(v.Provider)
			if recProv == "" {
				parts := splitKey(k)
				recProv = strings.ToLower(parts[0])
			}
			if recProv != provLower {
				cf.Credentials[k] = v
			}
		}
	} else {
		cf.Credentials = make(map[string]OAuthRecord)
	}

	err := writeCredentialsFile(s.filePath, cf)
	if err == nil {
		_ = s.watcher.reload()
	}
	return err
}

func writeCredentialsFile(filePath string, cf CredentialsFile) error {
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("credential: marshal credentials.json: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("credential: create dir: %w", err)
	}

	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("credential: write temp file: %w", err)
	}
	_ = os.Chmod(tmp, 0o600)

	if err := os.Rename(tmp, filePath); err != nil {
		return fmt.Errorf("credential: rename temp file: %w", err)
	}
	return nil
}
