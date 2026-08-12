## Why

The providers dashboard lists only *configured* providers and hides the model
catalog inside a `<select>` dropdown. Users cannot see the full provider catalog
at a glance, whitelisting a model means picking from a dropdown, and OAuth
providers cannot be connected from the UI at all (only via the CLI). This change
turns the providers page into a complete, grouped catalog of all known providers,
makes every model a first-class card, lets unconfigured presets be configured
straight from their detail page, and adds in-browser OAuth connect.

## What Changes

- **Providers page**: heading `Providers`; lists **all** providers (configured ∪
  presets) grouped **Free Tier → OAuth → API Key**. Each card shows logo,
  Title-Cased display name (`opencode-zen`→`Opencode Zen`, `anthropic`→`Anthropic`),
  dialect, and auth/tier badges. The **whole card is clickable** to its detail
  view — no Manage/Configure buttons, no base URL, no health flag on the card.
  Cross-section search filters the list.
- **Provider detail**: Title-Cased name in the breadcrumb and heading. A single
  merged **Models** section lists every catalog model — whitelisted models render
  as active (Test/Remove), the rest render a "+ Add" action (one click appends to
  the whitelist). An **unconfigured** preset opens the detail page with an
  "encourage to configure" message + Configure action instead of the model list.
  The Endpoint Settings section is removed.
- **OAuth connect**: OAuth-capable providers get a **Connect** button on the
  detail page that runs the OAuth flow in-browser (PKCE authorize/callback, or
  device-code for device-flow presets), via a new reusable `internal/oauth`
  runner extracted from `cli/auth.go` and dashboard routes mirroring the
  reference `/oauth/{provider}/{action}` pattern. Resolved tokens are stored
  through the existing credential store and shown as a masked connection.

## Capabilities

### New Capabilities

_(none)_

### Modified Capabilities

- `management-dashboard`: providers list becomes grouped + clickable Title-Cased
  cards; provider detail gets a merged Models section, a not-configured state,
  and loses Endpoint Settings; and gains an in-browser OAuth connect flow.

## Impact

- **Code**: `internal/dashboard/handler.go`, `view_providers.templ`,
  `view_provider_detail.templ`, new dashboard OAuth route handlers; a new
  `internal/oauth` package (PKCE/device/token logic extracted from
  `internal/cli/auth.go`).
- **Endpoints**: new dashboard OAuth routes (start / callback / device / poll);
  reuses `POST /dashboard/providers/add`, `POST /dashboard/models/add`,
  `POST /dashboard/models/remove`.
- **Data**: none — consumes existing `preset` OAuth config, the model catalog,
  and the credential store.
- **Dependencies**: stacks on the `serve-dashboard` change (the
  `management-dashboard` capability must exist in main specs first).
