package preset_test

import (
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/preset"
)

func TestPresets(t *testing.T) {
	all := preset.All()
	if len(all) == 0 {
		t.Fatalf("expected presets, got 0")
	}

	names := preset.Names()
	if len(names) != len(all) {
		t.Errorf("len(Names()) = %d, len(All()) = %d", len(names), len(all))
	}

	// Test Opencode Zen lookup
	zenPreset := preset.Get("opencode-zen")
	if zenPreset == nil {
		t.Fatalf("expected opencode-zen preset, got nil")
	}
	if zenPreset.Name != "opencode-zen" {
		t.Errorf("zenPreset.Name = %q, want opencode-zen", zenPreset.Name)
	}

	// Test aliases/variations for Opencode Zen
	for _, variant := range []string{"zen", "Opencode Zen", "opencode_zen", "OPENCODE-ZEN"} {
		p := preset.Get(variant)
		if p == nil {
			t.Errorf("preset.Get(%q) = nil, expected opencode-zen", variant)
		} else if p.Name != "opencode-zen" {
			t.Errorf("preset.Get(%q).Name = %q, want opencode-zen", variant, p.Name)
		}
	}

	// Test OpenAI Compatible lookup
	compatPreset := preset.Get("openai-compatible")
	if compatPreset == nil {
		t.Fatalf("expected openai-compatible preset, got nil")
	}
	if compatPreset.Name != "openai-compatible" {
		t.Errorf("compatPreset.Name = %q, want openai-compatible", compatPreset.Name)
	}

	// Test aliases/variations for OpenAI Compatible
	for _, variant := range []string{"openai compatible", "OpenAI Compatible", "openai_compatible", "OPENAI-COMPATIBLE"} {
		p := preset.Get(variant)
		if p == nil {
			t.Errorf("preset.Get(%q) = nil, expected openai-compatible", variant)
		} else if p.Name != "openai-compatible" {
			t.Errorf("preset.Get(%q).Name = %q, want openai-compatible", variant, p.Name)
		}
	}
}

func TestXAIPresetOAuthMetadata(t *testing.T) {
	p := preset.Get("xai")
	if p == nil {
		t.Fatalf("preset.Get(\"xai\") returned nil")
	}

	if !p.OAuthCapable {
		t.Errorf("expected OAuthCapable true, got false")
	}
	if p.FlowType != "device_code" {
		t.Errorf("expected FlowType device_code, got %q", p.FlowType)
	}
	// ClientID should be present but not tested for specific value (configured by user)
	if p.ClientID == "" {
		t.Errorf("expected non-empty ClientID for xai OAuth provider")
	}
	if p.DeviceEndpoint != "https://auth.x.ai/oauth2/device" {
		t.Errorf("expected DeviceEndpoint https://auth.x.ai/oauth2/device, got %q", p.DeviceEndpoint)
	}
	if p.TokenEndpoint != "https://auth.x.ai/oauth2/token" {
		t.Errorf("expected TokenEndpoint https://auth.x.ai/oauth2/token, got %q", p.TokenEndpoint)
	}

	expectedScopes := []string{"openid", "profile", "email", "offline_access"}
	if len(p.Scopes) != len(expectedScopes) {
		t.Fatalf("expected %d scopes, got %d", len(expectedScopes), len(p.Scopes))
	}
	for i, s := range expectedScopes {
		if p.Scopes[i] != s {
			t.Errorf("scope[%d] = %q, want %q", i, p.Scopes[i], s)
		}
	}
}

