package config

import (
	"os"
	"testing"
)

func TestLoadService_LogFormatAndLogLevel_Defaults(t *testing.T) {
	if val, ok := os.LookupEnv("TINYROUTE_LOG_FORMAT"); ok {
		os.Unsetenv("TINYROUTE_LOG_FORMAT")
		t.Cleanup(func() { os.Setenv("TINYROUTE_LOG_FORMAT", val) })
	}
	if val, ok := os.LookupEnv("TINYROUTE_LOG_LEVEL"); ok {
		os.Unsetenv("TINYROUTE_LOG_LEVEL")
		t.Cleanup(func() { os.Setenv("TINYROUTE_LOG_LEVEL", val) })
	}

	svc, err := LoadService()
	if err != nil {
		t.Fatalf("LoadService() unexpected error: %v", err)
	}
	if svc.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want %q", svc.LogFormat, "text")
	}
	if svc.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", svc.LogLevel, "info")
	}
}

func TestLoadService_LogFormat_AcceptedValues(t *testing.T) {
	tests := []struct {
		envValue string
		want     string
	}{
		{"text", "text"},
		{"json", "json"},
		{"TEXT", "text"},
		{"JSON", "json"},
		{"Json", "json"},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			t.Setenv("TINYROUTE_LOG_FORMAT", tt.envValue)
			svc, err := LoadService()
			if err != nil {
				t.Fatalf("LoadService() unexpected error for TINYROUTE_LOG_FORMAT=%q: %v", tt.envValue, err)
			}
			if svc.LogFormat != tt.want {
				t.Errorf("svc.LogFormat = %q, want %q", svc.LogFormat, tt.want)
			}
		})
	}
}

func TestLoadService_LogLevel_AcceptedValues(t *testing.T) {
	tests := []struct {
		envValue string
		want     string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
		{"DEBUG", "debug"},
		{"INFO", "info"},
		{"WARN", "warn"},
		{"ERROR", "error"},
		{"Warn", "warn"},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			t.Setenv("TINYROUTE_LOG_LEVEL", tt.envValue)
			svc, err := LoadService()
			if err != nil {
				t.Fatalf("LoadService() unexpected error for TINYROUTE_LOG_LEVEL=%q: %v", tt.envValue, err)
			}
			if svc.LogLevel != tt.want {
				t.Errorf("svc.LogLevel = %q, want %q", svc.LogLevel, tt.want)
			}
		})
	}
}

func TestLoadService_LogFormat_InvalidValues(t *testing.T) {
	invalidValues := []string{"invalid", "yaml", "xml", "log", "123"}
	for _, val := range invalidValues {
		t.Run(val, func(t *testing.T) {
			t.Setenv("TINYROUTE_LOG_FORMAT", val)
			_, err := LoadService()
			if err == nil {
				t.Fatalf("LoadService() expected error for TINYROUTE_LOG_FORMAT=%q, got nil", val)
			}
			expectedErrMsg := `TINYROUTE_LOG_FORMAT: "` + val + `" must be "text" or "json"`
			if err.Error() != expectedErrMsg {
				t.Errorf("error msg = %q, want %q", err.Error(), expectedErrMsg)
			}
		})
	}
}

func TestLoadService_LogLevel_InvalidValues(t *testing.T) {
	invalidValues := []string{"invalid", "trace", "fatal", "verbose", "123"}
	for _, val := range invalidValues {
		t.Run(val, func(t *testing.T) {
			t.Setenv("TINYROUTE_LOG_LEVEL", val)
			_, err := LoadService()
			if err == nil {
				t.Fatalf("LoadService() expected error for TINYROUTE_LOG_LEVEL=%q, got nil", val)
			}
			expectedErrMsg := `TINYROUTE_LOG_LEVEL: "` + val + `" must be "debug", "info", "warn", or "error"`
			if err.Error() != expectedErrMsg {
				t.Errorf("error msg = %q, want %q", err.Error(), expectedErrMsg)
			}
		})
	}
}

func TestLoadService_HistoryDB(t *testing.T) {
	t.Setenv("TINYROUTE_HISTORY_DB", "/custom/path/history.db")
	svc, err := LoadService()
	if err != nil {
		t.Fatalf("LoadService() error: %v", err)
	}
	if svc.HistoryDBPath != "/custom/path/history.db" {
		t.Errorf("HistoryDBPath = %q, want /custom/path/history.db", svc.HistoryDBPath)
	}
}
