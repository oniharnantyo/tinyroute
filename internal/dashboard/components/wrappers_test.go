package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/dashboard/components/icon"
)

func TestStatusBadgeRendering(t *testing.T) {
	tests := []struct {
		variant StatusBadgeVariant
		want    string
	}{
		{StatusSuccess, "border-emerald-500/30"},
		{StatusWarning, "border-amber-500/30"},
		{StatusError, "border-rose-500/30"},
		{StatusInfo, "border-sky-500/30"},
		{StatusNeutral, "border-border"},
	}

	for _, tt := range tests {
		buf := new(bytes.Buffer)
		err := StatusBadge(StatusBadgeProps{Variant: tt.variant}).Render(context.Background(), buf)
		if err != nil {
			t.Fatalf("failed to render StatusBadge(%s): %v", tt.variant, err)
		}
		if !strings.Contains(buf.String(), tt.want) {
			t.Errorf("StatusBadge(%s) missing expected class %q, got: %s", tt.variant, tt.want, buf.String())
		}
	}
}

func TestKPICardRendering(t *testing.T) {
	buf := new(bytes.Buffer)
	err := KPICard(KPICardProps{
		Title:       "Active Routes",
		Value:       "42",
		Description: "+3 today",
		Icon:        icon.Route,
	}).Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render KPICard: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Active Routes") || !strings.Contains(out, "42") || !strings.Contains(out, "+3 today") {
		t.Errorf("KPICard missing content, got: %s", out)
	}
}

func TestAlertBannerRendering(t *testing.T) {
	tests := []struct {
		variant AlertBannerVariant
		title   string
		desc    string
		want    string
	}{
		{AlertSuccess, "Success", "Saved successfully", "border-emerald-500/30"},
		{AlertWarning, "Warning", "Needs attention", "border-amber-500/30"},
		{AlertError, "Error", "Failed to connect", "border-rose-500/30"},
		{AlertInfo, "Info", "System update", "border-sky-500/30"},
	}

	for _, tt := range tests {
		buf := new(bytes.Buffer)
		err := AlertBanner(AlertBannerProps{
			Variant:     tt.variant,
			Title:       tt.title,
			Description: tt.desc,
		}).Render(context.Background(), buf)
		if err != nil {
			t.Fatalf("failed to render AlertBanner(%s): %v", tt.variant, err)
		}
		out := buf.String()
		if !strings.Contains(out, tt.want) || !strings.Contains(out, tt.title) || !strings.Contains(out, tt.desc) {
			t.Errorf("AlertBanner(%s) missing expected content or class, got: %s", tt.variant, out)
		}
	}
}
