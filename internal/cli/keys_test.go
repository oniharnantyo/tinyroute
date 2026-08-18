package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCmdKeysCreateWithExpiryAndRate(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create key with --expires duration and --rate
	err := cmdKeysCreate([]string{"--name=test-bot", "--expires=48h", "--rate=60/1m"})
	if err != nil {
		t.Fatalf("cmdKeysCreate error: %v", err)
	}

	keysPath := filepath.Join(dotDir, "keys.json")
	kf, err := loadKeyFile(keysPath)
	if err != nil {
		t.Fatalf("loadKeyFile error: %v", err)
	}
	if len(kf.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(kf.Keys))
	}
	k := kf.Keys[0]
	if k.Name != "test-bot" {
		t.Errorf("Name = %q, want test-bot", k.Name)
	}
	if k.Expires == nil || k.Expires.Before(time.Now().Add(47*time.Hour)) {
		t.Errorf("Expires = %v, expected ~48h in future", k.Expires)
	}
	if k.Rate == nil || k.Rate.Requests != 60 || k.Rate.Interval != "1m" {
		t.Errorf("Rate = %+v, want 60/1m", k.Rate)
	}

	// Create another key with RFC3339 timestamp
	futureRFC := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	err = cmdKeysCreate([]string{"--name=rfc-bot", "--expires=" + futureRFC})
	if err != nil {
		t.Fatalf("cmdKeysCreate rfc error: %v", err)
	}
	kf, _ = loadKeyFile(keysPath)
	if len(kf.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(kf.Keys))
	}
	if kf.Keys[1].Expires == nil {
		t.Errorf("expected RFC3339 expiry to be parsed")
	}

	// Invalid rate
	if err := cmdKeysCreate([]string{"--name=bad-rate", "--rate=invalid"}); err == nil {
		t.Error("expected error for invalid --rate")
	}

	// Invalid expires
	if err := cmdKeysCreate([]string{"--name=bad-exp", "--expires=not-a-time"}); err == nil {
		t.Error("expected error for invalid --expires")
	}
}

func TestCmdKeysListHidesRevoked(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create two keys
	err := cmdKeysCreate([]string{"--name=key-active", "--rate=100/1m"})
	if err != nil {
		t.Fatalf("create active key error: %v", err)
	}
	err = cmdKeysCreate([]string{"--name=key-revoked"})
	if err != nil {
		t.Fatalf("create second key error: %v", err)
	}

	keysPath := filepath.Join(dotDir, "keys.json")
	kf, err := loadKeyFile(keysPath)
	if err != nil {
		t.Fatalf("loadKeyFile error: %v", err)
	}
	if len(kf.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(kf.Keys))
	}

	// Revoke the second key
	err = cmdKeysRevoke([]string{kf.Keys[1].ID, "--force"})
	if err != nil {
		t.Fatalf("cmdKeysRevoke error: %v", err)
	}

	// Capture stdout of cmdKeysList
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = cmdKeysList(nil)
	w.Close()
	os.Stdout = oldStdout
	if err != nil {
		t.Fatalf("cmdKeysList error: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "key-active") {
		t.Errorf("expected output to contain key-active, got:\n%s", out)
	}
	if strings.Contains(out, "key-revoked") {
		t.Errorf("expected revoked key to be hidden from list output, got:\n%s", out)
	}
	if !strings.Contains(out, "100/1m") {
		t.Errorf("expected output to show rate 100/1m, got:\n%s", out)
	}
}
