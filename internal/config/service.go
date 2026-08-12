package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Service holds immutable deployment settings parsed from the environment.
type Service struct {
	Listen                string
	ConfigPath            string
	KeysPath              string
	CredentialsPath       string
	StatePath             string
	HistoryPath           string // Deprecated: unused legacy JSONL path
	HistoryDBPath         string
	Capture               string // "full" or "metadata"
	LogFormat             string // "text" or "json"
	LogLevel              string // "debug", "info", "warn", or "error"
	InjectUsage           bool
	Cooldown429           time.Duration
	Cooldown5xx           time.Duration
	TrustProxy            bool
	TLSCert               string
	TLSKey                string
	DashboardEnable       bool
	DashboardListen       string
	DashboardPasswordPath string
}

// LoadService reads the recognized TINYROUTE_ environment variables and returns
// an immutable Service struct. It warns about unknown TINYROUTE_ variables.
// It fails (returns error) on malformed durations or incomplete TLS config.
func LoadService() (Service, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	defaultDir := filepath.Join(home, ".tinyroute")

	s := Service{
		Listen:                getEnvOr("TINYROUTE_LISTEN", "127.0.0.1:8787"),
		ConfigPath:            expandHome(getEnvOr("TINYROUTE_CONFIG", filepath.Join(defaultDir, "config.yaml")), home),
		KeysPath:              expandHome(getEnvOr("TINYROUTE_KEYS", filepath.Join(defaultDir, "keys.json")), home),
		CredentialsPath:       expandHome(getEnvOr("TINYROUTE_CREDENTIALS", filepath.Join(defaultDir, "credentials.json")), home),
		StatePath:             expandHome(getEnvOr("TINYROUTE_STATE", filepath.Join(defaultDir, "state.json")), home),
		HistoryPath:           expandHome(getEnvOr("TINYROUTE_HISTORY", filepath.Join(defaultDir, "requests.jsonl")), home),
		HistoryDBPath:         expandHome(getEnvOr("TINYROUTE_HISTORY_DB", filepath.Join(defaultDir, "history.db")), home),
		Capture:               getEnvOr("TINYROUTE_CAPTURE", "full"),
		LogFormat:             strings.ToLower(getEnvOr("TINYROUTE_LOG_FORMAT", "text")),
		LogLevel:              strings.ToLower(getEnvOr("TINYROUTE_LOG_LEVEL", "info")),
		TLSCert:               getEnvOr("TINYROUTE_TLS_CERT", ""),
		TLSKey:                getEnvOr("TINYROUTE_TLS_KEY", ""),
		DashboardListen:       getEnvOr("TINYROUTE_DASHBOARD_LISTEN", "127.0.0.1:8787"),
		DashboardPasswordPath: expandHome(getEnvOr("TINYROUTE_DASHBOARD_PASSWORD_PATH", filepath.Join(defaultDir, "dashboard.json")), home),
	}

	// Parse booleans.
	s.InjectUsage = parseBool(getEnvOr("TINYROUTE_INJECT_USAGE", "true"))
	s.TrustProxy = parseBool(getEnvOr("TINYROUTE_TRUST_PROXY", "false"))
	s.DashboardEnable = parseBool(getEnvOr("TINYROUTE_DASHBOARD", "true"))

	// Parse durations.
	d429, err := time.ParseDuration(getEnvOr("TINYROUTE_COOLDOWN_429", "60s"))
	if err != nil {
		return Service{}, fmt.Errorf("TINYROUTE_COOLDOWN_429: %q is not a valid duration", getEnvOr("TINYROUTE_COOLDOWN_429", "60s"))
	}
	s.Cooldown429 = d429

	d5xx, err := time.ParseDuration(getEnvOr("TINYROUTE_COOLDOWN_5XX", "10s"))
	if err != nil {
		return Service{}, fmt.Errorf("TINYROUTE_COOLDOWN_5XX: %q is not a valid duration", getEnvOr("TINYROUTE_COOLDOWN_5XX", "10s"))
	}
	s.Cooldown5xx = d5xx

	// Validate TLS pair.
	if (s.TLSCert == "") != (s.TLSKey == "") {
		return Service{}, fmt.Errorf("TINYROUTE_TLS_CERT and TINYROUTE_TLS_KEY must both be set or both be empty")
	}

	// Validate capture mode.
	if s.Capture != "full" && s.Capture != "metadata" {
		return Service{}, fmt.Errorf("TINYROUTE_CAPTURE: %q must be \"full\" or \"metadata\"", s.Capture)
	}

	// Validate log format.
	if s.LogFormat != "text" && s.LogFormat != "json" {
		return Service{}, fmt.Errorf("TINYROUTE_LOG_FORMAT: %q must be \"text\" or \"json\"", s.LogFormat)
	}

	// Validate log level.
	if s.LogLevel != "debug" && s.LogLevel != "info" && s.LogLevel != "warn" && s.LogLevel != "error" {
		return Service{}, fmt.Errorf("TINYROUTE_LOG_LEVEL: %q must be \"debug\", \"info\", \"warn\", or \"error\"", s.LogLevel)
	}

	// Warn about unknown TINYROUTE_ variables.
	known := map[string]bool{
		"TINYROUTE_LISTEN": true, "TINYROUTE_CONFIG": true, "TINYROUTE_KEYS": true,
		"TINYROUTE_CREDENTIALS": true, "TINYROUTE_STATE": true, "TINYROUTE_HISTORY": true, "TINYROUTE_HISTORY_DB": true,
		"TINYROUTE_CAPTURE": true, "TINYROUTE_INJECT_USAGE": true,
		"TINYROUTE_COOLDOWN_429": true, "TINYROUTE_COOLDOWN_5XX": true,
		"TINYROUTE_TRUST_PROXY": true, "TINYROUTE_TLS_CERT": true, "TINYROUTE_TLS_KEY": true,
		"TINYROUTE_LOG_FORMAT": true, "TINYROUTE_LOG_LEVEL": true,
		"TINYROUTE_DASHBOARD": true, "TINYROUTE_DASHBOARD_LISTEN": true, "TINYROUTE_DASHBOARD_PASSWORD_PATH": true,
	}
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if strings.HasPrefix(parts[0], "TINYROUTE_") && !known[parts[0]] {
			log.Printf("WARNING: unknown setting %s (ignored)", parts[0])
		}
	}

	return s, nil
}

func getEnvOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func expandHome(path, home string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}
