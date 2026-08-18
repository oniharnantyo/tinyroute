# Refine the Client Detail Page

## Why

The client detail view at `/dashboard/clients/{id}` has a functionally broken model picker —
the Alpine `:class` binding on model option buttons glues static Tailwind classes onto the front
of a JavaScript ternary, which is invalid JS; Alpine silently drops the binding and every model
option renders as an unstyled inline button (model names run together with no borders or
spacing). Separately, the committed Tailwind build is stale (`px-5` and `max-h-[65vh]` are used
in the templ sources but missing from `styles.css`), leaving the **Apply Changes** button
without horizontal padding. The page also carries visual clutter the user wants gone.

## What Changes

- **Model picker rebuilt on the shadcn-templ `dialog` component** (installed at
  `internal/dashboard/components/dialog`): every option row — including `(None / Default)` —
  renders as the same styled row component with a visible selected state; selecting an option
  closes the dialog and updates the slot. The invalid `:class` expression pattern is eliminated
  (static classes stay in the `class` attribute; `:class` carries only a pure selected-state
  expression).
- **Component behavior JS is mounted**: the dialog component's `dialog.js` (vanilla Base-UI
  port) is embedded, served under `/dashboard/assets/`, and loaded by the dashboard layout —
  currently it exists on disk but is neither embedded nor referenced anywhere.
- **Header refined**: removes the `ID: <id> • Dialect: <dialect>` metadata subtitle; adds the
  client's brand logo using the existing `clientLogo` helper (embedded SVG with monogram
  fallback).
- **Helper paragraphs removed**: the descriptive text under "Endpoint & Key Configuration" and
  under "Model Slots" is dropped; section headings remain.
- **Experimental Settings block removed**: the non-functional "coming soon" toggles
  (Filter Naming Requests, Exa MCP / Web Search) are deleted.
- **Tailwind output rebuilt** so `styles.css` matches the templ sources (restores `px-5`,
  `max-h-[65vh]`, and includes the dialog component's classes).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `management-dashboard`: the existing "Clients are managed from the dashboard" requirement
  set gains spec-level coverage for the detail view's presentation contract: header identity
  (logo, no metadata subtitle), uniform model-picker option rows in a modal dialog, and serving
  of the component behavior scripts the dialog depends on.

## Impact

- `internal/dashboard/view_client_detail.templ` — header, helper text, picker, Experimental
  Settings removal (regenerates `view_client_detail_templ.go`).
- `internal/dashboard/components/dialog/js.go` — new: embeds `dialog.js`.
- `internal/dashboard/handler.go` — route serving the dialog script.
- `internal/dashboard/view_layout.templ` — script tag for the dialog bundle (regenerates
  `view_layout_templ.go`).
- `internal/dashboard/assets/styles.css` — rebuilt via the existing Tailwind v3 pipeline.
- No API, config, or persistence changes; purely dashboard presentation.