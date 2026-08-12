package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllAdaptersRegistry(t *testing.T) {
	all := All()
	if len(all) != 13 {
		t.Fatalf("expected 13 registered adapters, got %d", len(all))
	}

	expectedIDs := []string{
		"claude", "cline", "codex", "copilot", "deepseek", "devin", "droid",
		"grok", "hermes", "jcode", "kilo", "openclaw", "opencode",
	}

	for i, id := range expectedIDs {
		if all[i].ID() != id {
			t.Errorf("adapter[%d].ID() = %q, want %q", i, all[i].ID(), id)
		}
		_, found := Get(id)
		if !found {
			t.Errorf("Get(%q) not found in registry", id)
		}
	}

	// Verify deferred adapters are absent
	for _, deferred := range []string{"cowork", "antigravity-mitm"} {
		if _, found := Get(deferred); found {
			t.Errorf("deferred adapter %q should not be registered", deferred)
		}
	}
}

func TestClineAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, ok := Get("cline")
	if !ok {
		t.Fatalf("cline adapter not found")
	}

	res, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai/v1",
		APIKey:  "tr_live_cline",
		Model:   "gpt-4o",
	})
	if err != nil {
		t.Fatalf("Apply cline: %v", err)
	}
	if len(res.Files) != 2 {
		t.Fatalf("expected 2 files for cline, got %d", len(res.Files))
	}

	st, err := a.Detect()
	if err != nil || !st.PointedAtTinyRoute {
		t.Errorf("expected pointed at tinyroute for cline, st=%+v, err=%v", st, err)
	}

	if err := a.Reset(); err != nil {
		t.Fatalf("Reset cline: %v", err)
	}

	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected not pointed at tinyroute after reset for cline")
	}
}

func TestCopilotAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, ok := Get("copilot")
	if !ok {
		t.Fatalf("copilot adapter not found")
	}

	res, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai",
		APIKey:  "tr_live_copilot",
		Model:   "gpt-4o",
	})
	if err != nil {
		t.Fatalf("Apply copilot: %v", err)
	}

	st, err := a.Detect()
	if err != nil || !st.PointedAtTinyRoute {
		t.Errorf("expected pointed at tinyroute for copilot, st=%+v, err=%v", st, err)
	}

	data, err := os.ReadFile(st.ConfigPath)
	if err != nil {
		t.Fatalf("read copilot config: %v", err)
	}
	if !strings.Contains(string(data), `"customendpoint"`) {
		t.Errorf("copilot config missing customendpoint vendor, got: %s", string(data))
	}
	if strings.Contains(string(data), "#models.ai.azure.com") {
		t.Errorf("copilot config contains azure fragment, got: %s", string(data))
	}

	if err := a.Reset(); err != nil {
		t.Fatalf("Reset copilot: %v", err)
	}
	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected not pointed after reset for copilot")
	}
	_ = res
}

func TestDeepseekAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, _ := Get("deepseek")
	_, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai",
		APIKey:  "tr_live_ds",
		Model:   "deepseek-coder",
	})
	if err != nil {
		t.Fatalf("Apply deepseek: %v", err)
	}

	st, _ := a.Detect()
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed for deepseek")
	}

	_ = a.Reset()
	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected reset for deepseek")
	}
}

func TestDevinAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, _ := Get("devin")
	st, err := a.Detect()
	if err != nil {
		t.Fatalf("Detect devin: %v", err)
	}
	if st.PointedAtTinyRoute {
		t.Errorf("devin should not point at tinyroute")
	}
	_, err = a.Apply(ApplyInput{})
	if err != nil {
		t.Fatalf("Apply devin: %v", err)
	}
	_ = a.Reset()
}

func TestDroidAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, _ := Get("droid")
	_, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai",
		APIKey:  "tr_live_droid",
		Model:   "gpt-4o",
	})
	if err != nil {
		t.Fatalf("Apply droid: %v", err)
	}

	st, _ := a.Detect()
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed for droid")
	}

	_ = a.Reset()
	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected reset for droid")
	}
}

func TestGrokAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, _ := Get("grok")
	_, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai",
		APIKey:  "tr_live_grok",
		Model:   "grok-beta",
	})
	if err != nil {
		t.Fatalf("Apply grok: %v", err)
	}

	st, _ := a.Detect()
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed for grok")
	}

	_ = a.Reset()
	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected reset for grok")
	}
}

func TestHermesAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, _ := Get("hermes")
	_, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai",
		APIKey:  "tr_live_hermes",
		Model:   "hermes-3",
	})
	if err != nil {
		t.Fatalf("Apply hermes: %v", err)
	}

	st, _ := a.Detect()
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed for hermes")
	}

	_ = a.Reset()
	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected reset for hermes")
	}
}

func TestJcodeAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, _ := Get("jcode")
	_, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai",
		APIKey:  "tr_live_jcode",
		Model:   "gpt-4o",
	})
	if err != nil {
		t.Fatalf("Apply jcode: %v", err)
	}

	st, _ := a.Detect()
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed for jcode")
	}

	_ = a.Reset()
	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected reset for jcode")
	}
}

func TestKiloAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, _ := Get("kilo")
	_, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai",
		APIKey:  "tr_live_kilo",
		Model:   "kilo-v1",
	})
	if err != nil {
		t.Fatalf("Apply kilo: %v", err)
	}

	st, _ := a.Detect()
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed for kilo")
	}

	kiloPath := expandHome("~/.config/kilo/kilo.json")
	dataKilo, err := os.ReadFile(kiloPath)
	if err != nil {
		t.Fatalf("read kilo.json: %v", err)
	}
	if !strings.Contains(string(dataKilo), `"tinyroute"`) {
		t.Errorf("kilo.json missing tinyroute provider, got: %s", string(dataKilo))
	}

	_ = a.Reset()
	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected reset for kilo")
	}
}

func TestOpenclawAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, _ := Get("openclaw")
	_, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai",
		APIKey:  "tr_live_openclaw",
		Model:   "claw-v1",
	})
	if err != nil {
		t.Fatalf("Apply openclaw: %v", err)
	}

	st, _ := a.Detect()
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed for openclaw")
	}

	_ = a.Reset()
	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected reset for openclaw")
	}
}

func TestOpencodeAdapter(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	a, _ := Get("opencode")
	_, err := a.Apply(ApplyInput{
		BaseURL: "http://localhost:8080/openai",
		APIKey:  "tr_live_opencode",
		Model:   "gpt-4o",
	})
	if err != nil {
		t.Fatalf("Apply opencode: %v", err)
	}

	st, _ := a.Detect()
	if !st.PointedAtTinyRoute {
		t.Errorf("expected pointed for opencode")
	}

	_ = a.Reset()
	stReset, _ := a.Detect()
	if stReset.PointedAtTinyRoute {
		t.Errorf("expected reset for opencode")
	}
}

func TestAdapterMetadataMethods(t *testing.T) {
	for _, a := range All() {
		if a.Name() == "" {
			t.Errorf("adapter %s returned empty Name", a.ID())
		}
		if a.Dialect() == "" {
			t.Errorf("adapter %s returned empty Dialect", a.ID())
		}
		_ = a.NeedsModel()
		_ = a.ModelSlots()
	}
}

func testHelperUnusedImportsFix(path string) {
	_ = filepath.Base(path)
	_ = strings.TrimSpace(path)
	_ = os.DevNull
}
