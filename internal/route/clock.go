package route

import "time"

// RealClock implements core.Clock using the system clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// FakeClock implements core.Clock for testing.
type FakeClock struct {
	T time.Time
}

func (c *FakeClock) Now() time.Time { return c.T }

// Advance moves the fake clock forward by d.
func (c *FakeClock) Advance(d time.Duration) { c.T = c.T.Add(d) }
