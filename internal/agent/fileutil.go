package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// expandHome expands a path starting with "~/" or equal to "~" using os.UserHomeDir().
func expandHome(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// backup creates a copy of the file at targetPath named targetPath + ".tinyroute.bak".
// If the target file does not exist, backup returns ("", nil).
// If successful, backup returns the path of the created backup file.
func backup(targetPath string) (string, error) {
	data, err := os.ReadFile(targetPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("agent: read file for backup %s: %w", targetPath, err)
	}

	bakPath := targetPath + ".tinyroute.bak"
	if err := atomicWrite(bakPath, data, 0600); err != nil {
		return "", fmt.Errorf("agent: write backup %s: %w", bakPath, err)
	}
	return bakPath, nil
}

// atomicWrite ensures target directory exists, writes data to a temporary file,
// and renames it to targetPath.
func atomicWrite(targetPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("agent: mkdir %s: %w", dir, err)
	}

	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("agent: write tmp %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, targetPath); err != nil {
		// Clean up tmp on rename failure
		_ = os.Remove(tmp)
		return fmt.Errorf("agent: rename %s -> %s: %w", tmp, targetPath, err)
	}

	return nil
}
