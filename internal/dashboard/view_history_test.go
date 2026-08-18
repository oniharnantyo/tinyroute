package dashboard

import (
	"context"
	"strings"
	"testing"
)

func renderHistoryPage(t *testing.T, data HistoryPageData) string {
	t.Helper()
	var sb strings.Builder
	if err := HistoryPage(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render HistoryPage: %v", err)
	}
	return sb.String()
}

func TestHistoryPage_Filters_RenderProviderSelect(t *testing.T) {
	html := renderHistoryPage(t, HistoryPageData{
		FilterProvider:     "anthropic",
		AvailableProviders: []string{"anthropic", "openai"},
	})

	if !strings.Contains(html, `action="/dashboard/history"`) {
		t.Error("expected filter form posting to /dashboard/history")
	}
	if !strings.Contains(html, `name="provider"`) {
		t.Error("expected hidden input named provider for form submission")
	}
	if !strings.Contains(html, `value="anthropic"`) {
		t.Error("expected default provider value carried into the select state")
	}
	if !strings.Contains(html, `id="history-provider"`) {
		t.Error("expected select trigger id for label association")
	}
	if !strings.Contains(html, "All providers") {
		t.Error("expected clear-selection item in provider list")
	}
	for _, p := range []string{"anthropic", "openai"} {
		if !strings.Contains(html, `data-tui-select-value="`+p+`"`) {
			t.Errorf("expected select item for provider %q", p)
		}
	}
}

func TestHistoryPage_Filters_ResetHiddenWithoutActiveFilters(t *testing.T) {
	cleared := renderHistoryPage(t, HistoryPageData{AvailableProviders: []string{"anthropic"}})
	if strings.Contains(cleared, "Reset") {
		t.Error("Reset should be hidden when no filter is active")
	}

	active := renderHistoryPage(t, HistoryPageData{FilterProvider: "anthropic"})
	if !strings.Contains(active, "Reset") {
		t.Error("expected Reset control when a filter is active")
	}
}
