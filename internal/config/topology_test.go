package config

import (
	"testing"
)

func TestTopologyValidationAndAccounts(t *testing.T) {
	rawYAML := `providers:
  openai:
    dialect: openai
    base_url: https://api.openai.com/v1
    selection: round_robin
    accounts:
    - name: acc1
      api_key: sk-1
    - name: acc2
      api_key: sk-2
combos:
- name: fast-panel
  members:
  - openai@acc1:gpt-4o
  - openai@acc2:gpt-4o-mini
  mode: pool
  capabilities:
  - vision
routes:
- from: openai
  match: '*'
  chain:
  - fast-panel:gpt-4o`

	topo, err := ParseTopology([]byte(rawYAML))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	registered := []string{"openai", "anthropic"}
	errs := ValidateTopology(topo, registered)
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	prov := topo.Providers["openai"]
	creds := prov.BuildCredentials("openai", nil)
	if len(creds) != 2 {
		t.Errorf("expected 2 credentials, got %d", len(creds))
	}
	if creds["acc1"] == nil || creds["acc2"] == nil {
		t.Errorf("expected acc1 and acc2 credentials, got map %v", creds)
	}
}

func TestTopologyValidation_DuplicateAccountAndUnknownComboMode(t *testing.T) {
	rawYAML := `providers:
  openai:
    dialect: openai
    base_url: https://api.openai.com/v1
    selection: invalid_strat
    accounts:
    - name: acc1
      api_key: sk-1
    - name: acc1
      api_key: sk-2
combos:
- name: bad-combo
  members:
  - openai:gpt-4o
  mode: unknown_mode`

	topo, err := ParseTopology([]byte(rawYAML))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	errs := ValidateTopology(topo, []string{"openai"})
	if len(errs) < 3 {
		t.Errorf("expected at least 3 validation errors for invalid strategy, duplicate account, and invalid combo mode, got %d: %v", len(errs), errs)
	}
}

func TestInterpolateVars(t *testing.T) {
	t.Setenv("TEST_SET_VAR", "secret_value")

	rawYAML := `providers:
  set_prov:
    dialect: openai
    base_url: https://api.example.com/v1
    api_key: ${TEST_SET_VAR}
  unset_prov:
    dialect: openai
    base_url: https://api.example.com/v1
    api_key: ${UNSET_ENV_VAR_TEST}`

	topo, err := ParseTopology([]byte(rawYAML))
	if err != nil {
		t.Fatalf("unexpected parse error when parsing topology with unset env var: %v", err)
	}

	if topo.Providers["set_prov"].APIKey != "secret_value" {
		t.Errorf("expected interpolated API key 'secret_value', got %q", topo.Providers["set_prov"].APIKey)
	}
	if topo.Providers["unset_prov"].APIKey != "" {
		t.Errorf("expected unset env var to interpolate as empty string, got %q", topo.Providers["unset_prov"].APIKey)
	}
}

func TestAntigravityConfigMigration(t *testing.T) {
	rawYAML := `providers:
  antigravity:
    dialect: gemini
    base_url: https://generativelanguage.googleapis.com`

	topo, err := ParseTopology([]byte(rawYAML))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	p, ok := topo.Providers["antigravity"]
	if !ok {
		t.Fatalf("expected antigravity provider")
	}

	if p.Transport != "cloudcode" {
		t.Errorf("expected migrated transport 'cloudcode', got %q", p.Transport)
	}
	if p.BaseURL != "https://daily-cloudcode-pa.googleapis.com" {
		t.Errorf("expected migrated BaseURL 'https://daily-cloudcode-pa.googleapis.com', got %q", p.BaseURL)
	}
	// A hand-edited antigravity entry omits the credential block; the migration
	// must synthesize OAuth-refresh metadata from the preset so BuildCredential
	// picks up the stored refresh token instead of returning an empty key.
	if p.Credential == nil {
		t.Fatal("expected migrated credential block for antigravity, got nil")
	}
	if p.Credential.Type != "oauth_refresh" {
		t.Errorf("expected credential type oauth_refresh, got %q", p.Credential.Type)
	}
	if p.Credential.ClientID == "" || p.Credential.ClientSecret == "" || p.Credential.TokenEndpoint == "" {
		t.Errorf("expected OAuth metadata synthesized from preset, got clientID=%q tokenEndpoint=%q",
			p.Credential.ClientID, p.Credential.TokenEndpoint)
	}
}

func TestAntigravityConfigMigrationPreservesExistingCredential(t *testing.T) {
	rawYAML := `providers:
  antigravity:
    dialect: gemini
    base_url: https://daily-cloudcode-pa.googleapis.com
    transport: cloudcode
    credential:
      type: static
      api_key: user-supplied`

	topo, err := ParseTopology([]byte(rawYAML))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	p, ok := topo.Providers["antigravity"]
	if !ok {
		t.Fatalf("expected antigravity provider")
	}
	if p.Credential == nil || p.Credential.Type != "static" || p.Credential.APIKey != "user-supplied" {
		t.Errorf("migration overwrote a user-supplied credential block: %+v", p.Credential)
	}
}
