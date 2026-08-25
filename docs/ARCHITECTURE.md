# tinyroute Architecture Documentation

## System Overview

tinyroute is a lightweight HTTP proxy designed to sit between LLM clients (such as Anthropic or OpenAI SDKs) and multiple LLM providers. It handles routing, authentication, rate limiting, and session history with minimal dependencies and configuration.

## High-Level Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  LLM Client     │────▶│   tinyroute      │────▶│ LLM Provider 1 │
│  (SDK/App)      │     │   HTTP Proxy     │     │ (Anthropic)     │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                               │
                               ├────▶ ┌─────────────────┐
                               │      │ LLM Provider 2 │
                               │      │ (OpenAI)        │
                               │      └─────────────────┘
                               │
                               └────▶ ┌─────────────────┐
                                      │ LLM Provider N │
                                      │ (Azure/etc.)    │
                                      └─────────────────┘
```

## Package Structure

```
main.go                  # Minimal entry point (runs internal/cli)
internal/
  cli/                   # CLI framework and commands
    cli.go              # CLI setup with New() function
    serve.go            # HTTP server command
    commands.go         # All other command implementations
  agent/                 # Coding agent config adapters (claude, codex, etc.)
  auth/                  # API key management and rate limiting
  config/                # Configuration loading with hot-reload watchers
  core/                  # Shared interfaces (Dialect, Provider, etc.)
  dialect/               # LLM protocol adapters (anthropic, openai)
  history/               # Request logging and blob storage
  preset/                 # Provider templates (presets.json)
  proxy/                 # HTTP proxy handler with retry logic
  route/                 # Request routing and health tracking
```

## Core Architecture Components

### 1. Core Package (`internal/core/`)

The foundation of tinyroute's architecture, defining all interfaces and shared types that enable the system's modularity and extensibility.

#### Key Interfaces

**Dialect Interface**
```go
type Dialect interface {
    Name() string
    Paths() []string
    ParseRequest(body []byte) (ParsedRequest, error)
    RewriteModel(body []byte, model string) ([]byte, error)
    AuthHeaders(cred string, headers map[string]*string) http.Header
    NewUsageScanner() UsageScanner
    WriteError(w http.ResponseWriter, status int, errType string, message string)
    InjectUsageOption(body []byte) ([]byte, bool)
}
```
- **Purpose**: Abstracts wire-format specifics for different LLM API protocols
- **Implementations**: `anthropic`, `openai`
- **Key Capability**: Enables protocol-aware request handling and response generation

**Router Interface**
```go
type Router interface {
    Resolve(surface string, model string) (ResolvedRoute, error)
}
```
- **Purpose**: Resolves client model requests into provider/account/model hops
- **Strategy**: Two-path resolution: named combo or `provider[@account]:model` prefix

**HealthStore Interface**
```go
type HealthStore interface {
    Available(provider string) bool
    Penalize(provider string, duration time.Duration)
    CooldownEnd(provider string) time.Time
    Save() error
    Load() error
}
```
- **Purpose**: Tracks provider availability and implements cooldowns
- **Strategy**: Time-based penalty system for failed providers

**KeyVerifier Interface**
```go
type KeyVerifier interface {
    Verify(token string) (keyID string, err error)
}
```
- **Purpose**: Authenticates inbound requests using internal API keys
- **Strategy**: Token-based validation with scope checking

### Agent Adapter Registry (`internal/agent/`)

Provides a modular registry of 13 coding agent adapters (`claude`, `codex`, `cline`, `copilot`, `deepseek`, `devin`, `droid`, `grok`, `hermes`, `jcode`, `kilo`, `openclaw`, `opencode`) that configure downstream coding tools to route requests through tinyroute.

#### Agent Interface
```go
type Agent interface {
    ID() string
    Name() string
    Dialect() string
    NeedsModel() bool
    Detect() (Status, error)
    Apply(input ApplyInput) (Result, error)
    Reset() error
}
```
- **Safety Guarantee**: Backs up existing config (`.tinyroute.bak`), merges fields preserving unknown user options, writes atomically at mode `0600`, and provides scoped reset (`Reset()`).
- **Deferred Adapters**: `cowork` (MCP registry) and `antigravity-mitm` (interception proxy) are deferred out of scope.


#### Core Types

**RequestRecord**
```go
type RequestRecord struct {
    Version   int
    Timestamp time.Time
    ID        string
    Key       string
    Session   string
    Endpoint  string
    ModelReq  string
    Stream    bool
    Attempts  []Attempt
    Usage     *Usage
    ReqBlob   string
    RespBlob  string
    Outcome   Outcome
}
```
- **Purpose**: Complete audit trail of each proxied request
- **Storage**: JSON lines format in `requests.jsonl`

**Failure Classification**
```go
type FailureClass int

