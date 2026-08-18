package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateKeyWithOptions(t *testing.T) {
	// Default generation
	plaintext, key, err := GenerateKey("test-key")
	if err != nil {
		t.Fatalf("GenerateKey error: %v", err)
	}
	if !strings.HasPrefix(plaintext, "tr_live_") {
		t.Errorf("plaintext %q missing prefix", plaintext)
	}
	if key.Name != "test-key" {
		t.Errorf("got name %q, want test-key", key.Name)
	}
	if key.Expires != nil {
		t.Errorf("expected nil Expires, got %v", key.Expires)
	}
	if key.Rate != nil {
		t.Errorf("expected nil Rate, got %v", key.Rate)
	}
	if key.Secret != plaintext {
		t.Errorf("key.Secret = %q, want %q", key.Secret, plaintext)
	}

	// With valid options
	exp := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	rate := &RateSpec{Requests: 100, Interval: "1m"}
	_, optKey, err := GenerateKey("opt-key", WithExpires(&exp), WithRate(rate))
	if err != nil {
		t.Fatalf("GenerateKey with options error: %v", err)
	}
	if optKey.Expires == nil || !optKey.Expires.Equal(exp) {
		t.Errorf("Expires = %v, want %v", optKey.Expires, exp)
	}
	if optKey.Rate == nil || optKey.Rate.Requests != 100 || optKey.Rate.Interval != "1m" {
		t.Errorf("Rate = %+v, want %+v", optKey.Rate, rate)
	}

	// With nil options
	_, nilOptKey, err := GenerateKey("nil-opt-key", WithExpires(nil), WithRate(nil))
	if err != nil || nilOptKey.Expires != nil || nilOptKey.Rate != nil {
		t.Errorf("expected clean key with nil options, got err=%v", err)
	}

	// Invalid expiry (past)
	past := time.Now().Add(-1 * time.Hour)
	_, _, err = GenerateKey("past-key", WithExpires(&past))
	if err == nil {
		t.Errorf("expected error for past expiry, got nil")
	}

	// Invalid rate requests
	_, _, err = GenerateKey("invalid-rate", WithRate(&RateSpec{Requests: 0, Interval: "1m"}))
	if err == nil {
		t.Errorf("expected error for non-positive requests, got nil")
	}

	// Invalid rate interval
	_, _, err = GenerateKey("invalid-interval", WithRate(&RateSpec{Requests: 10, Interval: "not-a-duration"}))
	if err == nil {
		t.Errorf("expected error for invalid interval, got nil")
	}
}

func TestVerify(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	past := time.Now().Add(-1 * time.Hour)

	tok1, k1, _ := GenerateKey("valid")
	tokExp, kExp, _ := GenerateKey("expired", WithExpires(&future))
	// Artificially age kExp to test expired verification
	kExp.Expires = &past
	tokDis, kDis, _ := GenerateKey("disabled")
	kDis.Disabled = true

	kf := KeyFile{Keys: []Key{k1, kExp, kDis}}
	ks := KeyStore{
		keys: kf.Keys,
		byDigest: map[string]*Key{
			k1.Digest:   &kf.Keys[0],
			kExp.Digest: &kf.Keys[1],
			kDis.Digest: &kf.Keys[2],
		},
	}

	// Nil keystore
	var nilStore *KeyStore
	if _, err := nilStore.Verify(tok1); err == nil {
		t.Error("expected error for nil keystore")
	}

	// Empty token
	if _, err := ks.Verify(""); err == nil {
		t.Error("expected error for empty token")
	}

	// Unknown token
	if _, err := ks.Verify("tr_live_unknown"); err == nil {
		t.Error("expected error for unknown token")
	}

	// Valid token
	if id, err := ks.Verify(tok1); err != nil || id != k1.ID {
		t.Errorf("Verify valid token failed: id=%q, err=%v", id, err)
	}

	// Expired token
	if _, err := ks.Verify(tokExp); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected expired error, got %v", err)
	}

	// Disabled token
	if _, err := ks.Verify(tokDis); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("expected disabled error, got %v", err)
	}
}

func TestUpdateKeyAndRevokeKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")

	// Create initial file with stale "allow" field to verify dropping on rewrite
	tok1, k1, _ := GenerateKey("key-1")
	tok2, k2, _ := GenerateKey("key-2")

	initialJSON := `{
  "keys": [
    {
      "id": "` + k1.ID + `",
      "name": "` + k1.Name + `",
      "prefix": "` + k1.Prefix + `",
      "digest": "` + k1.Digest + `",
      "secret": "` + tok1 + `",
      "created": "2026-01-01T00:00:00Z",
      "allow": ["openai:*"]
    },
    {
      "id": "` + k2.ID + `",
      "name": "` + k2.Name + `",
      "prefix": "` + k2.Prefix + `",
      "digest": "` + k2.Digest + `",
      "secret": "` + tok2 + `",
      "created": "2026-01-01T00:00:00Z"
    }
  ]
}
`
	if err := os.WriteFile(path, []byte(initialJSON), 0o600); err != nil {
		t.Fatalf("failed to write initial keys.json: %v", err)
	}

	// 1. Update Key 1
	newName := "updated-key-1"
	future := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	newRate := &RateSpec{Requests: 50, Interval: "10s"}
	err := UpdateKey(path, k1.ID, KeyUpdate{
		Name:    &newName,
		Expires: &future,
		Rate:    newRate,
	})
	if err != nil {
		t.Fatalf("UpdateKey error: %v", err)
	}

	// Verify persistence and that secret & digest are untouched, and "allow" dropped
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read back file: %v", err)
	}
	if strings.Contains(string(raw), "allow") {
		t.Errorf("expected stale allow to be dropped after rewrite, got:\n%s", string(raw))
	}

	ks, err := ParseKeyFile(raw)
	if err != nil {
		t.Fatalf("ParseKeyFile error: %v", err)
	}
	u1, ok := ks.Lookup(k1.ID)
	if !ok {
		t.Fatalf("Lookup %s failed", k1.ID)
	}
	if u1.Name != "updated-key-1" || u1.Secret != tok1 || u1.Digest != k1.Digest {
		t.Errorf("unexpected updated key state: %+v", u1)
	}
	if u1.Expires == nil || !u1.Expires.Equal(future) {
		t.Errorf("Expires = %v, want %v", u1.Expires, future)
	}
	if u1.Rate == nil || u1.Rate.Requests != 50 || u1.Rate.Interval != "10s" {
		t.Errorf("Rate = %+v, want %+v", u1.Rate, newRate)
	}

	// Clear expires & rate
	err = UpdateKey(path, k1.ID, KeyUpdate{
		ClearExpires: true,
		ClearRate:    true,
	})
	if err != nil {
		t.Fatalf("UpdateKey clear error: %v", err)
	}
	raw, _ = os.ReadFile(path)
	ks, _ = ParseKeyFile(raw)
	u1, _ = ks.Lookup(k1.ID)
	if u1.Expires != nil {
		t.Errorf("expected nil Expires after ClearExpires, got %v", u1.Expires)
	}
	if u1.Rate != nil {
		t.Errorf("expected nil Rate after ClearRate, got %v", u1.Rate)
	}

	// Update with empty name should error
	emptyName := "   "
	if err := UpdateKey(path, k1.ID, KeyUpdate{Name: &emptyName}); err == nil {
		t.Error("expected error on empty name")
	}

	// Update with past expiry should error
	pastExp := time.Now().Add(-1 * time.Hour)
	if err := UpdateKey(path, k1.ID, KeyUpdate{Expires: &pastExp}); err == nil {
		t.Error("expected error on past expiry update")
	}

	// Update with invalid rate spec should error
	if err := UpdateKey(path, k1.ID, KeyUpdate{Rate: &RateSpec{Requests: 0, Interval: "1m"}}); err == nil {
		t.Error("expected error on zero requests rate update")
	}

	// Update unknown id
	if err := UpdateKey(path, "k_nonexistent", KeyUpdate{Name: &newName}); err == nil {
		t.Error("expected error on unknown id")
	}

	// Update non-existent file
	if err := UpdateKey(filepath.Join(dir, "no-file.json"), k1.ID, KeyUpdate{Name: &newName}); err == nil {
		t.Error("expected error on non-existent file")
	}

	// 2. Revoke Key 2
	if err := RevokeKey(path, k2.ID); err != nil {
		t.Fatalf("RevokeKey error: %v", err)
	}
	raw, _ = os.ReadFile(path)
	ks, _ = ParseKeyFile(raw)
	u2, ok := ks.Lookup(k2.ID)
	if !ok || !u2.Disabled {
		t.Fatalf("key2 disabled = %v, want true", u2.Disabled)
	}

	// Revoking already disabled key should error
	if err := RevokeKey(path, k2.ID); err == nil {
		t.Error("expected error when revoking already disabled key")
	}

	// Revoke unknown key
	if err := RevokeKey(path, "k_nonexistent"); err == nil {
		t.Error("expected error when revoking unknown key")
	}

	// Revoke non-existent file
	if err := RevokeKey(filepath.Join(dir, "no-file.json"), k2.ID); err == nil {
		t.Error("expected error when file does not exist")
	}

	// Corrupt file tests
	corruptPath := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corruptPath, []byte("bad-json"), 0o600); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}
	if err := UpdateKey(corruptPath, k1.ID, KeyUpdate{Name: &newName}); err == nil {
		t.Error("expected error on corrupt file in UpdateKey")
	}
	if err := RevokeKey(corruptPath, k2.ID); err == nil {
		t.Error("expected error on corrupt file in RevokeKey")
	}

	// WriteKeyFile to unwritable path
	if err := WriteKeyFile(filepath.Join(dir, "nonexistent-dir", "keys.json"), KeyFile{}); err == nil {
		t.Error("expected error writing to unwritable path")
	}
}

