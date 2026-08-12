// Package concerns holds pure mappings and shared utilities for dialect translation.
package concerns

import (
	"regexp"
	"sync"
)

var geminiNameInvalidChars = regexp.MustCompile(`[^a-zA-Z0-9_.:-]`)

// GeminiNameMap maintains sanitization round-trip mappings between OpenAI/Anthropic
// tool names and Gemini-compliant function names [a-zA-Z_][a-zA-Z0-9_.:-]{0,63}.
type GeminiNameMap struct {
	mu        sync.RWMutex
	sanToOrig map[string]string
}

// NewGeminiNameMap initializes a new GeminiNameMap.
func NewGeminiNameMap() *GeminiNameMap {
	return &GeminiNameMap{
		sanToOrig: make(map[string]string),
	}
}

// Sanitize converts a tool name into a Gemini-compliant identifier and records
// the original name for response recovery.
func (m *GeminiNameMap) Sanitize(name string) string {
	if name == "" {
		return ""
	}
	san := geminiNameInvalidChars.ReplaceAllString(name, "_")
	if len(san) > 64 {
		san = san[:64]
	}
	first := san[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		san = "_" + san
		if len(san) > 64 {
			san = san[:64]
		}
	}
	if m != nil {
		m.mu.Lock()
		m.sanToOrig[san] = name
		m.mu.Unlock()
	}
	return san
}

// Restore converts a sanitized Gemini function name back to its original name.
func (m *GeminiNameMap) Restore(san string) string {
	if m == nil {
		return san
	}
	m.mu.RLock()
	orig, ok := m.sanToOrig[san]
	m.mu.RUnlock()
	if ok {
		return orig
	}
	return san
}
