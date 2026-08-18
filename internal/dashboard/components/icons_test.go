package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/dashboard/components/icon"
)

func TestTypedIconRendering(t *testing.T) {
	buf := new(bytes.Buffer)
	err := icon.Cpu(icon.Props{Class: "w-6 h-6"}).Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render icon: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "w-6 h-6") || !strings.Contains(out, "<svg") {
		t.Errorf("unexpected icon rendering output: %s", out)
	}
}

func TestUnknownIconNameReturnsError(t *testing.T) {
	buf := new(bytes.Buffer)
	err := icon.Icon("no-such-icon")(icon.Props{}).Render(context.Background(), buf)
	if err == nil {
		t.Error("expected error for unknown icon name, got nil")
	}
	if !strings.Contains(err.Error(), "no-such-icon") {
		t.Errorf("error should name the unknown icon, got: %v", err)
	}
}
