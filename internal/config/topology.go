package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/preset"
	"gopkg.in/yaml.v3"
)

// AccountStrategy defines how multiple accounts within a provider are selected.
type AccountStrategy string

const (
	StrategyRoundRobin       AccountStrategy = "round_robin"
	StrategyFillFirst        AccountStrategy = "fill_first"
	StrategySticky           AccountStrategy = "sticky"
	StrategyStickyRoundRobin AccountStrategy = "sticky_round_robin"
)

// Account is an individual credentialed account under a Provider.
type Account struct {
	Name       string            `yaml:"name"`
	Type       string            `yaml:"type,omitempty"` // "static" or "oauth_refresh"
	APIKey     string            `yaml:"api_key,omitempty"`
	Credential *CredentialConfig `yaml:"credential,omitempty"`
	Quota      *core.QuotaConfig `yaml:"quota,omitempty"`
}

// Combo defines a logical model panel combining multiple model endpoints.
type Combo struct {
	Name         string   `yaml:"name"`
	Members      []string `yaml:"members"`                // ordered list of "provider[(@account)]:model" or combo names
	Mode         string   `yaml:"mode,omitempty"`         // "ordered" (default), "pool", "fused"
	Capabilities []string `yaml:"capabilities,omitempty"` // e.g. ["vision", "pdf", "audio", "video"]
}

// Topology represents the runtime configuration loaded from config.yaml.
type Topology struct {
	Providers map[string]Provider `yaml:"providers"`
	Routes    []Route             `yaml:"routes"`
	Combos    []Combo             `yaml:"combos,omitempty"`
}

// CredentialConfig configures outbound provider credentials in config.yaml.
type CredentialConfig struct {
	Type          string                    `yaml:"type,omitempty"` // "static" or "oauth_refresh"
	APIKey        string                    `yaml:"api_key,omitempty"`
	RefreshToken  string                    `yaml:"refresh_token,omitempty"`
	ClientID      string                    `yaml:"client_id,omitempty"`
	ClientSecret  string                    `yaml:"client_secret,omitempty"`
	TokenEndpoint string                    `yaml:"token_endpoint,omitempty"`
	Profile       credential.RefreshProfile `yaml:"profile,omitempty"`
}

// Provider is a provider entry in config.yaml.
type Provider struct {
	Dialect     string             `yaml:"dialect"`
	BaseURL     string             `yaml:"base_url"`
	Transport   string             `yaml:"transport,omitempty"`
	APIKey      string             `yaml:"api_key,omitempty"`
	Credential  *CredentialConfig  `yaml:"credential,omitempty"`
	Accounts    []Account          `yaml:"accounts,omitempty"`
	Selection   AccountStrategy    `yaml:"selection,omitempty"`
	StickyLimit int                `yaml:"sticky_limit,omitempty"`
	Quota       *core.QuotaConfig  `yaml:"quota,omitempty"`
	Headers     map[string]*string `yaml:"headers,omitempty"` // null value means remove
	Cooldown429 string             `yaml:"cooldown_429,omitempty"`
	Cooldown5xx string             `yaml:"cooldown_5xx,omitempty"`
	Models      []string           `yaml:"models,omitempty"`
}

// Route maps an inbound surface + model glob to an ordered chain of hops.
type Route struct {
	From  string   `yaml:"from"`  // inbound dialect name (e.g. "anthropic", "openai")
	Match string   `yaml:"match"` // glob pattern against model name
	Chain []string `yaml:"chain"` // ordered hops as "provider:model" or "provider:$model"
}

var interpolateRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// ParseTopology parses and validates config.yaml content.
// It applies ${VAR} interpolation from the process environment.
func ParseTopology(data []byte) (Topology, error) {
	// Interpolate variables before JSON parsing
	interpolated, err := interpolateVars(string(data))
	if err != nil {
		return Topology{}, err
	}

	// YAML decoding (gopkg.in/yaml.v3). Unknown fields are intentionally
	// allowed since config.yaml may carry future fields we don't model yet.
	var t Topology
	if err := yaml.Unmarshal([]byte(interpolated), &t); err != nil {
		return Topology{}, fmt.Errorf("config.yaml: %w", err)
	}

	if t.Providers == nil {
		t.Providers = make(map[string]Provider)
	}

	migrateTopology(&t)

	return t, nil
}

// ParseRawTopology parses config.yaml content into Topology without variable interpolation.
// It is intended for inspection and editing commands (e.g. provider add, auth set)
// that preserve variable references like "${VAR}".
func ParseRawTopology(data []byte) (Topology, error) {
	var t Topology
	if err := yaml.Unmarshal(data, &t); err != nil {
		return Topology{}, fmt.Errorf("config.yaml: %w", err)
	}

	if t.Providers == nil {
		t.Providers = make(map[string]Provider)
	}

	migrateTopology(&t)

	return t, nil
}

