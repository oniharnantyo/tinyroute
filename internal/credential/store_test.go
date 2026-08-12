package credential

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_AccountReKeyingAndMigration(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")

	// Write legacy credentials file with single provider keys
	legacyJSON := `{
		"credentials": {
			"codex": {
				"provider": "codex",
				"refresh_token": "rt-legacy-123"
			}
		}
	}`

	if err := os.WriteFile(credPath, []byte(legacyJSON), 0600); err != nil {
		t.Fatalf("failed to write legacy file: %v", err)
	}

	store, err := NewStore(credPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// Should be retrievable via legacy key "codex" and composite key "codex/default"
	rec, ok := store.Get("codex")
	if !ok {
		t.Fatalf("expected to find legacy record for codex")
	}
	if rec.RefreshToken != "rt-legacy-123" {
		t.Errorf("expected rt-legacy-123, got %s", rec.RefreshToken)
	}
	if rec.Account != "default" {
		t.Errorf("expected account 'default', got %s", rec.Account)
	}

	recAcc, ok := store.GetAccount("codex", "default")
	if !ok || recAcc.RefreshToken != "rt-legacy-123" {
		t.Errorf("expected GetAccount to work for migrated legacy record")
	}

	// Save a multi-account record
	newRec := OAuthRecord{
		Provider:     "codex",
		Account:      "work",
		RefreshToken: "rt-work-456",
	}
	if err := store.Save(newRec); err != nil {
		t.Fatalf("failed to save work account: %v", err)
	}

	workRec, ok := store.GetAccount("codex", "work")
	if !ok || workRec.RefreshToken != "rt-work-456" {
		t.Errorf("expected to retrieve codex/work record")
	}

	// ListMasked should mask sensitive fields
	maskedList := store.ListMasked()
	if len(maskedList) != 2 {
		t.Errorf("expected 2 records in store, got %d", len(maskedList))
	}
	for _, r := range maskedList {
		if r.RefreshToken == "rt-legacy-123" || r.RefreshToken == "rt-work-456" {
			t.Errorf("expected refresh token to be masked in ListMasked, got %s", r.RefreshToken)
		}
	}
}
