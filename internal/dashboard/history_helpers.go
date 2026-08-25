package dashboard

import (
	"encoding/json"
	"fmt"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/dashboard/components"
)

// decodeAttempts decodes a JSON string of attempts into []core.Attempt.
// It returns an empty slice if the input is empty or malformed.
func decodeAttempts(raw string) []core.Attempt {
	if raw == "" {
		return []core.Attempt{}
	}
	var atts []core.Attempt
	if err := json.Unmarshal([]byte(raw), &atts); err != nil {
		return []core.Attempt{}
	}
	if atts == nil {
		return []core.Attempt{}
	}
	return atts
}

// deriveStatusCode derives the HTTP status code for a history record.
// Order of precedence:
// 1. Status of the first 2xx attempt (the winning hop).
// 2. Status of the last attempt (all hops failed).
// 3. Fallback outcome mapping for records with no attempts or malformed attempts.
func deriveStatusCode(outcome string, attempts []core.Attempt) int {
	// 1. First 2xx attempt
	for _, att := range attempts {
		if att.Status >= 200 && att.Status < 300 {
			return att.Status
		}
	}

	// 2. Last attempt's status if attempts exist
	if len(attempts) > 0 {
		return attempts[len(attempts)-1].Status
	}

	// 3. Outcome map fallback
	switch core.Outcome(outcome) {
	case core.OutcomeNoRoute:
		return 404
	case core.OutcomeAuthFailed:
		return 401
	case core.OutcomeRateLimited:
		return 429
	case core.OutcomeBodyTooLarge:
		return 413
	case core.OutcomeChainExhausted, core.OutcomeMidStream:
		return 502
	case core.OutcomeOK:
		return 200
	}

	if outcome == "success" {
		return 200
	}

	return 500
}

// historyStatusBadgeVariant returns the StatusBadgeVariant for a given HTTP status code:
// 2xx -> StatusSuccess
// 4xx -> StatusWarning
// 5xx, 0, or others -> StatusError (destructive)
func historyStatusBadgeVariant(statusCode int) components.StatusBadgeVariant {
	if statusCode >= 200 && statusCode < 300 {
		return components.StatusSuccess
	}
	if statusCode >= 400 && statusCode < 500 {
		return components.StatusWarning
	}
	return components.StatusError
}

const maxBodyDisplayBytes = 512 * 1024 // 512 KB

type FormattedBody struct {
	Content     string
	ByteCount   int
	IsTruncated bool
	TotalSize   string
	IsJSON      bool
}

// formatBodyPane pretty-prints JSON if valid, truncates bodies exceeding 512 KB,
// and returns formatting metadata for display.
func formatBodyPane(raw string) FormattedBody {
	byteLen := len(raw)
	if byteLen == 0 {
		return FormattedBody{
			Content:   "",
			ByteCount: 0,
		}
	}

	fb := FormattedBody{
		ByteCount: byteLen,
	}

	// Try JSON pretty-printing first on raw body
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
			fb.IsJSON = true
			if len(pretty) > maxBodyDisplayBytes {
				fb.IsTruncated = true
				fb.TotalSize = formatBytes(int64(byteLen))
				fb.Content = string(pretty[:maxBodyDisplayBytes])
			} else {
				fb.Content = string(pretty)
			}
			return fb
		}
	}

	// Plain text fallback
	if byteLen > maxBodyDisplayBytes {
		fb.IsTruncated = true
		fb.TotalSize = formatBytes(int64(byteLen))
		fb.Content = raw[:maxBodyDisplayBytes]
	} else {
		fb.Content = raw
	}
	fb.IsJSON = false
	return fb
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatCompact formats large numbers into compact human-readable strings (e.g. 1.2k, 2.4M).
func formatCompact(n int64) string {
	if n < 0 {
		return "-" + formatCompact(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000.0)
	}
	if n < 1_000_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000.0)
	}
	if n < 1_000_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000.0)
	}
	return fmt.Sprintf("%.1fT", float64(n)/1_000_000_000_000.0)
}
