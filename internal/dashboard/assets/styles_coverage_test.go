package assets

import (
	"strings"
	"testing"
)

// TestStylesCSSContainsChartUtilities guards against a stale styles.css:
// the compiled stylesheet must contain the utilities the chart surfaces
// rely on. When the chart was installed, styles.css was never regenerated —
// the overview chart container lost its only height source (h-[240px]) and
// chart.js rendered nothing into the zero-height panel.
func TestStylesCSSContainsChartUtilities(t *testing.T) {
	css, err := ReadFile("styles.css")
	if err != nil {
		t.Fatalf("read styles.css: %v", err)
	}

	critical := []string{
		`.h-\[240px\]`,  // overview chart container height
		`.aspect-video`, // chart container default aspect ratio
		`.aspect-auto`,  // chart container override used by the overview
	}
	for _, class := range critical {
		if !strings.Contains(string(css), class) {
			t.Errorf("styles.css is missing %s — regenerate it (see assets.go comment): the chart renders zero-height without it", class)
		}
	}

	// The chart component's arbitrary-variant styling (axis ticks, grid,
	// tooltip cursor) must also be present, or the chart renders unstyled.
	if !strings.Contains(string(css), `recharts-cartesian-axis-tick_text`) {
		t.Errorf("styles.css is missing the chart recharts arbitrary variants — regenerate it after installing chart components")
	}
}