func migrateTopology(t *Topology) {
	if t.Providers == nil {
		return
	}
	for name, p := range t.Providers {
		if !strings.EqualFold(name, "antigravity") {
			continue
		}
		if p.Transport == "" {
			p.Transport = "cloudcode"
			p.BaseURL = "https://daily-cloudcode-pa.googleapis.com"
		}
		// A hand-edited antigravity entry typically omits the credential block.
		// Synthesize OAuth-refresh metadata from the preset so BuildCredential
		// resolves the stored refresh token (under <provider>/default in the
		// credential store) instead of returning an empty key — otherwise the
		// cloudcode transport fails onboarding with "access token is required".
		if p.Credential == nil {
			if ag := preset.Get("antigravity"); ag != nil {
				p.Credential = &CredentialConfig{
					Type:          "oauth_refresh",
					ClientID:      ag.ClientID,
					ClientSecret:  ag.ClientSecret,
					TokenEndpoint: ag.TokenEndpoint,
					Profile:       ag.RefreshProfile,
				}
			}
		}
		t.Providers[name] = p
	}
}

// interpolateVars replaces ${VAR} references with their environment values.
// If a variable is unset in the environment, it evaluates to empty string ("").
func interpolateVars(s string) (string, error) {
	result := interpolateRe.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1] // strip ${ and }
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return ""
	})
	return result, nil
}

// BuildCredential builds a credential.Credential strategy for the default account of Provider.
func (p Provider) BuildCredential(providerName string, store *credential.Store) credential.Credential {
	return p.BuildAccountCredential(providerName, "default", store)
}

// BuildAccountCredential builds a credential.Credential strategy for a specific account of Provider.
func (p Provider) BuildAccountCredential(providerName string, accountName string, store *credential.Store) credential.Credential {
	if accountName == "" {
		accountName = "default"
	}
	for _, acc := range p.Accounts {
		if acc.Name == accountName {
			if acc.Credential != nil || acc.APIKey != "" || acc.Type != "" {
				cfg := acc.Credential
				if cfg == nil && acc.Type != "" {
					cfg = &CredentialConfig{Type: acc.Type, APIKey: acc.APIKey}
				}
				return buildCredentialFromConfig(providerName, acc.Name, cfg, acc.APIKey, store)
			}
		}
	}
	return buildCredentialFromConfig(providerName, accountName, p.Credential, p.APIKey, store)
}

// BuildCredentials builds all credential.Credential strategies configured for Provider,
// keyed by account name.
func (p Provider) BuildCredentials(providerName string, store *credential.Store) map[string]credential.Credential {
	res := make(map[string]credential.Credential)
	if len(p.Accounts) > 0 {
		for _, acc := range p.Accounts {
			accName := acc.Name
			if accName == "" {
				accName = "default"
			}
			cfg := acc.Credential
			if cfg == nil && acc.Type != "" {
				cfg = &CredentialConfig{Type: acc.Type, APIKey: acc.APIKey}
			}
			res[accName] = buildCredentialFromConfig(providerName, accName, cfg, acc.APIKey, store)
		}
	} else {
		res["default"] = p.BuildAccountCredential(providerName, "default", store)
	}
	return res
}

func buildCredentialFromConfig(providerName, accountName string, cfg *CredentialConfig, apiKey string, store *credential.Store) credential.Credential {
	storeKey := providerName
	if accountName != "" && accountName != "default" {
		storeKey = providerName + "/" + accountName
	}
	if cfg != nil {
		switch cfg.Type {
		case "static":
			return credential.NewStaticKey(cfg.APIKey)
		case "oauth_refresh":
			rt := cfg.RefreshToken
			at := ""
			exp := time.Time{}
			if store != nil {
				if rec, ok := store.Get(storeKey); ok {
					if rec.RefreshToken != "" {
						rt = rec.RefreshToken
					}
					at = rec.AccessToken
					exp = rec.ExpiresAt
				} else if rec, ok := store.Get(providerName); ok {
					if rec.RefreshToken != "" {
						rt = rec.RefreshToken
					}
					at = rec.AccessToken
					exp = rec.ExpiresAt
				}
			}
			return credential.NewOAuthRefreshable(credential.OAuthRefreshableConfig{
				Provider:      storeKey,
				RefreshToken:  rt,
				ClientID:      cfg.ClientID,
				ClientSecret:  cfg.ClientSecret,
				TokenEndpoint: cfg.TokenEndpoint,
				Profile:       cfg.Profile,
				AccessToken:   at,
				ExpiresAt:     exp,
				Store:         store,
			})
		}
	}
	if store != nil {
		var rec credential.OAuthRecord
		var ok bool
		if rec, ok = store.Get(storeKey); !ok {
			rec, ok = store.Get(providerName)
		}
		if ok && (rec.RefreshToken != "" || rec.AccessToken != "") {
			tokenEndpoint := rec.TokenEndpoint
			clientID := rec.ClientID
			clientSecret := rec.ClientSecret
			profile := rec.Profile
			if pre := preset.Get(providerName); pre != nil {
				if tokenEndpoint == "" {
					tokenEndpoint = pre.TokenEndpoint
				}
				if clientID == "" {
					clientID = pre.ClientID
				}
				if clientSecret == "" {
					clientSecret = pre.ClientSecret
				}
				if profile.BodyFormat == "" && pre.RefreshProfile.BodyFormat != "" {
					profile = pre.RefreshProfile
				}
			}
			return credential.NewOAuthRefreshable(credential.OAuthRefreshableConfig{
				Provider:            storeKey,
				RefreshToken:        rec.RefreshToken,
				ClientID:            clientID,
				ClientSecret:        clientSecret,
				TokenEndpoint:       tokenEndpoint,
				Profile:             profile,
				AccessToken:         rec.AccessToken,
				ExpiresAt:           rec.ExpiresAt,
				Store:               store,
				DeviceID:            rec.DeviceID,
				DeviceHeaderProfile: rec.DeviceHeaderProfile,
			})
		}
	}
	if apiKey != "" {
		return credential.NewStaticKey(apiKey)
	}
	return credential.NewStaticKey("")
}

