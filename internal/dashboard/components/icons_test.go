package components

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestIconRendering(t *testing.T) {
	buf := new(bytes.Buffer)
	err := Icon("cpu", "w-6 h-6").Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render icon: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "w-6 h-6") || !strings.Contains(out, "<svg") {
		t.Errorf("unexpected icon rendering output: %s", out)
	}

	// Test fallback icon
	buf.Reset()
	err = Icon("unknown-icon-name", "").Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render fallback icon: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, "w-5 h-5") {
		t.Errorf("expected default class w-5 h-5, got %s", out)
	}
}
