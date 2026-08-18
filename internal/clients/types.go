package clients

// Status represents the configuration status of a coding agent.
type Status struct {
	Installed          bool              `json:"installed"`
	PointedAtTinyRoute bool              `json:"pointed_at_tinyroute"`
	ConfigPath         string            `json:"config_path"`
	CurrentBaseURL     string            `json:"current_base_url,omitempty"`
	MaskedKey          string            `json:"masked_key,omitempty"`
	RawKey             string            `json:"raw_key,omitempty"`
	SlotValues         map[string]string `json:"slot_values,omitempty"`
}

// ModelSlotKind defines the shape of a model selection slot (single or multi-select).
type ModelSlotKind int

const (
	SlotSingle ModelSlotKind = iota
	SlotMulti
)

// ModelSlot defines a model selection slot required or supported by an adapter.
type ModelSlot struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Kind     ModelSlotKind `json:"kind"`
	Required bool          `json:"required"`
}

// ApplyInput holds the options supplied when installing or updating an agent configuration.
type ApplyInput struct {
	BaseURL       string            `json:"base_url"`
	APIKey        string            `json:"api_key"`
	Model         string            `json:"model,omitempty"`          // primary model selection
	Models        []string          `json:"models,omitempty"`         // multi-model selection list
	ModelSlots    map[string]string `json:"model_slots,omitempty"`    // slot-id -> model-id map
	ContextWindow string            `json:"context_window,omitempty"` // max context tokens override
	Account       string            `json:"account,omitempty"`        // target provider/account
	Accounts      []string          `json:"accounts,omitempty"`       // multi-account pool for rotation
}

// Result describes the outcome of applying an agent configuration.
type Result struct {
	Files  []string `json:"files"`
	Key    string   `json:"key,omitempty"`
	Backup string   `json:"backup,omitempty"`
}