const (
    FailureRetryWithCooldown    // connection error, 5xx, 429
    FailureRetryNoCooldown      // 404
    FailureNoRetryWithCooldown  // 401/403
    FailureNoRetryNoCooldown    // other 4xx
)
```
- **Purpose**: Determines retry behavior and cooldown duration
- **Strategy**: Intelligent failure categorization for optimal routing

### 2. Proxy Package (`internal/proxy/`)

The orchestrator that implements the attempt loop and SSE relay logic.

#### Architecture Principles

**Dependency Injection Pattern**
```go
type Deps struct {
    Transport   *http.Transport
    GetProvider func(name string) (ProviderInfo, bool)
    GetDialect  func(name string) (core.Dialect, bool)
    Health      core.HealthStore
    Selector    core.Selector
    Recorder    core.Recorder
    BlobStore   core.BlobStore
    CaptureMode string
    InjectUsage bool
    Cooldown429 time.Duration
    Cooldown5xx time.Duration
}
```
- **Design**: All dependencies injected as interfaces or function values
- **Benefit**: Testable, modular, no sibling package imports
- **Pattern**: Pure dependency injection with no framework coupling

**Attempt Loop Strategy**
```go
for _, hop := range availableHops {
    // 1. Resolve provider and dialect
    // 2. Rewrite request body with target model
    // 3. Send outbound request
    // 4. Classify response
    // 5. Apply penalty if failed
    // 6. Break on success or non-retryable error
}
```
- **Key Design**: First-byte commitment (D5) - once response starts, no further hops
- **Failure Handling**: Intelligent classification with cooldowns
- **Streaming**: SSE relay with per-chunk flushing (no whole-response buffering)

### 3. Route Package (`internal/route/`)

Handles request routing and health tracking.

#### Router Implementation

**Two-Path Resolution**
- **Combo Resolution**: A model identifier matching a configured `Combo` resolves to its panel of member hops (ordered fallback, pool, or fused).
- **Prefix Resolution**: An identifier in `provider[@account]:model` or `provider:model` format resolves directly to that provider/account.
- **Unprefixed Rejection**: Unprefixed non-combo models fail with an actionable error directing clients to use a prefix or define a combo.

**Health Tracking**
```go
type HealthStore struct {
    // provider -> cooldown end time
    cooldowns map[string]time.Time
    clock     Clock
    path      string
}
```
- **Strategy**: Time-based cooldown with persistent state and per-model isolation (`provider/account#model`)
- **Persistence**: Saved to `state.json` for restart recovery
- **Clock Abstraction**: Enables testing with fake time

#### Tiered Budget Fallback & Quota Gates

Tiered fallback is implemented seamlessly using multi-member `Combo` configurations paired with quota-gated accounts:
- **Subscription Tier**: High-performance or primary accounts with token/request limits (`Account.Quota`).
- **Cheap / Secondary Tier**: Cost-effective fallback account or provider once subscription quota is exhausted.
- **Free Tier**: Rate-limited or free tier account as the final safety net in the combo panel.

