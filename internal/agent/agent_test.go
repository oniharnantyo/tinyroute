package agent

import (
	"testing"
)

type dummyAgent struct {
	id      string
	name    string
	dialect string
}

func (d *dummyAgent) ID() string                       { return d.id }
func (d *dummyAgent) Name() string                     { return d.name }
func (d *dummyAgent) Dialect() string                  { return d.dialect }
func (d *dummyAgent) NeedsModel() bool                 { return false }
func (d *dummyAgent) ModelSlots() []ModelSlot          { return nil }
func (d *dummyAgent) Detect() (Status, error)          { return Status{}, nil }
func (d *dummyAgent) Apply(ApplyInput) (Result, error) { return Result{}, nil }
func (d *dummyAgent) Reset() error                     { return nil }

func TestRegistry(t *testing.T) {
	a1 := &dummyAgent{id: "agent_b", name: "Agent B", dialect: "openai"}
	a2 := &dummyAgent{id: "agent_a", name: "Agent A", dialect: "anthropic"}

	Register(a1)
	Register(a2)

	gotA, ok := Get("agent_a")
	if !ok || gotA.Name() != "Agent A" {
		t.Fatalf("expected Agent A, got %v (ok=%v)", gotA, ok)
	}

	_, ok = Get("nonexistent")
	if ok {
		t.Fatalf("expected nonexistent to fail")
	}

	all := All()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 agents in All(), got %d", len(all))
	}

	// Verify sorting order in All()
	var foundA, foundB bool
	for _, a := range all {
		if a.ID() == "agent_a" {
			foundA = true
		}
		if a.ID() == "agent_b" {
			foundB = true
			if !foundA {
				t.Errorf("expected agent_a before agent_b in All()")
			}
		}
	}
	if !foundA || !foundB {
		t.Errorf("registered agents not found in All()")
	}
}
