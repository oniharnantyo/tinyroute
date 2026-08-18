package auth

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	specs := map[string]*RateSpec{
		"unlimited": nil,
		"invalid":   {Requests: 0, Interval: "1m"},
		"bad-intv":  {Requests: 10, Interval: "xyz"},
		"limited":   {Requests: 2, Interval: "1s"},
	}

	rl := NewRateLimiter(func(keyID string) *RateSpec {
		return specs[keyID]
	})

	// Unlimited key
	if ok, d := rl.Allow("unlimited"); !ok || d != 0 {
		t.Errorf("expected unlimited key to be allowed, got ok=%v, d=%v", ok, d)
	}

	// Invalid spec keys
	if ok, _ := rl.Allow("invalid"); !ok {
		t.Errorf("expected invalid spec to allow")
	}
	if ok, _ := rl.Allow("bad-intv"); !ok {
		t.Errorf("expected bad interval to allow")
	}

	// Limited key
	if ok, _ := rl.Allow("limited"); !ok {
		t.Errorf("first request should be allowed")
	}
	if ok, _ := rl.Allow("limited"); !ok {
		t.Errorf("second request should be allowed")
	}
	if ok, retry := rl.Allow("limited"); ok || retry < time.Millisecond {
		t.Errorf("third request should be rate limited, got ok=%v, retry=%v", ok, retry)
	}
}
