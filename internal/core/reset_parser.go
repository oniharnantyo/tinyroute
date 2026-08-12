package core

import (
	"net/http"
	"time"
)

// ResetParser parses upstream rate limit headers and error response bodies
// to extract exact cooldown/reset durations.
type ResetParser interface {
	Duration(resp *http.Response, body []byte, fc *FailureClass) time.Duration
}
