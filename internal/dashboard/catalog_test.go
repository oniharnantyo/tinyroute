package dashboard

import (
	"reflect"
	"testing"
)

func TestSplitCatalogModels_Empty(t *testing.T) {
	whitelisted, available := splitCatalogModels(nil)
	if len(whitelisted) != 0 || len(available) != 0 {
		t.Errorf("nil input: expected empty partitions, got whitelisted=%v available=%v", whitelisted, available)
	}

	whitelisted, available = splitCatalogModels([]CatalogModelItem{})
	if len(whitelisted) != 0 || len(available) != 0 {
		t.Errorf("empty input: expected empty partitions, got whitelisted=%v available=%v", whitelisted, available)
	}
}

func TestSplitCatalogModels_AllWhitelisted(t *testing.T) {
	in := []CatalogModelItem{{ID: "a", Whitelisted: true}, {ID: "b", Whitelisted: true}}
	whitelisted, available := splitCatalogModels(in)
	if !reflect.DeepEqual(whitelisted, in) {
		t.Errorf("whitelisted=%v, want %v", whitelisted, in)
	}
	if len(available) != 0 {
		t.Errorf("available=%v, want empty", available)
	}
}

func TestSplitCatalogModels_AllAvailable(t *testing.T) {
	in := []CatalogModelItem{{ID: "a"}, {ID: "b"}}
	whitelisted, available := splitCatalogModels(in)
	if len(whitelisted) != 0 {
		t.Errorf("whitelisted=%v, want empty", whitelisted)
	}
	if !reflect.DeepEqual(available, in) {
		t.Errorf("available=%v, want %v", available, in)
	}
}

func TestSplitCatalogModels_MixedPreservesOrder(t *testing.T) {
	in := []CatalogModelItem{
		{ID: "wl-a", Whitelisted: true},
		{ID: "cat-1"},
		{ID: "wl-b", Whitelisted: true},
		{ID: "cat-2"},
		{ID: "cat-3"},
		{ID: "wl-c", Whitelisted: true},
	}
	whitelisted, available := splitCatalogModels(in)

	wantWhitelisted := []CatalogModelItem{{ID: "wl-a", Whitelisted: true}, {ID: "wl-b", Whitelisted: true}, {ID: "wl-c", Whitelisted: true}}
	wantAvailable := []CatalogModelItem{{ID: "cat-1"}, {ID: "cat-2"}, {ID: "cat-3"}}

	if !reflect.DeepEqual(whitelisted, wantWhitelisted) {
		t.Errorf("whitelisted=%v, want %v", whitelisted, wantWhitelisted)
	}
	if !reflect.DeepEqual(available, wantAvailable) {
		t.Errorf("available=%v, want %v", available, wantAvailable)
	}

	// The union of both partitions must reconstruct the full catalog.
	if len(whitelisted)+len(available) != len(in) {
		t.Errorf("partition sizes %d+%d != input %d", len(whitelisted), len(available), len(in))
	}
}

func TestSplitCatalogModels_DoesNotAliasInput(t *testing.T) {
	in := []CatalogModelItem{{ID: "a", Whitelisted: true}}
	whitelisted, _ := splitCatalogModels(in)
	whitelisted[0].ID = "mutated"

	// The original slice must be unaffected by mutations to the result.
	if in[0].ID != "a" {
		t.Errorf("splitCatalogModels aliased the input: in[0].ID=%q, want %q", in[0].ID, "a")
	}
}