Example configuration in `config.yaml`:
```yaml
providers:
  openai:
    dialect: openai
    base_url: https://api.openai.com/v1
    selection: sticky_round_robin
    sticky_limit: 5
    accounts:
    - name: subscription
      type: static
      api_key: ${PROD_KEY}
      quota:
        window: 86400s
        tokens: 1000000
    - name: pay_as_you_go
      type: static
      api_key: ${BACKUP_KEY}
  deepseek:
    dialect: openai
    base_url: https://api.deepseek.com/v1
    api_key: ${DEEPSEEK_KEY}

combos:
- name: smart-fallback
  members:
  - openai@subscription:gpt-4o
  - openai@pay_as_you_go:gpt-4o
  - deepseek:deepseek-chat
```
When `openai@subscription` exhausts its rolling 24-hour quota, the proxy automatically skips it pre-request and descends to `openai@pay_as_you_go`, and finally to `deepseek:deepseek-chat` if needed.

### 4. Auth Package (`internal/auth/`)

Handles API key management and rate limiting.

#### Key Management

**KeyStore Structure**
```go
type KeyStore struct {
    Keys map[string]*Key
}

type Key struct {
    ID        string
    Token     string
    Enabled   bool
    Expires   *time.Time
    Scope     []string
    Rate      *RateSpec
}
```
- **Storage**: JSON file (`keys.json`)
- **Hot Reload**: File watcher for runtime updates
- **Features**: Expiration, scope-based access, per-key rate limits

#### Rate Limiting

**Token Bucket Strategy**
```go
type RateLimiter struct {
    getRateSpec func(keyID string) *RateSpec
    // keyID -> limiter state
}
```
- **Strategy**: Per-key rate limiting with configurable windows
- **Response**: `Retry-After` header on limit exceeded
- **Hot Reload**: Rate specs updated from KeyStore changes

### 5. Dialect Package (`internal/dialect/`)

Protocol adapters for different LLM providers.

#### Architecture Pattern

**Registry Pattern**
```go
var dialects = map[string]core.Dialect{
    "anthropic": &anthropic.Dialect{},
    "openai":    &openai.Dialect{},
}
```
- **Design**: Pluggable dialect registration
- **Isolation**: Each dialect owns its wire format logic
- **Extensibility**: New providers added as dialect implementations

**Key Capabilities**

1. **Request Parsing**: Extract routing-relevant fields (model, stream, session inputs)
2. **Model Rewriting**: Replace model field while preserving rest of request
3. **Auth Shaping**: Provider-specific authentication headers
4. **Usage Extraction**: Token usage from SSE responses
5. **Error Envelopes**: Native error response formats

### 6. Config Package (`internal/config/`)

Configuration management with hot-reload capability.

#### Hot Reload Architecture

**File Watcher Pattern**
```go
type Watcher[T any] struct {
    path    string
    parser  func([]byte) (T, error)
    current atomic.Value
    reload  chan struct{}
}
```
- **Strategy**: File system watcher for runtime updates
- **Type Safety**: Generic watcher for different config types
- **Atomic Updates**: Thread-safe config access

**Configuration Types**
- **Service Config**: Environment variables, immutable after load
- **Topology Config**: Providers and routes, hot-reloaded
- **Keys Config**: API keys and rate limits, hot-reloaded

### 7. Preset Package (`internal/preset/`)

Manages built-in provider presets, dialect mappings, credential variables, OAuth metadata, and tier classifications.

#### Preset Catalog & Classification Reference
- **Provider Categories**:
  - **API Key Providers**: Standard REST API keys (`openai`, `anthropic`, `gemini`, `deepseek`, etc.).
  - **OAuth-Capable Providers**: Supports OAuth authorization flows (`xai`, `qwen`, `github`, `claude`, `codex`).
  - **Free / Freemium Tiers**: Sourced from upstream catalog data:
    - `tier: "free"`: Zero cost, no credential required.
    - `tier: "freemium"`: Free quota allocation available, credential still required (e.g. Gemini 15 RPM free allocation).
- **OAuth Flow Support**: Supports Device Code Flow (`device_code`) and PKCE Authorization Code Flow (`pkce`).
- **Auditability**: Preset client IDs, endpoints, scopes, refresh profiles, and risk notices are centralized in `catalog.go` / `presets.json` for auditability.

