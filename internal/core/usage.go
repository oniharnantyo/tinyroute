package core

import (
	"time"
)

// QuotaConfig defines usage limits over a rolling time window for an account or provider.
type QuotaConfig struct {
	Window   time.Duration `json:"window,omitempty" yaml:"window,omitempty"`
	Tokens   int64         `json:"tokens,omitempty" yaml:"tokens,omitempty"`
	Requests int           `json:"requests,omitempty" yaml:"requests,omitempty"`
}

// UsageSnapshot summarizes usage vs limits for an account or provider.
type UsageSnapshot struct {
	Window        time.Duration `json:"window"`
	UsedTokens    int64         `json:"used_tokens"`
	UsedRequests  int           `json:"used_requests"`
	LimitTokens   int64         `json:"limit_tokens"`
	LimitRequests int           `json:"limit_requests"`
	ResetAt       time.Time     `json:"reset_at"`
}
