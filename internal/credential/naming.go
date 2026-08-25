package credential

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// ValidateAccountName validates that name matches [A-Za-z0-9._@-], has no slashes or whitespace,
// is non-empty, and is at most 64 characters long.
func ValidateAccountName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("account name cannot be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("account name %q exceeds maximum length of 64 characters", name)
	}
	for _, r := range name {
		if !isValidAccountRune(r) {
			return fmt.Errorf("account name %q contains invalid character %q (allowed: letters, digits, '.', '_', '@', '-')", name, string(r))
		}
	}
	return nil
}

func isValidAccountRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '@' || r == '-'
}

// SanitizeDerivedName cleans an identity claim into a valid account name by converting to lowercase,
// removing illegal characters, and truncating to 64 characters. Returns "" if unsalvageable.
func SanitizeDerivedName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if isValidAccountRune(r) {
			b.WriteRune(r)
		}
	}
	res := b.String()
	if len(res) > 64 {
		res = res[:64]
	}
	if ValidateAccountName(res) != nil {
		return ""
	}
	return res
}

// ResolveAccount picks the target account name for a credential write:
//  1. explicit label (validated; collision updates in place)
//  2. identity hint (sanitized; collision falls through to stage 3)
//  3. first free slot starting at account-2 (or "default" if no existing accounts)
func ResolveAccount(provider, explicit, identityHint string, existing []string) (string, error) {
	if explicit != "" {
		if err := ValidateAccountName(explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}

	if identityHint != "" {
		derived := SanitizeDerivedName(identityHint)
		if derived != "" && ValidateAccountName(derived) == nil {
			if !slices.Contains(existing, derived) {
				return derived, nil
			}
		}
	}

	if len(existing) == 0 {
		return "default", nil
	}

	for i := 2; ; i++ {
		candidate := fmt.Sprintf("account-%d", i)
		if !slices.Contains(existing, candidate) {
			return candidate, nil
		}
	}
}