### 8. Credential Package (`internal/credential/`)

Handles dynamic OAuth tokens and static API keys with secure custodian storage.

#### Credential Interface & OAuth Strategy
- **`Credential` Interface**: Strategy for resolving tokens (`Token(ctx context.Context) (TokenResult, error)` returning `KindStatic` or `KindOAuthBearer`).
- **`StaticKey`**: Wraps fixed API key strings.
- **`OAuthRefreshable`**: Proactive OAuth token refresher using singleflight locks (`provider:tokenSuffix`), lead-time refresh (default 5m), and 10s result caching.
- **Custodian Store (`store.go`)**: Manages `credentials.json` with strict mode `0600` permissions and atomic `tmp+rename` file writes to prevent token corruption or plaintext exposure. Hot-reloaded via lightweight internal file watcher. Provides `ListMasked()` and `OAuthRecord.Masked()` for safe CLI rendering.
- **Masking & Security**: Refresh/access tokens are masked in CLI renderings (`tinyroute auth status`) and never logged to audit trails (`requests.jsonl`) or stdout. `Store.List()` returns unmasked structs and requires callers to mask before display.

#### OAuth Provider ToS & Revocation Trade-Offs
- **Client ID Provenance**: Subscriber OAuth client IDs for proprietary tools (e.g. Claude Code, GitHub Copilot, Codex) belong to the upstream tool authors.
- **Revocation Risk**: Proprietary providers may update Terms of Service (ToS), execute automated client/origin detection, or revoke client IDs and active access/refresh tokens.
- **Risk Notices**: Presets subject to revocation checks carry a `RiskNotice` field (e.g. `claude`, `github`) surfaced during CLI commands (`auth login`, `provider add`, `provider list`) to notify operators prior to connecting.
- **Manual Import Alternative**: Operators concerned about ToS or automated browser authentication flows can bypass interactive OAuth login via `tinyroute auth import <provider>`, reusing tokens acquired directly through official tools.

#### Recording Strategy

**Async Recording**
```go
go recordOutcome(deps, reqCtx, r.URL.Path, parsed, body, respBody, attempts, finalUsage, outcome)
```
- **Design**: Off-critical-path recording
- **Benefit**: Never delays response to client
- **Format**: JSON lines for append-only logging

#### Blob Storage

**Content-Addressable Storage**
```go
type BlobStore struct {
    dir string
}
```
- **Strategy**: SHA256-content addressing
- **Location**: `~/.tinyroute/blobs/`
- **Usage**: Optional, enabled by `TINYROUTE_CAPTURE=full`

## Request Flow Architecture

### 1. Client Request Processing

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. HTTP Request Received                                         │
│    ├─ Path matching (by dialect)                                │
│    ├─ Authorization header extraction                           │
│    └─ Body buffering (32MB limit)                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. Authentication & Authorization                               │
│    ├─ Bearer token validation                                  │
│    ├─ Key store lookup (hot-reloaded)                           │
│    ├─ Scope check (surface + model)                            │
│    └─ Rate limit check (per-key)                                │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. Request Parsing & Route Resolution                           │
│    ├─ Dialect-specific request parsing                         │
│    ├─ Model identification                                     │
│    ├─ Route matching (glob patterns)                            │
│    └─ Chain resolution (with $model passthrough)                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 4. Attempt Loop (Proxy Orchestrator)                            │
│    ├─ Select available hops (health-filtered)                  │
│    ├─ For each hop:                                             │
│    │   ├─ Resolve provider + dialect                           │
│    │   ├─ Rewrite request body                                 │
│    │   ├─ Send outbound request                                │
│    │   ├─ Classify response                                    │
│    │   ├─ Apply penalty if failed                               │
│    │   └─ Break on success or non-retryable error              │
│    └─ Handle chain exhaustion                                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. Response Relay                                                │
│    ├─ Streaming: SSE relay with per-chunk flush                 │
│    ├─ Non-streaming: Direct relay                               │
│    ├─ Usage extraction (token counts)                           │
│    └─ Capture (optional: blob storage)                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 6. Async Recording (Off Critical Path)                          │
│    ├─ Request record persistence                               │
│    ├─ Blob storage (if full capture)                           │
│    └─ Health state persistence                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 2. Error Handling Strategy

