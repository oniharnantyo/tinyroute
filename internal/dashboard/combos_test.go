package dashboard

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/config"
)

func newAuthRequest(method, urlStr string, form url.Values, sessCookie string) *http.Request {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, urlStr, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, urlStr, nil)
	}
	req.Host = "127.0.0.1:8787"
	if sessCookie != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessCookie})
	}
	return req
}

func TestCombosPage_ListAndEmptyState(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	t.Run("empty state when no combos exist", func(t *testing.T) {
		req := newAuthRequest(http.MethodGet, "/dashboard/combos", nil, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "No combos configured") {
			t.Errorf("expected body to contain empty state title, got: %s", body)
		}
		if !strings.Contains(body, "href=\"/dashboard/combos\"") {
			t.Errorf("expected navigation bar to contain combos link, got: %s", body)
		}
	})

	t.Run("renders configured combo with numbered members and mode badge", func(t *testing.T) {
		data, err := os.ReadFile(deps.Service.ConfigPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		rawTopo, err := config.ParseRawTopology(data)
		if err != nil {
			t.Fatalf("parse topo: %v", err)
		}
		rawTopo.Combos = []config.Combo{
			{
				Name:         "coding-priority",
				Mode:         "ordered",
				Members:      []string{"openai:gpt-4o", "openai:gpt-4o-mini"},
				Capabilities: []string{"vision"},
			},
		}
		if err := config.WriteTopology(deps.Service.ConfigPath, rawTopo); err != nil {
			t.Fatalf("write topo: %v", err)
		}
		now := time.Now().Add(time.Second)
		_ = os.Chtimes(deps.Service.ConfigPath, now, now)
		_ = deps.TopologyWatcher.Get()

		req := newAuthRequest(http.MethodGet, "/dashboard/combos", nil, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "coding-priority") {
			t.Errorf("expected combo name to render, got: %s", body)
		}
		if !strings.Contains(body, "ordered") {
			t.Errorf("expected mode badge to render, got: %s", body)
		}
		if !strings.Contains(body, "openai:gpt-4o") || !strings.Contains(body, "openai:gpt-4o-mini") {
			t.Errorf("expected member models to render, got: %s", body)
		}
		if !strings.Contains(body, "vision") {
			t.Errorf("expected capability tag to render, got: %s", body)
		}
	})

	t.Run("flash param renders no static SSR toast (layout Toaster handles it)", func(t *testing.T) {
		req := newAuthRequest(http.MethodGet, "/dashboard/combos?flash="+url.QueryEscape("Successfully created combo 'my-combo'"), nil, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, "Successfully created combo") {
			t.Errorf("expected flash message not to render server-side, got: %s", body)
		}
	})

	t.Run("error param renders no static SSR toast (layout Toaster handles it)", func(t *testing.T) {
		req := newAuthRequest(http.MethodGet, "/dashboard/combos?error="+url.QueryEscape("Failed to delete combo"), nil, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, "Failed to delete combo") {
			t.Errorf("expected error message not to render server-side, got: %s", body)
		}
	})
}

func TestCombosWizard_StepAdvancementAndValidation(t *testing.T) {
	mux, deps, tmpDir := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	configPath := filepath.Join(tmpDir, "config.yaml")
	cfgContent := `providers:
  openai:
    dialect: openai
    base_url: https://api.openai.com/v1
    api_key: sk-test
    models:
    - gpt-4o
    - gpt-4o-mini
  anthropic:
    dialect: anthropic
    base_url: https://api.anthropic.com
    api_key: sk-ant-test
    models:
    - claude-sonnet-4.5
`
	if err := os.WriteFile(configPath, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	now := time.Now().Add(time.Second)
	_ = os.Chtimes(configPath, now, now)
	_ = deps.TopologyWatcher.Get()

	t.Run("open create dialog", func(t *testing.T) {
		form := url.Values{"action": {"open_create"}}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Name") || !strings.Contains(body, "wizard-combo-name") {
			t.Errorf("expected wizard step 1 to render, got: %s", body)
		}
	})

	t.Run("invalid name in step 1 re-renders with error", func(t *testing.T) {
		form := url.Values{
			"action": {"next_step_1"},
			"name":   {"invalid:name"},
			"step":   {"1"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "cannot contain") {
			t.Errorf("expected colon error, got: %s", body)
		}
	})

	t.Run("valid name advances to step 2 and can navigate back", func(t *testing.T) {
		form := url.Values{
			"action": {"next_step_1"},
			"name":   {"smart-priority"},
			"step":   {"1"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Members") || !strings.Contains(body, "selected_model") {
			t.Errorf("expected wizard step 2, got: %s", body)
		}

		// Navigate back to step 1
		backForm := url.Values{
			"action": {"back_step_1"},
			"name":   {"smart-priority"},
		}
		backReq := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", backForm, sess)
		backRec := httptest.NewRecorder()
		mux.ServeHTTP(backRec, backReq)
		if !strings.Contains(backRec.Body.String(), "wizard-combo-name") {
			t.Errorf("expected step 1 after back_step_1")
		}
	})

	t.Run("step 2 adding, reordering and removing members", func(t *testing.T) {
		// Add first member
		form := url.Values{
			"action":           {"add_member"},
			"name":             {"smart-priority"},
			"step":             {"2"},
			"selected_model":   {"openai:gpt-4o"},
			"selected_account": {"any"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		body := rec.Body.String()
		if !strings.Contains(body, "openai:gpt-4o") {
			t.Errorf("expected member 1 to be added, got: %s", body)
		}

		// Trying to advance with no members fails with error
		form = url.Values{
			"action": {"next_step_2"},
			"name":   {"smart-priority"},
			"step":   {"2"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		body = rec.Body.String()
		if !strings.Contains(body, "Add at least one member") {
			t.Errorf("expected empty-members error, got: %s", body)
		}

		// A single member is enough to continue
		form = url.Values{
			"action":       {"next_step_2"},
			"name":         {"smart-priority"},
			"step":         {"2"},
			"members_json": {"openai:gpt-4o"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		body = rec.Body.String()
		if !strings.Contains(body, "draft_mode") {
			t.Errorf("expected step 3 with a single member, got: %s", body)
		}

		// Add second member
		form = url.Values{
			"action":           {"add_member"},
			"name":             {"smart-priority"},
			"step":             {"2"},
			"members_json":     {"openai:gpt-4o"},
			"selected_model":   {"anthropic:claude-sonnet-4.5"},
			"selected_account": {"any"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		body = rec.Body.String()
		if !strings.Contains(body, "anthropic:claude-sonnet-4.5") {
			t.Errorf("expected member 2 to be added, got: %s", body)
		}

		// Reorder: Move Down index 0
		form = url.Values{
			"action":       {"move_down_0"},
			"name":         {"smart-priority"},
			"step":         {"2"},
			"members_json": {"openai:gpt-4o,anthropic:claude-sonnet-4.5"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		body = rec.Body.String()
		anthropicIdx := strings.Index(body, "anthropic:claude-sonnet-4.5")
		openaiIdx := strings.Index(body, "openai:gpt-4o")
		if anthropicIdx == -1 || openaiIdx == -1 || anthropicIdx > openaiIdx {
			t.Errorf("expected anthropic to appear before openai after move_down_0")
		}

		// Reorder: Move Up index 1
		form = url.Values{
			"action":       {"move_up_1"},
			"name":         {"smart-priority"},
			"step":         {"2"},
			"members_json": {"anthropic:claude-sonnet-4.5,openai:gpt-4o"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Remove index 0
		form = url.Values{
			"action":       {"remove_0"},
			"name":         {"smart-priority"},
			"step":         {"2"},
			"members_json": {"openai:gpt-4o,anthropic:claude-sonnet-4.5"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "Current Members (2)") {
			t.Errorf("expected 1 member after remove_0")
		}
	})

	t.Run("step 3 mode and step 4 capabilities transitions", func(t *testing.T) {
		// Advance to step 3
		form := url.Values{
			"action":       {"next_step_2"},
			"name":         {"smart-priority"},
			"members_json": {"openai:gpt-4o,anthropic:claude-sonnet-4.5"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), "draft_mode") {
			t.Fatalf("expected step 3, got %s", rec.Body.String())
		}

		// Advance to step 4 with pool mode
		form = url.Values{
			"action":       {"next_step_3"},
			"name":         {"smart-priority"},
			"members_json": {"openai:gpt-4o,anthropic:claude-sonnet-4.5"},
			"draft_mode":   {"pool"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), "draft_caps") {
			t.Fatalf("expected step 4, got %s", rec.Body.String())
		}

		// Back to step 3
		form = url.Values{
			"action":       {"back_step_3"},
			"name":         {"smart-priority"},
			"members_json": {"openai:gpt-4o,anthropic:claude-sonnet-4.5"},
			"mode":         {"pool"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), "draft_mode") {
			t.Fatalf("expected step 3 after back_step_3")
		}

		// Advance to step 5 with capabilities
		form = url.Values{
			"action":       {"next_step_4"},
			"name":         {"smart-priority"},
			"members_json": {"openai:gpt-4o,anthropic:claude-sonnet-4.5"},
			"mode":         {"pool"},
			"draft_caps":   {"vision", "pdf"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), "Execution Mode:") {
			t.Fatalf("expected step 5 review, got %s", rec.Body.String())
		}

		// Back to step 4
		form = url.Values{
			"action":            {"back_step_4"},
			"name":              {"smart-priority"},
			"members_json":      {"openai:gpt-4o,anthropic:claude-sonnet-4.5"},
			"mode":              {"pool"},
			"capabilities_json": {"vision,pdf"},
		}
		req = newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), "draft_caps") {
			t.Fatalf("expected step 4 after back_step_4")
		}
	})

	t.Run("complete flow and submit create", func(t *testing.T) {
		form := url.Values{
			"action":            {"submit_create"},
			"name":              {"smart-priority"},
			"members_json":      {"openai:gpt-4o,anthropic:claude-sonnet-4.5"},
			"mode":              {"ordered"},
			"capabilities_json": {"vision,pdf"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect on success, got %d", rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "flash=") || !strings.Contains(loc, "smart-priority") {
			t.Errorf("expected flash message in redirect location, got: %s", loc)
		}

		data, err := os.ReadFile(deps.Service.ConfigPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		rawTopo, err := config.ParseRawTopology(data)
		if err != nil {
			t.Fatalf("parse topo: %v", err)
		}

		if len(rawTopo.Combos) != 1 {
			t.Fatalf("expected 1 combo in file, got %d", len(rawTopo.Combos))
		}
		cb := rawTopo.Combos[0]
		if cb.Name != "smart-priority" || cb.Mode != "ordered" {
			t.Errorf("unexpected combo in file: %+v", cb)
		}
		if len(cb.Members) != 2 || cb.Members[0] != "openai:gpt-4o" || cb.Members[1] != "anthropic:claude-sonnet-4.5" {
			t.Errorf("unexpected members in file: %v", cb.Members)
		}
	})
}

func TestCombos_EditAndDelete(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	data, err := os.ReadFile(deps.Service.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		t.Fatalf("parse topo: %v", err)
	}
	rawTopo.Combos = []config.Combo{
		{
			Name:         "test-combo",
			Mode:         "ordered",
			Members:      []string{"openai:gpt-4o", "openai:gpt-4o-mini"},
			Capabilities: []string{"vision"},
		},
	}
	if err := config.WriteTopology(deps.Service.ConfigPath, rawTopo); err != nil {
		t.Fatalf("write topo: %v", err)
	}
	now := time.Now().Add(time.Second)
	_ = os.Chtimes(deps.Service.ConfigPath, now, now)
	_ = deps.TopologyWatcher.Get()

	t.Run("open edit non-existent combo returns error redirect", func(t *testing.T) {
		form := url.Values{
			"action": {"open_edit"},
			"name":   {"non-existent"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Location"), "error=") {
			t.Errorf("expected error in location redirect")
		}
	})

	t.Run("open edit pre-fills current values", func(t *testing.T) {
		form := url.Values{
			"action": {"open_edit"},
			"name":   {"test-combo"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Edit Combo") {
			t.Errorf("expected Edit Combo title, got: %s", body)
		}
		if !strings.Contains(body, "value=\"test-combo\"") {
			t.Errorf("expected pre-filled combo name, got: %s", body)
		}
	})

	t.Run("submit create with no members fails", func(t *testing.T) {
		form := url.Values{
			"action": {"submit_create"},
			"name":   {"invalid-combo"},
			"mode":   {"ordered"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect on error, got %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Location"), "error=") {
			t.Errorf("expected error in location")
		}
	})

	t.Run("save edit updates topology", func(t *testing.T) {
		form := url.Values{
			"action":            {"submit_create"},
			"is_edit":           {"true"},
			"initial_name":      {"test-combo"},
			"name":              {"test-combo-updated"},
			"members_json":      {"openai:gpt-4o-mini,openai:gpt-4o"},
			"mode":              {"pool"},
			"capabilities_json": {"audio"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}

		data, err := os.ReadFile(deps.Service.ConfigPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		rawTopo, err := config.ParseRawTopology(data)
		if err != nil {
			t.Fatalf("parse topo: %v", err)
		}

		if len(rawTopo.Combos) != 1 {
			t.Fatalf("expected 1 combo, got %d", len(rawTopo.Combos))
		}
		cb := rawTopo.Combos[0]
		if cb.Name != "test-combo-updated" || cb.Mode != "pool" || len(cb.Capabilities) != 1 || cb.Capabilities[0] != "audio" {
			t.Errorf("unexpected updated combo: %+v", cb)
		}
		if cb.Members[0] != "openai:gpt-4o-mini" || cb.Members[1] != "openai:gpt-4o" {
			t.Errorf("unexpected updated members: %v", cb.Members)
		}
	})

	t.Run("delete combo with empty name fails", func(t *testing.T) {
		form := url.Values{"name": {""}}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/delete", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
			t.Errorf("expected error redirect for empty name")
		}
	})

	t.Run("delete non-existent combo fails", func(t *testing.T) {
		form := url.Values{"name": {"non-existent"}}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/delete", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
			t.Errorf("expected error redirect for nonexistent combo")
		}
	})

	t.Run("step 3 unknown mode defaults to ordered", func(t *testing.T) {
		form := url.Values{
			"action":       {"next_step_3"},
			"name":         {"test-combo"},
			"members_json": {"openai:gpt-4o,openai:gpt-4o-mini"},
			"draft_mode":   {"unknown_mode"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), "draft_caps") {
			t.Errorf("expected step 4 after unknown mode")
		}
	})

	t.Run("step 2 adding duplicate candidate is ignored", func(t *testing.T) {
		form := url.Values{
			"action":           {"add_member"},
			"name":             {"test-combo"},
			"step":             {"2"},
			"members_json":     {"openai:gpt-4o"},
			"selected_model":   {"openai:gpt-4o"},
			"selected_account": {"any"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if strings.Contains(rec.Body.String(), "Fallback Chain (2)") {
			t.Errorf("expected duplicate member to not be added")
		}
	})

	t.Run("submit create with invalid topology fails", func(t *testing.T) {
		form := url.Values{
			"action":       {"submit_create"},
			"name":         {"invalid-topo"},
			"members_json": {"openai:gpt-4o,openai:gpt-4o-mini"},
			"mode":         {"unsupported_mode"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
			t.Errorf("expected error redirect on invalid topology")
		}
	})

	t.Run("delete combo removes it from config", func(t *testing.T) {
		form := url.Values{
			"name": {"test-combo-updated"},
		}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/delete", form, sess)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "flash=") || !strings.Contains(loc, "deleted") {
			t.Errorf("expected delete flash message, got: %s", loc)
		}

		data, err := os.ReadFile(deps.Service.ConfigPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		rawTopo, err := config.ParseRawTopology(data)
		if err != nil {
			t.Fatalf("parse topo: %v", err)
		}
		if len(rawTopo.Combos) != 0 {
			t.Errorf("expected 0 combos after deletion, got %d", len(rawTopo.Combos))
		}
	})

	t.Run("delete combo config parse failure", func(t *testing.T) {
		// Corrupt config file temporarily
		orig, _ := os.ReadFile(deps.Service.ConfigPath)
		_ = os.WriteFile(deps.Service.ConfigPath, []byte("invalid: [yaml: content"), 0600)
		defer func() { _ = os.WriteFile(deps.Service.ConfigPath, orig, 0600) }()

		form := url.Values{"name": {"test-combo"}}
		req := newAuthRequest(http.MethodPost, "/dashboard/combos/delete", form, sess)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
			t.Errorf("expected error redirect on corrupted config file")
		}
	})

	t.Run("delete combo form parse error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/dashboard/combos/delete", &errReader{})
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = "127.0.0.1:8787"
		req.AddCookie(&http.Cookie{Name: "tinyroute_session", Value: sess})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "error=") {
			t.Errorf("expected error redirect on form parse error")
		}
	})
}

type errReader struct{}

func (e *errReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("read error")
}

func TestCombosPage_RenderVariants(t *testing.T) {
	ctx := context.Background()

	// Step 1 with error
	var buf strings.Builder
	err := CombosPage(CombosPageData{
		DialogOpen: true,
		Draft: WizardDraft{
			Step:  1,
			Error: "Invalid combo name",
		},
	}).Render(ctx, &buf)
	if err != nil {
		t.Fatalf("render step 1: %v", err)
	}
	if !strings.Contains(buf.String(), "Invalid combo name") {
		t.Errorf("expected error message in step 1")
	}

	// Step 2 empty members
	buf.Reset()
	err = CombosPage(CombosPageData{
		DialogOpen:   true,
		ModelOptions: []string{"openai:gpt-4o", "anthropic:claude-3.5"},
		Draft: WizardDraft{
			Step:    2,
			Name:    "my-combo",
			Members: nil,
		},
	}).Render(ctx, &buf)
	if err != nil {
		t.Fatalf("render step 2 empty: %v", err)
	}
	if !strings.Contains(buf.String(), "No members added yet") {
		t.Errorf("expected empty members notice in step 2")
	}

	// Step 2 with members
	buf.Reset()
	err = CombosPage(CombosPageData{
		DialogOpen:     true,
		ModelOptions:   []string{"openai:gpt-4o", "anthropic:claude-3.5"},
		AccountOptions: []string{"glm@personal", "glm@work"},
		Draft: WizardDraft{
			Step:    2,
			Name:    "my-combo",
			Members: []string{"openai:gpt-4o", "anthropic:claude-3.5"},
		},
	}).Render(ctx, &buf)
	if err != nil {
		t.Fatalf("render step 2 with members: %v", err)
	}
	if !strings.Contains(buf.String(), "Current Members (2)") {
		t.Errorf("expected Current Members (2) in step 2")
	}

	// Step 3 pool mode
	buf.Reset()
	err = CombosPage(CombosPageData{
		DialogOpen: true,
		Draft: WizardDraft{
			Step:    3,
			Name:    "my-combo",
			Members: []string{"openai:gpt-4o", "anthropic:claude-3.5"},
			Mode:    "pool",
		},
	}).Render(ctx, &buf)
	if err != nil {
		t.Fatalf("render step 3 pool: %v", err)
	}
	if !strings.Contains(buf.String(), "Mode") || !strings.Contains(buf.String(), "pool") {
		t.Errorf("expected step 3 in output")
	}

	// Step 4 with capabilities
	buf.Reset()
	err = CombosPage(CombosPageData{
		DialogOpen: true,
		Draft: WizardDraft{
			Step:         4,
			Name:         "my-combo",
			Members:      []string{"openai:gpt-4o", "anthropic:claude-3.5"},
			Capabilities: []string{"vision", "audio"},
		},
	}).Render(ctx, &buf)
	if err != nil {
		t.Fatalf("render step 4: %v", err)
	}
	if !strings.Contains(buf.String(), "Caps") {
		t.Errorf("expected step 4 in output")
	}

	// Step 5 edit mode with capabilities
	buf.Reset()
	err = CombosPage(CombosPageData{
		DialogOpen: true,
		Draft: WizardDraft{
			Step:         5,
			IsEdit:       true,
			InitialName:  "my-combo",
			Name:         "my-combo-updated",
			Members:      []string{"openai:gpt-4o", "anthropic:claude-3.5"},
			Mode:         "fused",
			Capabilities: []string{"vision"},
		},
	}).Render(ctx, &buf)
	if err != nil {
		t.Fatalf("render step 5 edit: %v", err)
	}
	if !strings.Contains(buf.String(), "Save Changes") || !strings.Contains(buf.String(), "vision") {
		t.Errorf("expected Save Changes and vision in step 5 edit")
	}

	// Combos list with multiple combos (some with capabilities, some without)
	buf.Reset()
	err = CombosPage(CombosPageData{
		Combos: []ComboItem{
			{Name: "c1", Mode: "ordered", Members: []string{"p1:m1", "p1:m2"}, Capabilities: []string{"vision"}},
			{Name: "c2", Mode: "pool", Members: []string{"p2:m1", "p2:m2"}, Capabilities: nil},
		},
	}).Render(ctx, &buf)
	if err != nil {
		t.Fatalf("render combos list: %v", err)
	}
	if !strings.Contains(buf.String(), "c1") || !strings.Contains(buf.String(), "c2") {
		t.Errorf("expected combo names rendered in cards")
	}
}

func TestCombos_TemplateHelpers(t *testing.T) {
	// groupModelsByProvider (models-only, combo: excluded)
	mGroups := groupModelsByProvider([]string{"combo:cheap", "combo:fast", "openai:gpt-4o", "openai:gpt-4o-mini", "anthropic:claude-3.5"})
	if len(mGroups) != 2 {
		t.Errorf("expected 2 provider model groups, got %d", len(mGroups))
	}
	if mGroups[0].Provider != "openai" || len(mGroups[0].Models) != 2 {
		t.Errorf("unexpected openai group: %+v", mGroups[0])
	}
	if mGroups[0].Models[0].Value != "openai:gpt-4o" || mGroups[0].Models[0].Label != "gpt-4o" {
		t.Errorf("unexpected openai model 0: %+v", mGroups[0].Models[0])
	}
	if mGroups[1].Provider != "anthropic" || len(mGroups[1].Models) != 1 {
		t.Errorf("unexpected anthropic group: %+v", mGroups[1])
	}

	// groupAccountsByProvider
	accGroups := groupAccountsByProvider([]string{"glm@personal", "glm@work", "aws@prod"})
	if len(accGroups) != 2 {
		t.Errorf("expected 2 provider account groups, got %d", len(accGroups))
	}
	if accGroups[0].Provider != "glm" || len(accGroups[0].Accounts) != 2 || accGroups[0].Accounts[0] != "personal" || accGroups[0].Accounts[1] != "work" {
		t.Errorf("unexpected glm account group: %+v", accGroups[0])
	}
	if accGroups[1].Provider != "aws" || len(accGroups[1].Accounts) != 1 {
		t.Errorf("unexpected aws account group: %+v", accGroups[1])
	}

	// isCapabilitySelected
	if !isCapabilitySelected("vision", []string{"vision", "pdf"}) {
		t.Errorf("expected vision to be selected")
	}
	if isCapabilitySelected("audio", []string{"vision", "pdf"}) {
		t.Errorf("expected audio not to be selected")
	}

	// isMemberChosen
	if !isMemberChosen("openai:gpt-4o", []string{"openai:gpt-4o"}) {
		t.Errorf("expected true for chosen member")
	}
	if isMemberChosen("openai:gpt-4o-mini", []string{"openai:gpt-4o"}) {
		t.Errorf("expected false for unchosen member")
	}

	// stepClass
	if stepClass(1, 1) != "text-primary font-bold" {
		t.Errorf("unexpected current step class")
	}
	if stepClass(1, 2) != "text-foreground" {
		t.Errorf("unexpected past step class")
	}
	if stepClass(3, 1) != "text-muted-foreground/60" {
		t.Errorf("unexpected future step class")
	}

	// stepCardClass
	if !strings.Contains(stepCardClass(1, 1), "border-primary") {
		t.Errorf("unexpected active step card class")
	}
	if !strings.Contains(stepCardClass(1, 2), "bg-muted/30") {
		t.Errorf("unexpected completed step card class")
	}
	if !strings.Contains(stepCardClass(3, 1), "border-border/60") {
		t.Errorf("unexpected upcoming step card class")
	}
}

func TestCombosWizard_AccountPinnedCandidatesAndCreation(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	// Configure provider with 2 accounts in topology
	data, err := os.ReadFile(deps.Service.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		t.Fatalf("parse topo: %v", err)
	}
	rawTopo.Providers["glm"] = config.Provider{
		Dialect: "openai",
		BaseURL: "https://api.zhipu.ai",
		Models:  []string{"glm-4.7"},
		Accounts: []config.Account{
			{Name: "work"},
			{Name: "personal"},
		},
	}
	if err := config.WriteTopology(deps.Service.ConfigPath, rawTopo); err != nil {
		t.Fatalf("write topo: %v", err)
	}
	now := time.Now().Add(time.Second)
	_ = os.Chtimes(deps.Service.ConfigPath, now, now)
	_ = deps.TopologyWatcher.Get()

	// 1. Check dropdowns rendering in step 2
	step2Req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", url.Values{
		"action": {"next_step_1"},
		"name":   {"my-pinned-combo"},
	}, sess)
	step2Rec := httptest.NewRecorder()
	mux.ServeHTTP(step2Rec, step2Req)
	if step2Rec.Code != http.StatusOK {
		t.Fatalf("step 2 get status: %d", step2Rec.Code)
	}
	step2Body := step2Rec.Body.String()

	// Model dropdown lists glm:glm-4.7 under glm group, no @account options
	if !strings.Contains(step2Body, `optgroup label="glm"`) || !strings.Contains(step2Body, `value="glm:glm-4.7"`) {
		t.Errorf("expected model dropdown with glm:glm-4.7 in step 2: %s", step2Body)
	}
	if strings.Contains(step2Body, `value="glm@work:glm-4.7"`) {
		t.Errorf("model dropdown should not contain @account options: %s", step2Body)
	}

	// Connection dropdown renders disabled until a model is selected; the
	// per-provider account list rides in a data-attribute JSON island
	// (entity-escaped in the HTML, unescaped by the DOM on read) consumed
	// by the client-side sync scoped to the selected model's provider
	if !strings.Contains(step2Body, `name="selected_account" disabled`) || !strings.Contains(step2Body, "Select a model first") {
		t.Errorf("expected connection dropdown disabled until model selection: %s", step2Body)
	}
	if !strings.Contains(html.UnescapeString(step2Body), `"glm":["personal","work"]`) {
		t.Errorf("expected account data island scoped per provider: %s", step2Body)
	}

	// 2. Mismatched connection rejection
	mismatchForm := url.Values{
		"action":           {"add_member"},
		"step":             {"2"},
		"name":             {"my-pinned-combo"},
		"selected_model":   {"openai:gpt-4o"},
		"selected_account": {"glm@work"},
	}
	mismatchReq := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", mismatchForm, sess)
	mismatchRec := httptest.NewRecorder()
	mux.ServeHTTP(mismatchRec, mismatchReq)
	if mismatchRec.Code != http.StatusOK {
		t.Fatalf("mismatch request status: %d", mismatchRec.Code)
	}
	mismatchBody := mismatchRec.Body.String()
	if !strings.Contains(mismatchBody, "belongs to provider") || !strings.Contains(mismatchBody, "glm") || !strings.Contains(mismatchBody, "openai") {
		t.Errorf("expected mismatch error in step 2: %s", mismatchBody)
	}

	// 3. Add unpinned member via Any connection
	unpinnedForm := url.Values{
		"action":           {"add_member"},
		"step":             {"2"},
		"name":             {"my-pinned-combo"},
		"selected_model":   {"glm:glm-4.7"},
		"selected_account": {"any"},
	}
	unpinnedReq := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", unpinnedForm, sess)
	unpinnedRec := httptest.NewRecorder()
	mux.ServeHTTP(unpinnedRec, unpinnedReq)
	unpinnedBody := unpinnedRec.Body.String()
	if !strings.Contains(unpinnedBody, "glm:glm-4.7") {
		t.Errorf("expected unpinned member added: %s", unpinnedBody)
	}

	// 4. Add pinned member via model + connection dropdown
	form := url.Values{
		"action":           {"add_member"},
		"step":             {"2"},
		"name":             {"my-pinned-combo"},
		"members_json":     {"glm:glm-4.7"},
		"selected_model":   {"glm:glm-4.7"},
		"selected_account": {"glm@work"},
	}
	req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", form, sess)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "glm@work:glm-4.7") || !strings.Contains(body, "glm:glm-4.7") {
		t.Errorf("expected both unpinned and pinned members in step 2: %s", body)
	}

	// 5. Duplicate composed member error
	dupForm := url.Values{
		"action":           {"add_member"},
		"step":             {"2"},
		"name":             {"my-pinned-combo"},
		"members_json":     {"glm:glm-4.7,glm@work:glm-4.7"},
		"selected_model":   {"glm:glm-4.7"},
		"selected_account": {"glm@work"},
	}
	dupReq := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", dupForm, sess)
	dupRec := httptest.NewRecorder()
	mux.ServeHTTP(dupRec, dupReq)
	if !strings.Contains(dupRec.Body.String(), "already in the combo") {
		t.Errorf("expected duplicate member error: %s", dupRec.Body.String())
	}

	// 6. Final submit_create writes pinned member verbatim
	submitForm := url.Values{
		"action":       {"submit_create"},
		"name":         {"my-pinned-combo"},
		"mode":         {"ordered"},
		"members_json": {"glm:glm-4.7,glm@work:glm-4.7"},
	}
	submitReq := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", submitForm, sess)
	submitRec := httptest.NewRecorder()
	mux.ServeHTTP(submitRec, submitReq)

	if submitRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on create, got %d", submitRec.Code)
	}

	savedData, _ := os.ReadFile(deps.Service.ConfigPath)
	savedTopo, _ := config.ParseRawTopology(savedData)
	var created *config.Combo
	for _, c := range savedTopo.Combos {
		if c.Name == "my-pinned-combo" {
			cp := c
			created = &cp
			break
		}
	}
	if created == nil {
		t.Fatalf("combo was not created in topology")
	}
	if len(created.Members) != 2 || created.Members[0] != "glm:glm-4.7" || created.Members[1] != "glm@work:glm-4.7" {
		t.Errorf("unexpected created combo members: %v", created.Members)
	}
}

func TestCombosWizard_EditRoundTrip_PreservesAccountPins(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	data, _ := os.ReadFile(deps.Service.ConfigPath)
	rawTopo, _ := config.ParseRawTopology(data)
	rawTopo.Providers["glm"] = config.Provider{
		Dialect: "openai",
		BaseURL: "https://api.zhipu.ai",
		Models:  []string{"glm-4.7"},
		Accounts: []config.Account{
			{Name: "work"},
			{Name: "personal"},
		},
	}
	rawTopo.Combos = []config.Combo{
		{
			Name:    "edit-pinned-combo",
			Mode:    "ordered",
			Members: []string{"glm@work:glm-4.7", "glm:glm-4.7"},
		},
	}
	_ = config.WriteTopology(deps.Service.ConfigPath, rawTopo)
	now := time.Now().Add(time.Second)
	_ = os.Chtimes(deps.Service.ConfigPath, now, now)
	_ = deps.TopologyWatcher.Get()

	// 1. Open edit
	openForm := url.Values{
		"action": {"open_edit"},
		"name":   {"edit-pinned-combo"},
	}
	req := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", openForm, sess)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "glm@work:glm-4.7") {
		t.Errorf("expected pinned member rendered in edit draft: %s", body)
	}

	// 2. Save edit without changing members
	saveForm := url.Values{
		"action":       {"submit_create"},
		"is_edit":      {"true"},
		"initial_name": {"edit-pinned-combo"},
		"name":         {"edit-pinned-combo"},
		"mode":         {"pool"},
		"members_json": {"glm@work:glm-4.7,glm:glm-4.7"},
	}
	saveReq := newAuthRequest(http.MethodPost, "/dashboard/combos/wizard", saveForm, sess)
	saveRec := httptest.NewRecorder()
	mux.ServeHTTP(saveRec, saveReq)

	if saveRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on save, got %d", saveRec.Code)
	}

	savedData, _ := os.ReadFile(deps.Service.ConfigPath)
	savedTopo, _ := config.ParseRawTopology(savedData)
	var updated *config.Combo
	for _, c := range savedTopo.Combos {
		if c.Name == "edit-pinned-combo" {
			cp := c
			updated = &cp
			break
		}
	}
	if updated == nil {
		t.Fatalf("combo not found after edit")
	}
	if updated.Mode != "pool" || updated.Members[0] != "glm@work:glm-4.7" || updated.Members[1] != "glm:glm-4.7" {
		t.Errorf("unexpected updated combo: %+v", updated)
	}
}

func TestDashboard_AccountRename_RewritesComboPins(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	data, _ := os.ReadFile(deps.Service.ConfigPath)
	rawTopo, _ := config.ParseRawTopology(data)
	rawTopo.Providers["glm"] = config.Provider{
		Dialect: "openai",
		BaseURL: "https://api.zhipu.ai",
		Accounts: []config.Account{
			{Name: "work", Type: "static", APIKey: "k1"},
			{Name: "personal", Type: "static", APIKey: "k2"},
		},
	}
	rawTopo.Combos = []config.Combo{
		{
			Name:    "pinned-combo",
			Members: []string{"glm@work:glm-4.7", "glm:glm-4.7"},
		},
		{
			Name:    "untouched-combo",
			Members: []string{"openai:gpt-4o", "anthropic:claude-3.5"},
		},
	}
	_ = config.WriteTopology(deps.Service.ConfigPath, rawTopo)

	form := url.Values{
		"provider":    {"glm"},
		"old_account": {"work"},
		"new_account": {"team"},
	}
	req := newAuthRequest(http.MethodPost, "/dashboard/providers/account/rename", form, sess)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}

	savedData, _ := os.ReadFile(deps.Service.ConfigPath)
	savedTopo, _ := config.ParseRawTopology(savedData)

	if savedTopo.Combos[0].Members[0] != "glm@team:glm-4.7" {
		t.Errorf("expected rewritten combo member glm@team:glm-4.7, got: %v", savedTopo.Combos[0].Members)
	}
	if savedTopo.Combos[1].Members[0] != "openai:gpt-4o" {
		t.Errorf("untouched combo modified: %v", savedTopo.Combos[1].Members)
	}
}

func TestDashboard_CredentialDelete_DowngradesComboPins(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	data, _ := os.ReadFile(deps.Service.ConfigPath)
	rawTopo, _ := config.ParseRawTopology(data)
	rawTopo.Providers["glm"] = config.Provider{
		Dialect: "openai",
		BaseURL: "https://api.zhipu.ai",
		Accounts: []config.Account{
			{Name: "work", Type: "static", APIKey: "k1"},
			{Name: "personal", Type: "static", APIKey: "k2"},
		},
	}
	rawTopo.Combos = []config.Combo{
		{
			Name:    "c1-downgrades",
			Members: []string{"glm@work:glm-4.7", "openai:gpt-4o"},
		},
		{
			Name:    "c2-dedup-to-unpinned",
			Members: []string{"glm@work:glm-4.7", "glm:glm-4.7"},
		},
	}
	_ = config.WriteTopology(deps.Service.ConfigPath, rawTopo)

	form := url.Values{
		"name":    {"glm"},
		"account": {"work"},
	}
	req := newAuthRequest(http.MethodPost, "/dashboard/providers/credential/delete", form, sess)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "c1-downgrades") || !strings.Contains(loc, "c2-dedup-to-unpinned") {
		t.Errorf("expected redirect message to name modified combos, got: %s", loc)
	}

	savedData, _ := os.ReadFile(deps.Service.ConfigPath)
	savedTopo, _ := config.ParseRawTopology(savedData)

	if len(savedTopo.Combos) != 2 {
		t.Fatalf("expected both combos to survive downgrade, got %d", len(savedTopo.Combos))
	}
	if savedTopo.Combos[0].Name != "c1-downgrades" || savedTopo.Combos[0].Members[0] != "glm:glm-4.7" {
		t.Errorf("expected c1 downgraded to glm:glm-4.7, got: %v", savedTopo.Combos[0].Members)
	}
	if len(savedTopo.Combos[1].Members) != 1 || savedTopo.Combos[1].Members[0] != "glm:glm-4.7" {
		t.Errorf("expected c2 deduped to a single unpinned member, got: %v", savedTopo.Combos[1].Members)
	}
}

func TestProviderDetail_DisconnectConfirmation_WarnsCombos(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	data, _ := os.ReadFile(deps.Service.ConfigPath)
	rawTopo, _ := config.ParseRawTopology(data)
	rawTopo.Providers["glm"] = config.Provider{
		Dialect: "openai",
		BaseURL: "https://api.zhipu.ai",
		Accounts: []config.Account{
			{Name: "work", Type: "static", APIKey: "k1"},
		},
	}
	rawTopo.Combos = []config.Combo{
		{
			Name:    "pinned-combo",
			Members: []string{"glm@work:glm-4.7", "openai:gpt-4o"},
		},
	}
	_ = config.WriteTopology(deps.Service.ConfigPath, rawTopo)
	now := time.Now().Add(time.Second)
	_ = os.Chtimes(deps.Service.ConfigPath, now, now)
	_ = deps.TopologyWatcher.Get()

	req := newAuthRequest(http.MethodGet, "/dashboard/providers/glm", nil, sess)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "dialog-disconnect-work") {
		t.Errorf("expected disconnect dialog in page: %s", body)
	}
	if !strings.Contains(body, "Warning: Combo routing will be affected") || !strings.Contains(body, "1 combo(s) referencing this connection will be adjusted") {
		t.Errorf("expected combo warning in disconnect dialog: %s", body)
	}
}

func TestCombosToggle(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	data, _ := os.ReadFile(deps.Service.ConfigPath)
	rawTopo, _ := config.ParseRawTopology(data)
	rawTopo.Combos = []config.Combo{
		{
			Name:         "toggle-combo",
			Mode:         "ordered",
			Members:      []string{"openai:gpt-4o"},
			Capabilities: []string{"vision"},
			Disabled:     false,
		},
	}
	_ = config.WriteTopology(deps.Service.ConfigPath, rawTopo)
	now := time.Now().Add(time.Second)
	_ = os.Chtimes(deps.Service.ConfigPath, now, now)
	_ = deps.TopologyWatcher.Get()

	// 1. Enable -> Disable
	form := url.Values{"name": {"toggle-combo"}}
	req := newAuthRequest(http.MethodPost, "/dashboard/combos/toggle", form, sess)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "disabled") || !strings.Contains(loc, "toggle-combo") {
		t.Errorf("expected flash redirect indicating disabled, got: %s", loc)
	}

	savedData, _ := os.ReadFile(deps.Service.ConfigPath)
	savedTopo, _ := config.ParseRawTopology(savedData)
	if len(savedTopo.Combos) != 1 || !savedTopo.Combos[0].Disabled {
		t.Fatalf("expected combo to be disabled in config, got: %+v", savedTopo.Combos[0])
	}
	if savedTopo.Combos[0].Mode != "ordered" || len(savedTopo.Combos[0].Members) != 1 || len(savedTopo.Combos[0].Capabilities) != 1 {
		t.Fatalf("expected combo members/mode/caps to be preserved, got: %+v", savedTopo.Combos[0])
	}

	// 2. Disable -> Enable
	_ = os.Chtimes(deps.Service.ConfigPath, now, now)
	_ = deps.TopologyWatcher.Get()
	req = newAuthRequest(http.MethodPost, "/dashboard/combos/toggle", form, sess)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}
	loc = rec.Header().Get("Location")
	if !strings.Contains(loc, "enabled") || !strings.Contains(loc, "toggle-combo") {
		t.Errorf("expected flash redirect indicating enabled, got: %s", loc)
	}

	savedData, _ = os.ReadFile(deps.Service.ConfigPath)
	savedTopo, _ = config.ParseRawTopology(savedData)
	if len(savedTopo.Combos) != 1 || savedTopo.Combos[0].Disabled {
		t.Fatalf("expected combo to be enabled in config, got: %+v", savedTopo.Combos[0])
	}

	// 3. Unknown combo name error
	unknownForm := url.Values{"name": {"unknown-combo"}}
	req = newAuthRequest(http.MethodPost, "/dashboard/combos/toggle", unknownForm, sess)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}
	loc = rec.Header().Get("Location")
	if !strings.Contains(loc, "error=") || !strings.Contains(loc, "not+found") {
		t.Errorf("expected error redirect for unknown combo, got: %s", loc)
	}

	// 4. CSRF / Origin protection rejection
	req = httptest.NewRequest(http.MethodPost, "/dashboard/combos/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.com")
	req.Host = "127.0.0.1:8787"
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sess})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for cross-origin mutation, got %d", rec.Code)
	}
}

func TestCombosCardRefinements(t *testing.T) {
	mux, deps, _ := setupTestMux(t)
	sess := deps.SessionStore.CreateSession(time.Hour)

	data, _ := os.ReadFile(deps.Service.ConfigPath)
	rawTopo, _ := config.ParseRawTopology(data)
	rawTopo.Combos = []config.Combo{
		{
			Name:    "two-members",
			Members: []string{"openai:gpt-4o", "anthropic:claude-3-5-sonnet"},
		},
		{
			Name:    "three-members",
			Members: []string{"openai:gpt-4o", "anthropic:claude-3-5-sonnet", "glm:glm-4.7"},
		},
		{
			Name:    "four-members",
			Members: []string{"openai:gpt-4o", "anthropic:claude-3-5-sonnet", "glm:glm-4.7", "cohere:command-r"},
		},
		{
			Name: "five-members-pinned",
			Members: []string{
				"glm@work:glm-4.7",
				"openai@acc1:gpt-4o",
				"openai@acc2:gpt-4o",
				"anthropic@team:claude-sonnet-4.5",
				"meta:llama-3",
			},
			Disabled: true,
		},
	}
	_ = config.WriteTopology(deps.Service.ConfigPath, rawTopo)
	now := time.Now().Add(time.Second)
	_ = os.Chtimes(deps.Service.ConfigPath, now, now)
	_ = deps.TopologyWatcher.Get()

	req := newAuthRequest(http.MethodGet, "/dashboard/combos", nil, sess)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. Title renders at text-base
	if !strings.Contains(body, "font-bold text-base text-foreground truncate") {
		t.Errorf("expected card titles to render with text-base class, got: %s", body)
	}

	// 2. 2-member and 3-member combos show all chips without overflow
	if strings.Contains(body, "+0 more") || strings.Contains(body, "+-1 more") {
		t.Errorf("unexpected invalid overflow marker in body")
	}

	// 3. 4-member combo renders +1 more…
	if !strings.Contains(body, "+1 more…") {
		t.Errorf("expected 4-member combo to render +1 more…, got: %s", body)
	}
	if !strings.Contains(body, "title=\"cohere:command-r\"") {
		t.Errorf("expected hover title for 4-member combo to list cohere:command-r, got: %s", body)
	}

	// 4. 5-member combo renders +2 more… with pin stripping
	if !strings.Contains(body, "+2 more…") {
		t.Errorf("expected 5-member combo to render +2 more…, got: %s", body)
	}
	// Pins stripped in chips: glm@work:glm-4.7 -> glm:glm-4.7
	if !strings.Contains(body, "glm:glm-4.7") {
		t.Errorf("expected stripped glm:glm-4.7 in body, got: %s", body)
	}
	if strings.Contains(body, "glm@work:glm-4.7") {
		t.Errorf("expected pin glm@work to NOT be rendered in card chips, got: %s", body)
	}
	// Duplicates render as-is: openai:gpt-4o appears
	if !strings.Contains(body, "openai:gpt-4o") {
		t.Errorf("expected openai:gpt-4o chip in body, got: %s", body)
	}
	// Hover title lists verbatim hidden members: anthropic@team:claude-sonnet-4.5, meta:llama-3
	if !strings.Contains(body, "title=\"anthropic@team:claude-sonnet-4.5, meta:llama-3\"") {
		t.Errorf("expected hover title for 5-member combo to list hidden members with pins, got: %s", body)
	}

	// 5. Disabled card styling and switch aria-checked
	if !strings.Contains(body, "opacity-60") {
		t.Errorf("expected disabled card to carry opacity-60 class, got: %s", body)
	}
	if !strings.Contains(body, "aria-checked=\"false\"") {
		t.Errorf("expected disabled switch to have aria-checked=false, got: %s", body)
	}
	if !strings.Contains(body, "aria-checked=\"true\"") {
		t.Errorf("expected enabled switch to have aria-checked=true, got: %s", body)
	}
	if !strings.Contains(body, "action=\"/dashboard/combos/toggle\"") {
		t.Errorf("expected footer toggle form pointing to /dashboard/combos/toggle, got: %s", body)
	}
}
