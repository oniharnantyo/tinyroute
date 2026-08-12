package oauth

import "time"

// Flow identifies the OAuth flow type.
type Flow int

const (
	// FlowAuthorizePKCE represents the authorization-code flow with PKCE (RFC 7636).
	FlowAuthorizePKCE Flow = iota

	// FlowDeviceCode represents the device code flow (RFC 8628).
	FlowDeviceCode
)

// Config is the resolved OAuth configuration for one provider instance.
type Config struct {
	Flow                Flow
	ProviderName        string
	ClientID            string
	ClientSecret        string
	AuthorizeURL        string
	TokenURL            string
	DeviceURL           string // device-code endpoint (FlowDeviceCode)
	RefreshURL          string
	RedirectURI         string
	Scopes              []string
	ExtraParams         map[string]string
	CodeChallengeMethod string // usually "S256"
	DeviceHeaderProfile string // optional, e.g. "kimi" for X-Msh-* headers
}

// AuthorizeResult holds the result of initiating a PKCE authorization flow.
type AuthorizeResult struct {
	URL          string // redirect the user's browser here
	State        string // persist server-side; verify at callback
	CodeVerifier string // persist server-side; needed by Exchange
}

// DeviceResult holds the result of requesting a device code.
type DeviceResult struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       int
	Interval        int
}

// Tokens are the resolved credentials from a completed OAuth flow.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	TokenType    string
	Scope        string
	IDToken      string
}