// ValidateTopology checks the topology for configuration errors.
// Returns all errors found (does not stop at the first).
func ValidateTopology(t Topology, registeredDialects []string) []error {
	var errs []error
	dialectSet := make(map[string]bool)
	for _, d := range registeredDialects {
		dialectSet[d] = true
	}

	// Check providers
	for name, p := range t.Providers {
		if !dialectSet[p.Dialect] {
			errs = append(errs, fmt.Errorf("provider %q: unknown dialect %q (registered: %s)",
				name, p.Dialect, strings.Join(registeredDialects, ", ")))
		}
		if p.Transport != "" && p.Transport != "cloudcode" {
			errs = append(errs, fmt.Errorf("provider %q: unknown transport %q", name, p.Transport))
		}
		if p.Credential != nil {
			if p.Credential.Type != "static" && p.Credential.Type != "oauth_refresh" {
				errs = append(errs, fmt.Errorf("provider %q: unknown credential type %q", name, p.Credential.Type))
			}
		}
		if p.Selection != "" && p.Selection != StrategyRoundRobin && p.Selection != StrategyFillFirst && p.Selection != StrategySticky && p.Selection != StrategyStickyRoundRobin {
			errs = append(errs, fmt.Errorf("provider %q: unknown selection strategy %q", name, p.Selection))
		}
		accountNames := make(map[string]bool)
		for _, acc := range p.Accounts {
			if acc.Name == "" {
				errs = append(errs, fmt.Errorf("provider %q: account missing name", name))
				continue
			}
			if accountNames[acc.Name] {
				errs = append(errs, fmt.Errorf("provider %q: duplicate account name %q", name, acc.Name))
			}
			accountNames[acc.Name] = true
			if acc.Credential != nil && acc.Credential.Type != "static" && acc.Credential.Type != "oauth_refresh" {
				errs = append(errs, fmt.Errorf("provider %q account %q: unknown credential type %q", name, acc.Name, acc.Credential.Type))
			}
		}
	}

	// Index combo names
	comboSet := make(map[string]bool)
	for _, cb := range t.Combos {
		if cb.Name == "" {
			errs = append(errs, fmt.Errorf("combo missing name"))
			continue
		}
		if comboSet[cb.Name] {
			errs = append(errs, fmt.Errorf("duplicate combo name %q", cb.Name))
		}
		comboSet[cb.Name] = true
		if cb.Mode != "" && cb.Mode != "ordered" && cb.Mode != "pool" && cb.Mode != "fused" {
			errs = append(errs, fmt.Errorf("combo %q: unknown mode %q", cb.Name, cb.Mode))
		}
	}

	// Check routes
	for i, r := range t.Routes {
		if !dialectSet[r.From] {
			errs = append(errs, fmt.Errorf("route[%d]: unknown surface dialect %q", i, r.From))
		}
		for _, hop := range r.Chain {
			parts := strings.SplitN(hop, ":", 2)
			if len(parts) != 2 {
				errs = append(errs, fmt.Errorf("route[%d]: malformed chain hop %q (expected provider:model or combo)", i, hop))
				continue
			}
			provSpec := parts[0]
			provName := provSpec
			accName := ""
			if strings.Contains(provSpec, "@") {
				sub := strings.SplitN(provSpec, "@", 2)
				provName = sub[0]
				accName = sub[1]
			}
			if _, isCombo := comboSet[provSpec]; isCombo {
				continue
			}
			prov, ok := t.Providers[provName]
			if !ok {
				errs = append(errs, fmt.Errorf("route[%d]: chain references undeclared provider or combo %q", i, provSpec))
				continue
			}
			if accName != "" && accName != "default" {
				foundAcc := false
				for _, acc := range prov.Accounts {
					if acc.Name == accName {
						foundAcc = true
						break
					}
				}
				if !foundAcc {
					errs = append(errs, fmt.Errorf("route[%d]: chain references unknown account %q for provider %q", i, accName, provName))
				}
			}
			// Check surface/chain dialect mismatch
			if prov.Dialect != r.From {
				errs = append(errs, fmt.Errorf("route[%d]: surface %q requires translation to provider %q (dialect %q) which is unavailable",
					i, r.From, provName, prov.Dialect))
			}
		}
	}

	return errs
}

// WriteTopology atomically writes a topology to disk as YAML.
// Uses temp file + rename for atomicity, mode 0600.
func WriteTopology(path string, t Topology) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(t); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
