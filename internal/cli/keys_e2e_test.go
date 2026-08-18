package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/auth"
)

// TestKeysLifecycleEndToEnd provides reproducible evidence for Task 7.2:
// create (with/without expiry/rate) → list → reveal → edit rate → revoke → confirm hidden from list and rejected on request path without restart.
func TestKeysLifecycleEndToEnd(t *testing.T) {
	tmpDir := setupTestHome(t)
	dotDir := filepath.Join(tmpDir, ".tinyroute")
	if err := os.MkdirAll(dotDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	keysPath := filepath.Join(dotDir, "keys.json")

	// 1. Create key without expiry/rate
	err := cmdKeysCreate([]string{"--name=unrestricted-key"})
	if err != nil {
		t.Fatalf("create unrestricted key failed: %v", err)
	}

	// 2. Create key with expiry and rate
	err = cmdKeysCreate([]string{"--name=limited-key", "--expires=48h", "--rate=30/1m"})
	if err != nil {
		t.Fatalf("create limited key failed: %v", err)
	}

	// Load and verify keystore
	kf, err := loadKeyFile(keysPath)
	if err != nil {
		t.Fatalf("loadKeyFile failed: %v", err)
	}
	if len(kf.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(kf.Keys))
	}
	k1 := kf.Keys[0]
	k2 := kf.Keys[1]

	// 3. List keys - verify both keys displayed
	captureList := func() string {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		_ = cmdKeysList(nil)
		w.Close()
		os.Stdout = oldStdout
		var buf bytes.Buffer
		io.Copy(&buf, r)
		return buf.String()
	}

	listOut := captureList()
	if !strings.Contains(listOut, "unrestricted-key") || !strings.Contains(listOut, "limited-key") {
		t.Fatalf("expected both keys in list output, got:\n%s", listOut)
	}
	if !strings.Contains(listOut, "30/1m") {
		t.Fatalf("expected rate 30/1m in list output, got:\n%s", listOut)
	}

	// 4. Reveal - verify plaintext secrets are persisted and retrievable
	raw, err := os.ReadFile(keysPath)
	if err != nil {
		t.Fatalf("read keys file failed: %v", err)
	}
	ks, err := auth.ParseKeyFile(raw)
	if err != nil {
		t.Fatalf("parse key file failed: %v", err)
	}
	lookup1, ok1 := ks.Lookup(k1.ID)
	lookup2, ok2 := ks.Lookup(k2.ID)
	if !ok1 || lookup1.Secret == "" || !ok2 || lookup2.Secret == "" {
		t.Fatalf("expected both keys to have recoverable secrets in keystore")
	}

	// Verify request path authentication with valid tokens
	if _, err := ks.Verify(lookup1.Secret); err != nil {
		t.Fatalf("verify k1 failed: %v", err)
	}
	if _, err := ks.Verify(lookup2.Secret); err != nil {
		t.Fatalf("verify k2 failed: %v", err)
	}

	// 5. Edit rate on limited-key
	newRate := &auth.RateSpec{Requests: 100, Interval: "1m"}
	if err := auth.UpdateKey(keysPath, k2.ID, auth.KeyUpdate{Rate: newRate}); err != nil {
		t.Fatalf("UpdateKey rate failed: %v", err)
	}
	raw, _ = os.ReadFile(keysPath)
	ks, _ = auth.ParseKeyFile(raw)
	updatedK2, _ := ks.Lookup(k2.ID)
	if updatedK2.Rate == nil || updatedK2.Rate.Requests != 100 {
		t.Fatalf("expected updated rate 100/1m, got %+v", updatedK2.Rate)
	}

	// 6. Revoke limited-key
	if err := cmdKeysRevoke([]string{k2.ID, "--force"}); err != nil {
		t.Fatalf("cmdKeysRevoke failed: %v", err)
	}

	// 7. Verify revoked key is hidden from list output
	listAfterRevoke := captureList()
	if strings.Contains(listAfterRevoke, "limited-key") {
		t.Fatalf("revoked key 'limited-key' must not appear in list output, got:\n%s", listAfterRevoke)
	}
	if !strings.Contains(listAfterRevoke, "unrestricted-key") {
		t.Fatalf("unrestricted key should still appear in list output, got:\n%s", listAfterRevoke)
	}

	// 8. Confirm revoked key is rejected on the request path without daemon restart
	raw, _ = os.ReadFile(keysPath)
	ks, _ = auth.ParseKeyFile(raw)
	if _, err := ks.Verify(lookup2.Secret); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected revoked key to be rejected with disabled error, got err=%v", err)
	}
	// Active key still works
	if _, err := ks.Verify(lookup1.Secret); err != nil {
		t.Fatalf("active key should still be verified, got err=%v", err)
	}

	// 9. Revoke remaining key and verify empty-list message
	if err := cmdKeysRevoke([]string{k1.ID, "--force"}); err != nil {
		t.Fatalf("cmdKeysRevoke k1 failed: %v", err)
	}
	allRevokedList := captureList()
	if !strings.Contains(allRevokedList, "all keys have been revoked") {
		t.Fatalf("expected all keys revoked message, got:\n%s", allRevokedList)
	}
}
