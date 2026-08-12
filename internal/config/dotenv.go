package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotenv loads a .env file following discovery order.
// envFile is the explicit --env-file path (empty means use discovery).
// It sets values into the process environment only when not already present.
func LoadDotenv(envFile string) error {
	path := discoverEnvFile(envFile)
	if path == "" {
		return nil // no file found, not an error
	}
	return loadEnvFile(path)
}

func discoverEnvFile(explicit string) string {
	if explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit
		}
		return ""
	}
	// Try ./.env
	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	// Try ~/.tinyroute/.env
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".tinyroute", ".env")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip optional export prefix
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		// Strip quotes
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// Only set if not already present in environment
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}
