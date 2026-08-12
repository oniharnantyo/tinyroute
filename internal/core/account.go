package core

import (
	"strings"
	"sync/atomic"
)

type AccountStrategy string

const (
	StrategyRoundRobin       AccountStrategy = "round_robin"
	StrategyFillFirst        AccountStrategy = "fill_first"
	StrategySticky           AccountStrategy = "sticky"
	StrategyStickyRoundRobin AccountStrategy = "sticky_round_robin"
)

// HopAccount identifies a specific provider account and model.
type HopAccount struct {
	Provider string
	Account  string
	Model    string
}

// Affinity tracks consecutive uses of accounts for sticky round-robin strategy.
type Affinity interface {
	Count(key string) int
	Touch(key string) int
	Reset(key string)
}

// SelectAccounts selects and orders available accounts according to strategy.
// available is a predicate checking whether an account ("provider/account" or account name) is healthy.
// counter is an atomic counter used for round_robin/sticky rotation.
func SelectAccounts(accounts []string, strategy AccountStrategy, available func(account string) bool, counter *uint64) []string {
	return SelectAccountsAffinity(accounts, strategy, available, counter, nil, 0, "")
}

// SelectAccountsAffinity extends SelectAccounts to support sticky_round_robin with affinity counters and consecutive limit.
func SelectAccountsAffinity(accounts []string, strategy AccountStrategy, available func(account string) bool, counter *uint64, affinity Affinity, limit int, provider string) []string {
	if len(accounts) == 0 {
		return nil
	}

	var healthy []string
	for _, acc := range accounts {
		if available == nil || available(acc) {
			healthy = append(healthy, acc)
		}
	}
	if len(healthy) == 0 {
		return nil
	}

	switch strategy {
	case StrategyFillFirst:
		return healthy
	case StrategySticky:
		if counter != nil {
			idx := int(atomic.LoadUint64(counter) % uint64(len(healthy)))
			res := make([]string, 0, len(healthy))
			res = append(res, healthy[idx])
			for i, acc := range healthy {
				if i != idx {
					res = append(res, acc)
				}
			}
			return res
		}
		return healthy
	case StrategyStickyRoundRobin:
		if counter != nil && affinity != nil && limit > 0 {
			currIdx := int(atomic.LoadUint64(counter) % uint64(len(healthy)))
			pinnedAcc := healthy[currIdx]
			pinnedKey := pinnedAcc
			if provider != "" && !strings.Contains(pinnedAcc, "/") {
				pinnedKey = provider + "/" + pinnedAcc
			}

			if affinity.Count(pinnedKey) >= limit {
				affinity.Reset(pinnedKey)
				currIdx = int(atomic.AddUint64(counter, 1) % uint64(len(healthy)))
				pinnedAcc = healthy[currIdx]
			}

			res := make([]string, 0, len(healthy))
			res = append(res, pinnedAcc)
			for _, acc := range healthy {
				if acc != pinnedAcc {
					res = append(res, acc)
				}
			}
			return res
		}
		fallthrough
	case StrategyRoundRobin:
		fallthrough
	default:
		if counter != nil {
			n := atomic.AddUint64(counter, 1) - 1
			idx := int(n % uint64(len(healthy)))
			res := make([]string, 0, len(healthy))
			res = append(res, healthy[idx:]...)
			res = append(res, healthy[:idx]...)
			return res
		}
		return healthy
	}
}
