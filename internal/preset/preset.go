package preset

import (
	_ "embed"
	"encoding/json"
	"os"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/credential"
)

//go:embed presets.json
var presetsData []byte

// Preset represents a known provider template.
type Preset struct {
	Name                string                    `json:"name"`
	Dialect             string                    `json:"dialect"`
	BaseURL             string                    `json:"base_url"`
	Transport           string                    `json:"transport,omitempty"`
	CredentialVar       string                    `json:"credential_var"`
	AltDialect          string                    `json:"alt_dialect,omitempty"`
	AltBaseURL          string                    `json:"alt_base_url,omitempty"`
	OAuthCapable        bool                      `json:"oauth_capable,omitempty"`
	FlowType            string                    `json:"flow_type,omitempty"`
	ClientID            string                    `json:"client_id,omitempty"`
	ClientSecret        string                    `json:"client_secret,omitempty"`
	AuthorizeEndpoint   string                    `json:"authorize_endpoint,omitempty"`
	TokenEndpoint       string                    `json:"token_endpoint,omitempty"`
	DeviceEndpoint      string                    `json:"device_endpoint,omitempty"`
	Scopes              []string                  `json:"scopes,omitempty"`
	Models              []string                  `json:"models,omitempty"`
	CallbackHost        string                    `json:"callback_host,omitempty"`
	CallbackPath        string                    `json:"callback_path,omitempty"`
	ExtraParams         map[string]string         `json:"extra_params,omitempty"`
	DeviceHeaderProfile string                    `json:"device_header_profile,omitempty"`
	RefreshProfile      credential.RefreshProfile `json:"refresh_profile,omitempty"`
	RiskNotice          string                    `json:"risk_notice,omitempty"`
	DisplayName         string                    `json:"display_name,omitempty"`
	Logo                string                    `json:"logo,omitempty"`
	Category            string                    `json:"category,omitempty"`
	Tier                string                    `json:"tier,omitempty"`      // "free" or "freemium"
	FreeNote            string                    `json:"free_note,omitempty"` // e.g. "15 RPM, 1M tokens/day on flash"
}

type presetsFile struct {
	Presets []Preset `json:"presets"`
}

func All() []Preset {
	var pf presetsFile
	if err := json.Unmarshal(presetsData, &pf); err != nil {
		panic("preset: embedded presets.json is invalid: " + err.Error())
	}
	return pf.Presets
}

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

func Get(name string) *Preset {
	norm := normalizeName(name)
	if norm == "zen" {
		norm = "opencodezen"
	}
	for _, p := range All() {
		if p.Name == name || normalizeName(p.Name) == norm {
			cp := p
			if strings.HasPrefix(cp.ClientID, "YOUR_") {
				if val := os.Getenv(strings.TrimPrefix(cp.ClientID, "YOUR_")); val != "" {
					cp.ClientID = val
				}
			}
			if strings.HasPrefix(cp.ClientSecret, "YOUR_") {
				if val := os.Getenv(strings.TrimPrefix(cp.ClientSecret, "YOUR_")); val != "" {
					cp.ClientSecret = val
				}
			}
			return &cp
		}
	}
	return nil
}

func Names() []string {
	presets := All()
	names := make([]string, len(presets))
	for i, p := range presets {
		names[i] = p.Name
	}
	return names
}
