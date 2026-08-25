package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var comboNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)

// ValidateComboName validates the charset, forbids colons, and checks uniqueness against existing combos.
func ValidateComboName(name string, existingCombos []Combo) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("combo name cannot be empty")
	}
	if name == "combo" {
		return fmt.Errorf("combo name cannot be \"combo\"")
	}
	if strings.Contains(name, ":") {
		return fmt.Errorf("combo name cannot contain ':'")
	}
	if !comboNameRegex.MatchString(name) {
		return fmt.Errorf("combo name can only contain letters, numbers, '.', '-', and '_'")
	}
	for _, c := range existingCombos {
		if c.Name == name {
			return fmt.Errorf("combo %q already exists", name)
		}
	}
	return nil
}

// GetModelCandidates derives unpinned provider:model candidates from parsed topology provider whitelists.
// Each model is offered once per provider. Providers with no whitelisted models are skipped.
// Returned in deterministic sorted order by provider name.
func GetModelCandidates(topo Topology) []string {
	if len(topo.Providers) == 0 {
		return []string{}
	}
	provNames := make([]string, 0, len(topo.Providers))
	for name := range topo.Providers {
		provNames = append(provNames, name)
	}
	sort.Strings(provNames)

	var candidates []string
	for _, pName := range provNames {
		p := topo.Providers[pName]
		if len(p.Models) == 0 {
			continue
		}
		for _, m := range p.Models {
			candidates = append(candidates, fmt.Sprintf("%s:%s", pName, m))
		}
	}
	return candidates
}

// GetAccountOptions returns provider@account options for providers declaring >=2 accounts, sorted.
func GetAccountOptions(topo Topology) []string {
	if len(topo.Providers) == 0 {
		return []string{}
	}
	provNames := make([]string, 0, len(topo.Providers))
	for name := range topo.Providers {
		provNames = append(provNames, name)
	}
	sort.Strings(provNames)

	var options []string
	for _, pName := range provNames {
		p := topo.Providers[pName]
		if len(p.Accounts) < 2 {
			continue
		}
		accNames := make([]string, 0, len(p.Accounts))
		for _, acc := range p.Accounts {
			accNames = append(accNames, acc.Name)
		}
		sort.Strings(accNames)
		for _, accName := range accNames {
			options = append(options, fmt.Sprintf("%s@%s", pName, accName))
		}
	}
	return options
}

// GetMemberCandidates derives provider:model and provider@account:model candidates
// from parsed topology provider whitelists.
// For providers declaring >=2 accounts, models are offered unpinned and once per account.
// For providers with 0-1 accounts, models are offered unpinned only.
// Providers with no whitelisted models are skipped.
func GetMemberCandidates(topo Topology) []string {
	if len(topo.Providers) == 0 {
		return []string{}
	}
	models := GetModelCandidates(topo)
	accountOptions := GetAccountOptions(topo)
	if len(accountOptions) == 0 {
		return models
	}

	provAccounts := make(map[string][]string)
	for _, opt := range accountOptions {
		parts := strings.SplitN(opt, "@", 2)
		if len(parts) == 2 {
			provAccounts[parts[0]] = append(provAccounts[parts[0]], parts[1])
		}
	}

	provNames := make([]string, 0, len(topo.Providers))
	for name := range topo.Providers {
		provNames = append(provNames, name)
	}
	sort.Strings(provNames)

	var candidates []string
	for _, pName := range provNames {
		p := topo.Providers[pName]
		if len(p.Models) == 0 {
			continue
		}
		for _, m := range p.Models {
			candidates = append(candidates, fmt.Sprintf("%s:%s", pName, m))
		}
		if accs, ok := provAccounts[pName]; ok {
			for _, accName := range accs {
				for _, m := range p.Models {
					candidates = append(candidates, fmt.Sprintf("%s@%s:%s", pName, accName, m))
				}
			}
		}
	}
	return candidates
}

// RenameComboAccount rewrites provider@oldAcc: -> provider@newAcc: across all combo members.
func RenameComboAccount(combos []Combo, providerName, oldAcc, newAcc string) []Combo {
	oldPrefix := providerName + "@" + oldAcc + ":"
	newPrefix := providerName + "@" + newAcc + ":"
	newCombos := make([]Combo, len(combos))
	for i, c := range combos {
		newMembers := make([]string, len(c.Members))
		for j, m := range c.Members {
			if strings.HasPrefix(m, oldPrefix) {
				newMembers[j] = newPrefix + strings.TrimPrefix(m, oldPrefix)
			} else {
				newMembers[j] = m
			}
		}
		c.Members = newMembers
		newCombos[i] = c
	}
	return newCombos
}

// DowngradeComboAccount downgrades provider@acc:model to provider:model across all combos.
// If the downgraded form is already a member of the same combo, it is dropped.
// Every combo keeps at least one member (downgrade preserves provider and model,
// and dedup only removes exact duplicates), so no combo is ever removed.
// Returns the updated combos slice and the names of modified combos.
func DowngradeComboAccount(combos []Combo, providerName, accName string) ([]Combo, []string) {
	prefix := providerName + "@" + accName + ":"
	unpinnedPrefix := providerName + ":"
	var newCombos []Combo
	var modified []string

	for _, c := range combos {
		hasPin := false
		for _, m := range c.Members {
			if strings.HasPrefix(m, prefix) {
				hasPin = true
				break
			}
		}
		if !hasPin {
			newCombos = append(newCombos, c)
			continue
		}

		var newMembers []string
		seen := make(map[string]bool)
		for _, m := range c.Members {
			var candidate string
			if strings.HasPrefix(m, prefix) {
				candidate = unpinnedPrefix + strings.TrimPrefix(m, prefix)
			} else {
				candidate = m
			}
			if !seen[candidate] {
				seen[candidate] = true
				newMembers = append(newMembers, candidate)
			}
		}

		c.Members = newMembers
		newCombos = append(newCombos, c)
		modified = append(modified, c.Name)
	}
	return newCombos, modified
}

// SplitAndTrim splits a comma-separated string and trims whitespace from each element.
func SplitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
