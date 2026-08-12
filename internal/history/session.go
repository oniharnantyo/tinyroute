package history

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
)

// DeriveSessionID returns a session identifier for the request.
// Uses the explicit header if present, otherwise derives from fingerprint inputs.
func DeriveSessionID(header string, parsed core.ParsedRequest, now time.Time) string {
	if header != "" {
		return header
	}

	h := sha256.New()
	for _, input := range parsed.SessionInputs {
		h.Write(input)
		h.Write([]byte{0}) // separator
	}
	// Day bucket: YYYY-MM-DD
	h.Write([]byte(now.Format("2006-01-02")))

	digest := h.Sum(nil)
	return hex.EncodeToString(digest[:3]) // 6 hex chars
}
