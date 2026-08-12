package core

import (
	"testing"
)

func TestSelectAccounts(t *testing.T) {
	accounts := []string{"acc1", "acc2", "acc3"}
	available := func(acc string) bool {
		return acc != "acc2" // acc2 is cooling down
	}

	// Fill first -> should return healthy accounts in original order
	resFill := SelectAccounts(accounts, StrategyFillFirst, available, nil)
	if len(resFill) != 2 || resFill[0] != "acc1" || resFill[1] != "acc3" {
		t.Errorf("expected [acc1, acc3], got %v", resFill)
	}

	// Round robin -> rotates across calls
	var counter uint64
	resRR1 := SelectAccounts(accounts, StrategyRoundRobin, available, &counter)
	if len(resRR1) != 2 || resRR1[0] != "acc1" || resRR1[1] != "acc3" {
		t.Errorf("expected [acc1, acc3] on first round, got %v", resRR1)
	}

	resRR2 := SelectAccounts(accounts, StrategyRoundRobin, available, &counter)
	if len(resRR2) != 2 || resRR2[0] != "acc3" || resRR2[1] != "acc1" {
		t.Errorf("expected [acc3, acc1] on second round, got %v", resRR2)
	}

	// Sticky -> keeps first selected sticky until counter changes
	var stickyCounter uint64
	resSticky := SelectAccounts(accounts, StrategySticky, available, &stickyCounter)
	if len(resSticky) != 2 || resSticky[0] != "acc1" {
		t.Errorf("expected acc1 sticky first, got %v", resSticky)
	}
}
