package clients

import (
	"testing"
)

type dummyClient struct {
	id      string
	name    string
	dialect string
}

func (d *dummyClient) ID() string                       { return d.id }
func (d *dummyClient) Name() string                     { return d.name }
func (d *dummyClient) Dialect() string                  { return d.dialect }
func (d *dummyClient) NeedsModel() bool                 { return false }
func (d *dummyClient) ModelSlots() []ModelSlot          { return nil }
func (d *dummyClient) Detect() (Status, error)          { return Status{}, nil }
func (d *dummyClient) Apply(ApplyInput) (Result, error) { return Result{}, nil }
func (d *dummyClient) Reset() error                     { return nil }

func TestRegistry(t *testing.T) {
	a1 := &dummyClient{id: "agent_b", name: "Client B", dialect: "openai"}
	a2 := &dummyClient{id: "agent_a", name: "Client A", dialect: "anthropic"}

	Register(a1)
	Register(a2)

	gotA, ok := Get("agent_a")
	if !ok || gotA.Name() != "Client A" {
		t.Fatalf("expected Client A, got %v (ok=%v)", gotA, ok)
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