**Failure Classification Matrix**

| Status Code | Failure Class                    | Retry Behavior | Cooldown | Strategy                           |
|-------------|----------------------------------|-----------------|----------|-----------------------------------|
| 0           | FailureRetryWithCooldown         | Next hop        | 10s      | Connection error/timeout          |
| 429         | FailureRetryWithCooldown         | Next hop        | 60s      | Rate limit (honors Retry-After)   |
| 5xx         | FailureRetryWithCooldown         | Next hop        | 10s      | Server error                      |
| 404         | FailureRetryNoCooldown           | Next hop        | None     | Model not at this provider       |
| 401/403     | FailureNoRetryWithCooldown       | Return error    | 15min    | Auth failure (log warning)        |
| Other 4xx   | FailureNoRetryNoCooldown         | Return error    | None     | Client error (return as-is)       |

## Design Decisions & Patterns

### 1. Interface-Driven Architecture

**Decision**: All major components defined as interfaces in `core` package
**Rationale**: 
- Testability: Easy to mock for testing
- Modularity: Components can be swapped independently
- Extensibility: New implementations without modifying existing code

### 2. Dependency Injection

**Decision**: All dependencies injected via `Deps` struct
**Rationale**:
- No circular dependencies between internal packages
- Clear dependency graph
- Easy to test with fake implementations

### 3. Hot-Reload Configuration

**Decision**: File watchers for runtime config updates
**Rationale**:
- Zero-downtime routing updates
- API key rotation without restart
- Rate limit changes immediate effect

### 4. Async Recording

**Decision**: Request logging off critical path
**Rationale**:
- Never delays response to client
- Storage failures don't affect proxy operation
- Better latency for client requests

### 5. First-Byte Commitment

**Decision**: No retry after first byte of response sent
**Rationale**:
- Prevents mid-request retries (client would see corruption)
- Clear failure semantics
- Respects streaming nature of LLM responses

### 6. Dialect Pattern

**Decision**: Protocol-specific logic encapsulated in dialects
**Rationale**:
- Provider-specific formats isolated
- Easy to add new providers
- Clear separation of routing vs. protocol logic

### 7. Health-Based Selection

**Decision**: Provider availability tracked with cooldowns
**Rationale**:
- Automatic failover for failing providers
- Configurable penalty durations
- Persistent state across restarts

## Scalability & Performance Considerations

### 1. Request Buffering

**Design**: 32MB limit on request body buffering
**Rationale**: Required for chain retry, but bounded to prevent memory exhaustion
**Trade-off**: Memory vs. retry capability

### 2. Streaming Response Handling

**Design**: SSE relay with per-chunk flushing
**Rationale**: 
- Minimal latency for streaming responses
- No whole-response buffering
- Respects streaming semantics

### 3. HTTP Transport Configuration

**Design**: `ResponseHeaderTimeout` bounds failover window
**Rationale**:
- Prevents indefinite hangs on failing providers
- Configurable per deployment
- No whole-request deadline (streaming safety)

### 4. Async Recording

**Design**: Goroutine-based recording
**Rationale**:
- Zero impact on request latency
- Isolated failure domains
- Configurable capture modes

## Security Architecture

### 1. Authentication Layer

**Strategy**: Internal API keys with bearer token authentication
**Features**:
- Token-based validation
- Scope-based access control (surface + model)
- Key expiration support
- Enable/disable capability

### 2. Rate Limiting

**Strategy**: Per-key rate limiting with token bucket
**Features**:
- Configurable rate windows
- Hot-reload rate specs
- Standard `Retry-After` responses

### 3. Provider Credential Isolation

**Design**: Provider credentials never exposed to clients
**Implementation**: Upstream auth headers set exclusively by dialect
**Benefit**: Client can't extract provider credentials

### 4. Session Tracking

