package credential

// BodyFormat specifies the HTTP request body format for token refresh.
type BodyFormat string

const (
	// FormatJSON formats refresh request body as application/json.
	FormatJSON BodyFormat = "json"
	// FormatForm formats refresh request body as application/x-www-form-urlencoded.
	FormatForm BodyFormat = "form"
)

// RefreshProfile defines per-provider OAuth refresh request formatting.
type RefreshProfile struct {
	BodyFormat          BodyFormat `json:"body_format,omitempty" yaml:"body_format,omitempty"`
	UseBasicAuth        bool       `json:"use_basic_auth,omitempty" yaml:"use_basic_auth,omitempty"`
	IncludeClientSecret bool       `json:"include_client_secret,omitempty" yaml:"include_client_secret,omitempty"`
	GrantType           string     `json:"grant_type,omitempty" yaml:"grant_type,omitempty"`
}
