package route_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/route"
)

func TestHealthStore(t *testing.T) {
	clock := &route.FakeClock{T: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	hs := route.NewHealthStore(clock, statePath)

	if !hs.Available("prov-1") {
		t.Errorf("expected prov-1 available initially")
	}

	hs.Penalize("prov-1", 10*time.Second)
	if hs.Available("prov-1") {
		t.Errorf("expected prov-1 unavailable during cooldown")
	}

	if hs.CooldownEnd("prov-1").IsZero() {
		t.Errorf("expected non-zero CooldownEnd")
	}

	active := hs.ActiveCooldowns()
	if len(active) != 1 || active["prov-1"].IsZero() {
		t.Errorf("expected 1 active cooldown in snapshot")
	}

	if err := hs.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load in a fresh store
	hs2 := route.NewHealthStore(clock, statePath)
	if err := hs2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if hs2.Available("prov-1") {
		t.Errorf("expected loaded store to preserve active cooldown")
	}

	// Advance clock past cooldown
	clock.Advance(20 * time.Second)
	if !hs.Available("prov-1") {
		t.Errorf("expected prov-1 available after clock advance")
	}

	hs.ClearStrikes("prov-1")
}

func TestHealthStorePerModelIsolation(t *testing.T) {
	clock := &route.FakeClock{T: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	hs := route.NewHealthStore(clock, statePath)

	key := "openai/primary"
	modelA := "gpt-4o"
	modelB := "gpt-4o-mini"

	// Initially available
	if !hs.AvailableModel(key, modelA) || !hs.AvailableModel(key, modelB) {
		t.Fatalf("expected both models available initially")
	}

	// Penalize modelA only
	hs.PenalizeModel(key, modelA, 30*time.Second)

	if hs.AvailableModel(key, modelA) {
		t.Errorf("expected modelA to be in cooldown")
	}
	if !hs.AvailableModel(key, modelB) {
		t.Errorf("expected modelB to remain available when modelA is penalized")
	}

	// Save and reload
	_ = hs.Save()
	hsReload := route.NewHealthStore(clock, statePath)
	_ = hsReload.Load()

	if hsReload.AvailableModel(key, modelA) {
		t.Errorf("expected reloaded store to preserve per-model cooldown for modelA")
	}
	if !hsReload.AvailableModel(key, modelB) {
		t.Errorf("expected reloaded store to preserve availability for modelB")
	}
}

func TestMemoryAffinity(t *testing.T) {
	aff := route.NewMemoryAffinity()

	key := "openai/acc1"
	if aff.Count(key) != 0 {
		t.Errorf("expected initial count 0, got %d", aff.Count(key))
	}

	if aff.Touch(key) != 1 {
		t.Errorf("expected count 1 after first touch")
	}
	if aff.Touch(key) != 2 {
		t.Errorf("expected count 2 after second touch")
	}

	aff.Reset(key)
	if aff.Count(key) != 0 {
		t.Errorf("expected count 0 after reset, got %d", aff.Count(key))
	}
}

func TestOrderedSelector(t *testing.T) {
	sel := &route.OrderedSelector{}
	hops := []core.Hop{
		{Provider: "p1", Model: "m1"},
		{Provider: "p2", Model: "m2"},
	}

	available := func(provider string) bool {
		return provider == "p2"
	}

	got := sel.Select(hops, available)
	if len(got) != 1 || got[0].Provider != "p2" {
		t.Errorf("unexpected selection: %+v", got)
	}
}

func TestRealClock(t *testing.T) {
	rc := route.RealClock{}
	if rc.Now().IsZero() {
		t.Errorf("RealClock.Now() returned zero time")
	}
}