**Capability**: Request history with optional full capture
**Privacy**: Configurable capture mode (off/metadata/full)
**Storage**: Content-addressed blobs for full capture

## Extension Points

### 1. Adding New Dialects

**Steps**:
1. Implement `core.Dialect` interface
2. Register in `internal/dialect` package
3. Add preset template (optional)

**Example**: Adding Google Gemini support
```go
type GeminiDialect struct{}

func (d *GeminiDialect) Name() string { return "gemini" }
func (d *GeminiDialect) Paths() []string { return []string{"/v1/models"} }
// ... implement other methods
```

### 2. Custom Health Stores

**Extension Point**: `core.HealthStore` interface
**Use Cases**: 
- Distributed health tracking
- Custom cooldown strategies
- External state management

### 3. Alternative Selectors

**Extension Point**: `core.Selector` interface
**Use Cases**:
- Weighted routing
- Latency-based selection
- Cost-optimized routing

### 4. Custom Recorders

**Extension Point**: `core.Recorder` interface
**Use Cases**:
- Remote logging services
- Analytics integration
- Custom audit formats

## Testing Strategy

### 1. Unit Testing

**Focus**: Individual components in isolation
**Approach**: Interface mocking with fakes
**Example**: Fake clock, fake health store

### 2. Integration Testing

**Focus**: Component interactions
**Approach**: Real implementations with test fixtures
**Example**: Full request flow with test providers

### 3. Hot Reload Testing

**Focus**: Configuration changes during operation
**Approach**: File modification during request processing
**Validation**: Atomic config switches, no request loss

## Deployment Architecture

### 1. Single Binary Deployment

**Design**: All dependencies bundled, no runtime dependencies
**Distribution**: Go binary executable
**Configuration**: File-based (`~/.tinyroute/`)

### 2. Environment-Based Configuration

**Strategy**: `TINYROUTE_*` environment variables
**Categories**:
- Service settings (listen address, TLS)
- Capture mode (metadata/full)
- Cooldown durations
- File paths (config, keys, state)

### 3. State Persistence

**Files**:
- `config.yaml` - Provider and route definitions
- `keys.json` - API keys and rate limits
- `state.json` - Provider cooldown state
- `requests.jsonl` - Request history
- `blobs/` - Content-addressed blob storage

## Monitoring & Observability

### 1. Request Logging

**Format**: JSON lines (newline-delimited JSON)
**Fields**: Request metadata, attempts, usage, outcome
**Location**: `requests.jsonl`

### 2. Health Tracking

**Metrics**: 
- Provider cooldown state
- Attempt counts per provider
- Failure classifications

### 3. Session Tracking

**Capability**: Request grouping by session
**Derivation**: Explicit header or fingerprint (system + first message)
**Use Case**: Conversation replay and debugging

## Future Architecture Considerations

### 1. Distributed State

**Current**: File-based state in single process
**Potential**: Shared state for multi-instance deployments
**Challenges**: Health coordination, distributed locking

### 2. Advanced Routing

**Current**: Pattern-based with ordered chains
**Potential**: 
- Weighted selection
- Cost-based routing
- A/B testing support

### 3. Plugin Architecture

**Current**: Compiled-in dialects and selectors
**Potential**: Dynamic loading via plugin system
**Benefits**: User-provided extensions without recompilation

### 4. Multi-Protocol Translation

**Current**: Same-protocol routing (Anthropic→Anthropic)
**Potential**: Cross-protocol translation (OpenAI→Anthropic)
**Status**: Translator interface defined, implementations pending

## Conclusion

tinyroute's architecture embodies principles of modularity, testability, and extensibility through:

1. **Interface-driven design** enabling component swapping
2. **Dependency injection** preventing circular dependencies  
3. **Hot-reload configuration** supporting zero-downtime updates
4. **Async operations** maintaining low latency
5. **Protocol abstraction** through the dialect pattern

The result is a lightweight, flexible LLM proxy that can be easily extended with new providers, routing strategies, and integrations while maintaining clean separation of concerns and testability.
