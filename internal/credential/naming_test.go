package credential

import (
	"strings"
	"testing"
)

func TestValidateAccountName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "my-account", false},
		{"valid with symbols", "user.name_123@domain", false},
		{"valid letters and digits", "Account1", false},
		{"empty string", "", true},
		{"contains slash", "foo/bar", true},
		{"contains backslash", "foo\\bar", true},
		{"contains whitespace", "foo bar", true},
		{"contains tab", "foo\tbar", true},
		{"contains newline", "foo\nbar", true},
		{"contains forbidden char colons", "foo:bar", true},
		{"contains special char exclamation", "foo!bar", true},
		{"length 64 is ok", strings.Repeat("a", 64), false},
		{"length 65 is rejected", strings.Repeat("a", 65), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAccountName(%q) err = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeDerivedName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple email", "User.Name@Example.COM", "user.name@example.com"},
		{"sub with invalid chars", "auth0|123456!#$", "auth0123456"},
		{"slashes and spaces", "foo / bar / baz", "foobarbaz"},
		{"already clean", "john_doe-123", "john_doe-123"},
		{"all invalid characters", "!#$%^&*()+={}[]|:;\"'<>,?/", ""},
		{"empty input", "", ""},
		{"truncates over 64 chars", strings.Repeat("a", 70), strings.Repeat("a", 64)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeDerivedName(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeDerivedName(%q) = %q, expected %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestResolveAccount(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		explicit     string
		identityHint string
		existing     []string
		want         string
		wantErr      bool
	}{
		{
			name:         "explicit label wins",
			provider:     "openai",
			explicit:     "work",
			identityHint: "jane@example.com",
			existing:     []string{"default"},
			want:         "work",
			wantErr:      false,
		},
		{
			name:         "explicit label invalid returns error",
			provider:     "openai",
			explicit:     "work/invalid",
			identityHint: "jane@example.com",
			existing:     nil,
			want:         "",
			wantErr:      true,
		},
		{
			name:         "explicit label collision updates in place",
			provider:     "openai",
			explicit:     "work",
			identityHint: "other@example.com",
			existing:     []string{"work", "default"},
			want:         "work",
			wantErr:      false,
		},
		{
			name:         "identity hint used when no explicit label",
			provider:     "github",
			explicit:     "",
			identityHint: "Jane.Doe@Example.com",
			existing:     []string{"default"},
			want:         "jane.doe@example.com",
			wantErr:      false,
		},
		{
			name:         "identity hint collision falls back to next free slot starting at account-2",
			provider:     "github",
			explicit:     "",
			identityHint: "jane@example.com",
			existing:     []string{"default", "jane@example.com"},
			want:         "account-2",
			wantErr:      false,
		},
		{
			name:         "no explicit, no hint, no existing returns default",
			provider:     "google",
			explicit:     "",
			identityHint: "",
			existing:     nil,
			want:         "default",
			wantErr:      false,
		},
		{
			name:         "no explicit, no hint, with existing returns first free slot starting at account-2",
			provider:     "google",
			explicit:     "",
			identityHint: "",
			existing:     []string{"default"},
			want:         "account-2",
			wantErr:      false,
		},
		{
			name:         "slot skips existing account-2 and picks account-3",
			provider:     "google",
			explicit:     "",
			identityHint: "",
			existing:     []string{"default", "account-2"},
			want:         "account-3",
			wantErr:      false,
		},
		{
			name:         "unsalvageable hint falls through to slot",
			provider:     "google",
			explicit:     "",
			identityHint: "!!!",
			existing:     []string{"default"},
			want:         "account-2",
			wantErr:      false,
		},
		{
			name:         "sparse slot filling fills lowest free slot",
			provider:     "google",
			explicit:     "",
			identityHint: "",
			existing:     []string{"default", "account-2", "account-4"},
			want:         "account-3",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveAccount(tt.provider, tt.explicit, tt.identityHint, tt.existing)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveAccount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ResolveAccount() = %q, want %q", got, tt.want)
			}
		})
	}
}
