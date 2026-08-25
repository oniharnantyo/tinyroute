package config

import (
	"os"
	"strings"
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

func TestProvider_UpsertAccount(t *testing.T) {
	p := Provider{
		Dialect: "openai",
		BaseURL: "https://api.openai.com/v1",
		Accounts: []Account{
			{Name: "work", Type: "static", APIKey: "sk-1"},
			{Name: "personal", Type: "static", APIKey: "sk-2"},
		},
	}

	// 1. Replace existing account by name
	updated := p.UpsertAccount(Account{Name: "work", Type: "static", APIKey: "sk-updated"})
	if len(updated.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(updated.Accounts))
	}
	if updated.Accounts[0].APIKey != "sk-updated" || updated.Accounts[0].Name != "work" {
		t.Errorf("expected first account updated, got %+v", updated.Accounts[0])
	}
	if updated.Accounts[1].APIKey != "sk-2" || updated.Accounts[1].Name != "personal" {
		t.Errorf("expected second account untouched, got %+v", updated.Accounts[1])
	}

	// 2. Append new account
	appended := updated.UpsertAccount(Account{Name: "backup", Type: "oauth_refresh"})
	if len(appended.Accounts) != 3 {
		t.Fatalf("expected 3 accounts, got %d", len(appended.Accounts))
	}
	if appended.Accounts[2].Name != "backup" || appended.Accounts[2].Type != "oauth_refresh" {
		t.Errorf("expected third account appended, got %+v", appended.Accounts[2])
	}
}

func TestValidateTopology_ComboMembers(t *testing.T) {
	validTopo := Topology{
		Providers: map[string]Provider{
			"glm": {
				Dialect: "openai",
				BaseURL: "https://api.zhipu.ai",
				Accounts: []Account{
					{Name: "work"},
					{Name: "personal"},
				},
			},
			"anthropic": {
				Dialect: "anthropic",
				BaseURL: "https://api.anthropic.com",
			},
		},
		Combos: []Combo{
			{
				Name: "subcombo",
				Members: []string{
					"glm@work:glm-4.7",
					"anthropic:claude-3-5-sonnet",
				},
			},
			{
				Name: "maincombo",
				Members: []string{
					"subcombo",
					"glm@personal:glm-4.7",
					"glm@default:glm-4.7",
				},
			},
		},
	}

	registeredDialects := []string{"openai", "anthropic"}
	errs := ValidateTopology(validTopo, registeredDialects)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for valid combos, got: %v", errs)
	}

	// 1. Unknown provider
	badProviderTopo := validTopo
	badProviderTopo.Combos = []Combo{
		{
			Name:    "bad-prov",
			Members: []string{"nosuchprov:some-model", "anthropic:claude-3-5-sonnet"},
		},
	}
	errs = ValidateTopology(badProviderTopo, registeredDialects)
	if len(errs) == 0 {
		t.Fatalf("expected error for undeclared provider")
	}
	foundBadProv := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "references undeclared provider \"nosuchprov\"") {
			foundBadProv = true
			break
		}
	}
	if !foundBadProv {
		t.Errorf("expected undeclared provider error copy, got: %v", errs)
	}

	// 2. Unknown account
	badAccountTopo := validTopo
	badAccountTopo.Combos = []Combo{
		{
			Name:    "bad-acc",
			Members: []string{"glm@ghost:glm-4.7", "anthropic:claude-3-5-sonnet"},
		},
	}
	errs = ValidateTopology(badAccountTopo, registeredDialects)
	if len(errs) == 0 {
		t.Fatalf("expected error for unknown account")
	}
	foundBadAcc := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "references unknown account \"ghost\" for provider \"glm\"") {
			foundBadAcc = true
			break
		}
	}
	if !foundBadAcc {
		t.Errorf("expected unknown account error copy naming combo/provider/account, got: %v", errs)
	}

	// 3. Malformed member
	malformedTopo := validTopo
	malformedTopo.Combos = []Combo{
		{
			Name:    "malformed-member",
			Members: []string{"not-a-colon-or-combo", "anthropic:claude-3-5-sonnet"},
		},
	}
	errs = ValidateTopology(malformedTopo, registeredDialects)
	if len(errs) == 0 {
		t.Fatalf("expected error for malformed member")
	}
}

