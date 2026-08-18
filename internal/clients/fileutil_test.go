package clients

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	if got := expandHome("~/test/config.json"); got != filepath.Join(tmpHome, "test/config.json") {
		t.Errorf("expandHome(~/test/config.json) = %q, want %q", got, filepath.Join(tmpHome, "test/config.json"))
	}
	if got := expandHome("~"); got != tmpHome {
		t.Errorf("expandHome(~) = %q, want %q", got, tmpHome)
	}
	if got := expandHome("/absolute/path"); got != "/absolute/path" {
		t.Errorf("expandHome(/absolute/path) = %q, want /absolute/path", got)
	}
}

func TestAtomicWriteAndBackup(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "sub", "config.json")

	// 1. Backup non-existent file
	bak, err := backup(targetPath)
	if err != nil {
		t.Fatalf("backup non-existent file: %v", err)
	}
	if bak != "" {
		t.Errorf("expected empty backup path for non-existent file, got %q", bak)
	}

	// 2. Atomic write initial content
	content := []byte(`{"key": "value"}`)
	if err := atomicWrite(targetPath, content, 0600); err != nil {
		t.Fatalf("atomicWrite initial: %v", err)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat targetPath: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// 3. Backup existing file
	bakPath, err := backup(targetPath)
	if err != nil {
		t.Fatalf("backup existing file: %v", err)
	}
	expectedBak := targetPath + ".tinyroute.bak"
	if bakPath != expectedBak {
		t.Errorf("backup path = %q, want %q", bakPath, expectedBak)
	}

	bakData, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(bakData) != string(content) {
		t.Errorf("backup content = %q, want %q", string(bakData), string(content))
	}

	// 4. Overwrite file atomically
	newContent := []byte(`{"key": "new_value"}`)
	if err := atomicWrite(targetPath, newContent, 0600); err != nil {
		t.Fatalf("atomicWrite update: %v", err)
	}
	updatedData, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read updated targetPath: %v", err)
	}
	if string(updatedData) != string(newContent) {
		t.Errorf("updated content = %q, want %q", string(updatedData), string(newContent))
	}
}
