package core

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// StandardResetParser implements ResetParser for standard HTTP and provider headers/bodies.
type StandardResetParser struct {
	MaxDuration time.Duration
}

// NewStandardResetParser creates a StandardResetParser with an optional maximum cap.
func NewStandardResetParser(maxCap time.Duration) *StandardResetParser {
	if maxCap <= 0 {
		maxCap = 24 * time.Hour
	}
	return &StandardResetParser{MaxDuration: maxCap}
}

func (p *StandardResetParser) Duration(resp *http.Response, body []byte, fc *FailureClass) time.Duration {
	if resp == nil {
		return 0
	}

	// 1. Try Retry-After header
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if d := parseDurationString(ra); d > 0 {
			return p.cap(d)
		}
		if sec, err := strconv.ParseInt(ra, 10, 64); err == nil && sec > 0 {
			return p.cap(time.Duration(sec) * time.Second)
		}
		if t, err := http.ParseTime(ra); err == nil {
			if d := time.Until(t); d > 0 {
				return p.cap(d)
			}
		}
	}

	// 2. Try OpenAI reset headers: x-ratelimit-reset-requests, x-ratelimit-reset-tokens
	for _, h := range []string{"x-ratelimit-reset-requests", "x-ratelimit-reset-tokens"} {
		if val := resp.Header.Get(h); val != "" {
			if d := parseDurationString(val); d > 0 {
				return p.cap(d)
			}
		}
	}

	// 3. Try Anthropic reset headers: anthropic-ratelimit-requests-reset, anthropic-ratelimit-tokens-reset
	for _, h := range []string{"anthropic-ratelimit-requests-reset", "anthropic-ratelimit-tokens-reset"} {
		if val := resp.Header.Get(h); val != "" {
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				if d := time.Until(t); d > 0 {
					return p.cap(d)
				}
			}
			if d := parseDurationString(val); d > 0 {
				return p.cap(d)
			}
		}
	}

	// 4. Try JSON body parsing (resets_at, reset_at, Retry-After in body, Codex usage_limit_reached)
	if len(body) > 0 {
		if d := parseJSONBodyReset(body); d > 0 {
			return p.cap(d)
		}
	}

	return 0
}

func (p *StandardResetParser) cap(d time.Duration) time.Duration {
	if p.MaxDuration > 0 && d > p.MaxDuration {
		return p.MaxDuration
	}
	return d
}

func parseDurationString(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	// OpenAI reset headers can format as "6m0s", "100ms", "1s", "1m", "1h"
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "m") || strings.HasSuffix(s, "h") || strings.HasSuffix(s, "ms") {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
	}
	return 0
}

func parseJSONBodyReset(body []byte) time.Duration {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}

	// Check root or error object
	var target map[string]any = payload
	if errObj, ok := payload["error"].(map[string]any); ok {
		target = errObj
	}

	// Check resets_at or reset_at (unix timestamp integer/float or RFC3339 string)
	for _, key := range []string{"resets_at", "reset_at", "retry_after"} {
		if val, ok := target[key]; ok {
			switch v := val.(type) {
			case float64:
				if v > 0 {
					// if v looks like epoch timestamp (> 1e9)
					if v > 1e9 {
						t := time.Unix(int64(v), 0)
						if d := time.Until(t); d > 0 {
							return d
						}
					} else {
						return time.Duration(v) * time.Second
					}
				}
			case string:
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					if d := time.Until(t); d > 0 {
						return d
					}
				}
				if d := parseDurationString(v); d > 0 {
					return d
				}
			}
		}
	}

	// Check Codex / special message codes (e.g. usage_limit_reached)
	if msg, ok := target["message"].(string); ok {
		if strings.Contains(msg, "usage_limit_reached") || strings.Contains(msg, "Quota exceeded") {
			// default fallback suggestion for quota resets if no specific time given
			return 5 * time.Minute
		}
	}

	return 0
}
