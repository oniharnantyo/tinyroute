package dashboard

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/clients"
)

type ClientEndpointOption struct {
	URL       string
	IsDefault bool
	IsCurrent bool
}

// ExistingKeyOption is a selectable entry for a key already stored in the
// gateway. Only a masked prefix is shown in the label; the plaintext secret
// is resolved server-side by key ID when the form is submitted.
type ExistingKeyOption struct {
	ID    string
	Label string
}

// ModelOption is one pickable model in the model picker dialog.
type ModelOption struct {
	Value string // full "provider:model" id sent to the gateway
	Label string // model name without the provider prefix, for display
}

// ProviderModelGroup groups whitelisted models under their provider for the
// model picker dialog.
type ProviderModelGroup struct {
	Provider string
	Models   []ModelOption
}

// groupModelsByProvider splits sorted "provider:model" ids into per-provider
// groups. Ids without a provider prefix are grouped under "defaults".
func groupModelsByProvider(models []string) []ProviderModelGroup {
	var groups []ProviderModelGroup
	for _, id := range models {
		provider, model, ok := strings.Cut(id, ":")
		if !ok {
			provider, model = "defaults", id
		}
		if len(groups) > 0 && groups[len(groups)-1].Provider == provider {
			groups[len(groups)-1].Models = append(groups[len(groups)-1].Models, ModelOption{Value: id, Label: model})
			continue
		}
		groups = append(groups, ProviderModelGroup{
			Provider: provider,
			Models:   []ModelOption{{Value: id, Label: model}},
		})
	}
	return groups
}

// contextWindowSuffix is appended to a slot's model id to request the 1M-token
// extended context window. It is a client-side hint: the router strips it
// before routing upstream.
const contextWindowSuffix = "[1m]"

// SlotValue returns the configured value for a slot, falling back across
// "model" / "models" synonyms if one is present.
func SlotValue(slotValues map[string]string, slot clients.ModelSlot) string {
	if slotValues == nil {
		return ""
	}
	if v := slotValues[slot.ID]; v != "" {
		return v
	}
	if slot.ID == "models" && slotValues["model"] != "" {
		return slotValues["model"]
	}
	if slot.ID == "model" && slotValues["models"] != "" {
		return slotValues["models"]
	}
	return ""
}

// slotInitJSON renders the current slot values (with any context-window suffix
// removed) as JSON for the Alpine state in viewClientDetail. The suffix flag
// lives in the separate oneM map so the picker dialog compares base ids.
func slotInitJSON(d ClientDetailPageData) string {
	m := make(map[string]string, len(d.ModelSlots))
	for _, slot := range d.ModelSlots {
		m[slot.ID] = strings.TrimSuffix(SlotValue(d.SlotValues, slot), contextWindowSuffix)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// oneMInitJSON renders per-slot booleans recording whether the stored slot
// value carried the context-window suffix, so the 1M checkbox starts checked
// for models selected with it.
func oneMInitJSON(d ClientDetailPageData) string {
	m := make(map[string]bool, len(d.ModelSlots))
	for _, slot := range d.ModelSlots {
		m[slot.ID] = strings.HasSuffix(SlotValue(d.SlotValues, slot), contextWindowSuffix)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// groupSearchStrings returns a comma-separated list of quoted strings representing
// the provider name and all model names/values in the group for Alpine's matchesQuery helper.
func groupSearchStrings(group ProviderModelGroup) string {
	parts := []string{fmt.Sprintf("%q", group.Provider)}
	for _, m := range group.Models {
		parts = append(parts, fmt.Sprintf("%q", m.Label), fmt.Sprintf("%q", m.Value))
	}
	return strings.Join(parts, ", ")
}

// slotPlaceholder returns the text shown when a slot has no model selected.
func slotPlaceholder(slot clients.ModelSlot) string {
	if slot.Required {
		return "Select a model…"
	}
	return "(None / Default)"
}

type ClientDetailPageData struct {
	Client           clients.Client
	ClientID         string
	ClientName       string
	Dialect          string
	Status           clients.Status
	StatusBadgeClass string
	StatusLabel      string
	CurrentEndpoint  string
	DefaultEndpoint  string
	Endpoints        []ClientEndpointOption
	ExistingKeys     []ExistingKeyOption
	SelectedKeyID    string
	// LegacyKeyCount is the number of stored keys minted before secrets were
	// persisted; they cannot be embedded and are excluded from ExistingKeys.
	LegacyKeyCount int
	MaskedKey      string
	RoutableModels []string
	// ProviderModelGroups is RoutableModels split into per-provider groups for
	// the model picker dialog.
	ProviderModelGroups []ProviderModelGroup
	SlotValues          map[string]string
	ModelSlots          []clients.ModelSlot
	ManualSnippet       string
}

type KeyRevealPageData struct {
	ClientName string
	ClientID   string
	MintedKey  string
	BaseURL    string
	ConfigPath string
}
