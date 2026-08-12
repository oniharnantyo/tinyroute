# Implementation Tasks

> Scope expanded after review: card/detail UI revisions (Track A) + in-browser
> OAuth connect (Track B). Earlier groups (1–6) built the grouped providers page
> and two-grid model section; groups 7–8 revise that UI per new feedback, and
> group 9 adds OAuth.

## 1. Data model & providers-page handler  (DONE)

- [x] 1.1 Extend `ProviderCardData` (`Configured`, `AuthType`, `Tier`, `FreeNote`, `Section`)
- [x] 1.2 `ProvidersPageData.Sections []ProviderSection`
- [x] 1.3 `handleProvidersView` buckets presets into Free Tier → OAuth → API Key (tiered = priority pull)
- [x] 1.4 Populate counts/health for configured cards

## 2. Providers page view  (DONE, revised by group 7)

- [x] 2.1 Heading → "Providers"
- [x] 2.2 Render grouped sections with counts
- [x] 2.3 Per-card badges + Manage/Configure CTAs  ← superseded by 7.2/7.3
- [x] 2.4 Cross-section search
- [x] 2.5 Custom-provider add path

## 3–4. Detail dedup + two-grid model section  (DONE, revised by group 8)

- [x] 3.1 Exclude whitelisted from available models
- [x] 4.1–4.4 Available Models grid + filter + show-more  ← superseded by 8.2

## 5–6. Build & tests  (DONE)

- [x] 5.1–5.3 templ generate, gofmt, go build
- [x] 6.1–6.5 handler tests + manual verify

## 7. Provider card UI revision (`view_providers.templ`)

- [x] 7.1 Add a `titleCase` helper; card title uses the Title-Cased display name (`opencode-zen`→`Opencode Zen`, `anthropic`→`Anthropic`), bumped one size larger
- [x] 7.2 Remove the base URL line, the healthy/cooldown flag, and the Manage/Configure buttons from the card
- [x] 7.3 Make the whole card a clickable link to `/dashboard/providers/{name}` (configured or not)
- [x] 7.4 Remove the orphan `ProvidersPageData.Presets` field and dead Alpine state vars

## 8. Provider detail UI revision (`view_provider_detail.templ` + `handler.go`)

- [x] 8.1 Title-Case the display name in the breadcrumb and `<h2>`
- [x] 8.2 Merge whitelisted + available into a single **Models** section: handler produces one annotated list (model id + `Whitelisted bool`); whitelisted rows render as active (Test/Remove), the rest render a "+ Add" action
- [x] 8.3 Relax the configured-only guard: an unconfigured preset opens the detail page with an "encourage to configure" message + a Configure action (posts `preset_name`), and no model list
- [x] 8.4 Remove the Endpoint Settings section

## 9. OAuth in-browser connect (Track B)

- [x] 9.1 `internal/oauth` runner extracted from `cli/auth.go` (PKCE authorize/exchange + device-code start/poll, `ConfigFromPreset`, `Tokens` shaped for `credential.NewOAuthRefreshable`)
- [x] 9.2 Dashboard routes mirroring the reference `/oauth/{provider}/{action}` pattern: `GET /dashboard/providers/{name}/oauth/start`, `GET /dashboard/oauth/callback`, plus device start/poll; persist verifier/state server-side
- [x] 9.3 On the detail page, for OAuth-capable providers show connection status (from `ListMasked`) + a **Connect** button that starts the flow
- [x] 9.4 Store resolved tokens via the credential store (`credential.NewOAuthRefreshable`) and reflect the masked connection

## 10. Verify

- [x] 10.1 `templ generate`, `gofmt -w .`, `go build ./...`, `go test ./...`
