package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotenvDoesNotOverrideExistingEnvironment(t *testing.T) {
	t.Setenv("TINYROUTE_DOTENV_PRECEDENCE", "process-value")
	path := writeEnvFile(t, t.TempDir(), "TINYROUTE_DOTENV_PRECEDENCE=file-value\n")

	if err := LoadDotenv(path); err != nil {
		t.Fatalf("LoadDotenv() error = %v", err)
	}
	if got := os.Getenv("TINYROUTE_DOTENV_PRECEDENCE"); got != "process-value" {
		t.Errorf("TINYROUTE_DOTENV_PRECEDENCE = %q, want process-value", got)
	}
}

func TestDiscoverEnvFileOrder(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	explicitPath := writeEnvFile(t, t.TempDir(), "TINYROUTE_DOTENV_SOURCE=explicit\n")
	writeEnvFile(t, workDir, "TINYROUTE_DOTENV_SOURCE=current\n")
	homePath := filepath.Join(homeDir, ".tinyroute", ".env")
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(homePath, []byte("TINYROUTE_DOTENV_SOURCE=home\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	changeDir(t, workDir)
	t.Setenv("HOME", homeDir)

	if got := discoverEnvFile(explicitPath); got != explicitPath {
		t.Errorf("discoverEnvFile(explicit) = %q, want %q", got, explicitPath)
	}
	if got := discoverEnvFile(""); got != ".env" {
		t.Errorf("discoverEnvFile() with current file = %q, want .env", got)
	}
	if err := os.Remove(filepath.Join(workDir, ".env")); err != nil {
		t.Fatalf("Remove(current .env) error = %v", err)
	}
	if got := discoverEnvFile(""); got != homePath {
		t.Errorf("discoverEnvFile() with only home file = %q, want %q", got, homePath)
	}
}

func TestLoadDotenvNoFileIsNotAnError(t *testing.T) {
	changeDir(t, t.TempDir())
	t.Setenv("HOME", t.TempDir())

	if err := LoadDotenv(""); err != nil {
		t.Fatalf("LoadDotenv() error = %v, want nil", err)
	}
}

func TestLoadDotenvParsesQuotedCommentsAndExport(t *testing.T) {
	path := writeEnvFile(t, t.TempDir(), `
# a comment

TINYROUTE_DOTENV_DOUBLE="double quoted"
TINYROUTE_DOTENV_SINGLE='single quoted'
export TINYROUTE_DOTENV_EXPORT=exported
  # an indented comment

`)

	for _, key := range []string{
		"TINYROUTE_DOTENV_DOUBLE",
		"TINYROUTE_DOTENV_SINGLE",
		"TINYROUTE_DOTENV_EXPORT",
	} {
		os.Unsetenv(key)
		t.Cleanup(func() { os.Unsetenv(key) })
	}

	if err := LoadDotenv(path); err != nil {
		t.Fatalf("LoadDotenv() error = %v", err)
	}

	for key, want := range map[string]string{
		"TINYROUTE_DOTENV_DOUBLE": "double quoted",
		"TINYROUTE_DOTENV_SINGLE": "single quoted",
		"TINYROUTE_DOTENV_EXPORT": "exported",
	} {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func changeDir(t *testing.T, dir string) {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}

func writeEnvFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}