func TestLookupAndKeys(t *testing.T) {
	tok, k, _ := GenerateKey("token-test")
	ks, err := ParseKeyFile([]byte(`{"keys":[{"id":"` + k.ID + `","name":"` + k.Name + `","prefix":"` + k.Prefix + `","digest":"` + k.Digest + `","secret":"` + tok + `","created":"2026-01-01T00:00:00Z"}]}`))
	if err != nil {
		t.Fatalf("ParseKeyFile error: %v", err)
	}

	// Keys()
	all := ks.Keys()
	if len(all) != 1 || all[0].ID != k.ID {
		t.Errorf("Keys() = %+v, want 1 key", all)
	}

	// Nil receiver methods
	var nilStore *KeyStore
	if nilStore.Keys() != nil {
		t.Errorf("expected nil from nilStore.Keys()")
	}
	if _, ok := nilStore.Lookup(k.ID); ok {
		t.Errorf("expected false from nilStore.Lookup()")
	}
	if _, ok := nilStore.LookupByToken(tok); ok {
		t.Errorf("expected false from nilStore.LookupByToken()")
	}

	// Lookup
	if found, ok := ks.Lookup(k.ID); !ok || found.Name != k.Name {
		t.Errorf("Lookup(%q) = %+v, %v", k.ID, found, ok)
	}
	if _, ok := ks.Lookup("k_unknown"); ok {
		t.Errorf("Lookup(unknown) returned true")
	}

	// LookupByToken
	found, ok := ks.LookupByToken(tok)
	if !ok || found.ID != k.ID {
		t.Errorf("LookupByToken(%q) = %+v, %v; want ID %s", tok, found, ok, k.ID)
	}
	if _, ok := ks.LookupByToken("unknown"); ok {
		t.Errorf("LookupByToken(unknown) returned true")
	}
	if _, ok := ks.LookupByToken(""); ok {
		t.Errorf("LookupByToken('') returned true")
	}

	// ParseKeyFile bad json
	if _, err := ParseKeyFile([]byte(`bad json`)); err == nil {
		t.Error("expected error on bad json in ParseKeyFile")
	}
}