func TestCombo_DisabledFieldAndPolarity(t *testing.T) {
	// 1. Absent flag defaults to enabled
	rawYAML := `combos:
- name: enabled-by-default
  members:
  - openai:gpt-4o
`
	topo, err := ParseTopology([]byte(rawYAML))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(topo.Combos) != 1 {
		t.Fatalf("expected 1 combo, got %d", len(topo.Combos))
	}
	cb := topo.Combos[0]
	if cb.Disabled {
		t.Errorf("expected combo Disabled to be false, got true")
	}
	if !cb.IsEnabled() {
		t.Errorf("expected combo IsEnabled() to be true")
	}

	// 2. disabled: true parses correctly
	rawDisabledYAML := `combos:
- name: disabled-combo
  members:
  - openai:gpt-4o
  disabled: true
`
	topoDisabled, err := ParseTopology([]byte(rawDisabledYAML))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	cbDisabled := topoDisabled.Combos[0]
	if !cbDisabled.Disabled {
		t.Errorf("expected combo Disabled to be true")
	}
	if cbDisabled.IsEnabled() {
		t.Errorf("expected combo IsEnabled() to be false")
	}

	// 3. Round-trip through write/read
	rawTopo, err := ParseRawTopology([]byte(rawDisabledYAML))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(rawTopo.Combos) != 1 || !rawTopo.Combos[0].Disabled {
		t.Fatalf("expected 1 disabled raw combo")
	}

	tmpFile := t.TempDir() + "/config.yaml"
	if err := WriteTopology(tmpFile, rawTopo); err != nil {
		t.Fatalf("failed to write topology: %v", err)
	}
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read tmpFile: %v", err)
	}
	loadedTopo, err := ParseTopology(data)
	if err != nil {
		t.Fatalf("failed to parse loaded topology: %v", err)
	}
	if len(loadedTopo.Combos) != 1 {
		t.Fatalf("expected 1 combo after reload, got %d", len(loadedTopo.Combos))
	}
	if !loadedTopo.Combos[0].Disabled || loadedTopo.Combos[0].IsEnabled() {
		t.Errorf("expected loaded combo to remain disabled: %+v", loadedTopo.Combos[0])
	}
}

func TestCheckDeprecated(t *testing.T) {
	// 1. Raw bytes containing top-level routes: block return warning naming routes: and mentioning combos
	withRoutes := []byte(`
providers:
  openai:
    dialect: openai
    base_url: https://api.openai.com/v1
routes:
  - from: openai
    match: "*"
    chain: ["openai:gpt-4o"]
`)
	warnings := CheckDeprecated(withRoutes)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "routes:") || !strings.Contains(warnings[0], "combos") {
		t.Errorf("expected warning mentioning 'routes:' and 'combos', got %q", warnings[0])
	}

	// 2. Bytes without routes: return nil
	withoutRoutes := []byte(`
providers:
  openai:
    dialect: openai
    base_url: https://api.openai.com/v1
`)
	if w := CheckDeprecated(withoutRoutes); len(w) != 0 {
		t.Errorf("expected nil/empty warnings, got %v", w)
	}

	// 3. Bytes with only unknown-but-unrelated keys return nil
	withUnknown := []byte(`
future_field:
  nested: 123
providers:
  openai:
    dialect: openai
    base_url: https://api.openai.com/v1
`)
	if w := CheckDeprecated(withUnknown); len(w) != 0 {
		t.Errorf("expected nil/empty warnings for future_field, got %v", w)
	}
}

func TestTopologyRoutesIgnoredAndDroppedOnWrite(t *testing.T) {
	// Config with invalid/malformed routes block parses and validates with 0 errors
	rawWithInvalidRoutes := `
providers:
  openai:
    dialect: openai
    base_url: https://api.openai.com/v1
routes:
  - from: unknown_dialect
    match: "*"
    chain:
      - malformed_hop_no_colon
      - undeclared_provider:some-model
`
	topo, err := ParseTopology([]byte(rawWithInvalidRoutes))
	if err != nil {
		t.Fatalf("unexpected parse error on config with routes: %v", err)
	}

	errs := ValidateTopology(topo, []string{"openai"})
	if len(errs) != 0 {
		t.Errorf("expected routes block to be completely ignored during validation, got errors: %v", errs)
	}

	// WriteTopology round-trip emits no routes: key
	tmpFile := t.TempDir() + "/config.yaml"
	if err := WriteTopology(tmpFile, topo); err != nil {
		t.Fatalf("failed to write topology: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read written topology: %v", err)
	}

	if strings.Contains(string(data), "routes:") {
		t.Errorf("expected written YAML to not contain 'routes:', got:\n%s", string(data))
	}
}