func TestAntigravityPresetOAuthMetadata(t *testing.T) {
	p := preset.Get("antigravity")
	if p == nil {
		t.Fatalf("preset.Get(\"antigravity\") returned nil")
	}

	if p.FlowType != "pkce" {
		t.Errorf("expected FlowType pkce, got %q", p.FlowType)
	}
	if p.BaseURL != "https://daily-cloudcode-pa.googleapis.com" {
		t.Errorf("expected BaseURL https://daily-cloudcode-pa.googleapis.com, got %q", p.BaseURL)
	}
	if p.Transport != "cloudcode" {
		t.Errorf("expected Transport cloudcode, got %q", p.Transport)
	}
	// ClientID and ClientSecret should be present but not tested for specific values (configured by user)
	if p.ClientID == "" {
		t.Errorf("expected non-empty ClientID for antigravity OAuth provider")
	}
	// Google's token endpoint requires the (non-confidential) installed-app
	// client_secret for the code exchange and for refreshes. Without it the
	// exchange fails with "client_secret is missing".
	if p.ClientSecret == "" {
		t.Errorf("expected non-empty ClientSecret for antigravity; Google requires it at token exchange")
	}
	if !p.RefreshProfile.IncludeClientSecret {
		t.Errorf("expected RefreshProfile.IncludeClientSecret true so refresh sends client_secret")
	}
	// access_type=offline + prompt=consent force Google to issue a refresh_token
	// on every consent; without them, repeat logins return no refresh_token.
	if p.ExtraParams["access_type"] != "offline" {
		t.Errorf("expected ExtraParams[access_type]=offline, got %q", p.ExtraParams["access_type"])
	}
	if p.ExtraParams["prompt"] != "consent" {
		t.Errorf("expected ExtraParams[prompt]=consent, got %q", p.ExtraParams["prompt"])
	}
	if len(p.Models) == 0 {
		t.Errorf("expected non-empty Models for antigravity so the provider page lists known models")
	}
	// Antigravity exposes tiered CloudCode IDs (not generic Gemini API IDs).
	found := false
	for _, m := range p.Models {
		if m == "gemini-3.6-flash-high" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected antigravity Models to include the tiered id gemini-3.6-flash-high, got %v", p.Models)
	}
}

func TestOAuthPresets(t *testing.T) {
	oauthProviders := []string{
		"qwen",
		"github",
		"claude",
		"codex",
		"kimi",
		"gitlab",
		"grok-cli",
		"kilocode",
		"qoder",
		"kimchi",
		"iflow",
		"codebuddy-cn",
		"codebuddy-intl",
		"trae",
		"cline",
		"clinepass",
		"zed",
		"gemini-cli",
		"antigravity",
	}

	for _, name := range oauthProviders {
		p := preset.Get(name)
		if p == nil {
			t.Errorf("preset.Get(%q) = nil, expected valid preset", name)
			continue
		}
		if !p.OAuthCapable {
			t.Errorf("preset %q: expected OAuthCapable = true", name)
		}
		if p.FlowType == "" {
			t.Errorf("preset %q: expected non-empty FlowType", name)
		}
	}
}

func TestRiskNotices(t *testing.T) {
	claude := preset.Get("claude")
	if claude == nil {
		t.Fatalf("preset.Get(\"claude\") = nil")
	}
	wantClaudeRisk := "Revocation risk: Claude OAuth tokens from Claude Code are subject to Anthropic ToS."
	if claude.RiskNotice != wantClaudeRisk {
		t.Errorf("claude.RiskNotice = %q, want %q", claude.RiskNotice, wantClaudeRisk)
	}

	github := preset.Get("github")
	if github == nil {
		t.Fatalf("preset.Get(\"github\") = nil")
	}
	wantGithubRisk := "Revocation risk: GitHub Copilot OAuth tokens are subject to provider ToS and automated revocation checks."
	if github.RiskNotice != wantGithubRisk {
		t.Errorf("github.RiskNotice = %q, want %q", github.RiskNotice, wantGithubRisk)
	}
}

func TestPresetTiers(t *testing.T) {
	gemini := preset.Get("gemini")
	if gemini == nil || gemini.Tier != "freemium" {
		t.Errorf("expected gemini tier freemium, got %+v", gemini)
	}

	openrouter := preset.Get("openrouter")
	if openrouter == nil || openrouter.Tier != "freemium" {
		t.Errorf("expected openrouter tier freemium, got %+v", openrouter)
	}

	opencode := preset.Get("opencode-zen")
	if opencode == nil || opencode.Tier != "free" {
		t.Errorf("expected opencode-zen tier free, got %+v", opencode)
	}

	geminiCLI := preset.Get("gemini-cli")
	if geminiCLI == nil || geminiCLI.Tier != "free" {
		t.Errorf("expected gemini-cli tier free, got %+v", geminiCLI)
	}
}
