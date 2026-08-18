# Design: Refine the Client Detail Page

## Context

The client detail view (`internal/dashboard/view_client_detail.templ`) is hand-rolled dark-slate
Tailwind + Alpine, using a native `<dialog>` for the model picker. Root causes found during
exploration:

1. **Broken picker**: `view_client_detail.templ:247` builds
   `:class={ fmt.Sprintf("w-full flex … transition-colors slots['%s'] === %q ? '…' : '…'", …) }`.
   The result is a single string that is neither a valid class list nor a valid JS expression;
   Alpine fails to evaluate it and the model buttons end up with no classes at all. The
   `(None / Default)` row works because it uses a plain static `class` attribute.
2. **Stale CSS**: `internal/dashboard/assets/styles.css` was compiled before recent class
   additions — `px-5` (Apply Changes button) and `max-h-[65vh]` (picker scroll area) are absent
   from the compiled output. The Taskfile pipeline is `npx -y tailwindcss@3.4.19 -i input.css
   -o styles.css`; nothing rebuilds it automatically on templ edits.
3. The shadcn-templ `dialog` component is installed (`internal/dashboard/components/dialog`)
   and its theme is already wired (`input.css` imports `tw-animate.css` + `shadcn-tailwind.css`),
   but its behavior script `dialog.js` (~28 KB vanilla Base-UI port) is embedded and served
   nowhere — `assets.go` embeds only `styles.css` and `logos/*`, and the layout loads only
   Alpine from a CDN.

`clientLogo(id, name)` in `view_clients.templ:71` already renders embedded brand SVGs with a
monogram fallback and is callable from the detail view (same `dashboard` package).

## Goals / Non-Goals

**Goals:**

- A model picker that actually works: modal dialog, uniform styled option rows (including
  `(None / Default)`), visible selected state, select-and-close behavior.
- Header with client logo and no metadata clutter; no helper paragraphs; no dead
  "Experimental Settings" block.
- Compiled CSS that matches the templ sources.
- The shadcn-templ dialog component becomes usable dashboard-wide (script mounted once in the
  layout).

**Non-Goals:**

- Re-skinning the whole detail page (or dashboard) with shadcn-templ components — only the
  picker moves to the dialog component; the rest keeps its hand-rolled styling.
- Any backend/API/config-format changes; the Alpine state model (`slots`, `oneM`, key strategy,
  plan preview flow) stays as is.
- Automating CSS rebuilds (watch mode already exists via `task dev`); this change just
  regenerates the output once.

## Decisions

### D1: Rebuild the picker on the shadcn-templ `dialog` component

Chosen over the minimal "fix the `:class` string" repair (which would keep the native
`<dialog>` and hand-rolled chrome). Rationale: the component gives backdrop, Esc / click-away
dismiss, focus trap, and ARIA wiring for free, and it is the direction the dashboard is already
invested in (components installed, theme imported). Cost: the script bundle must be mounted
(D2) and the dialog's default `bg-popover` theming must be overridden to match the page's
dark-slate look via `ContentProps.Class` (e.g. `bg-slate-900 border border-slate-800
ring-0 w-[min(28rem,90vw)] sm:max-w-none`).

### D2: Serve `dialog.js` via a package-local embed + a dashboard route

`go:embed` cannot reach from `assets/` into `components/dialog/`, so the component package
embeds its own script: new `internal/dashboard/components/dialog/js.go` with
`//go:embed dialog.js` exposing `FS()` (or bytes). `handler.go` registers
`GET /dashboard/assets/dialog.js` next to the existing assets handler, and `view_layout.templ`
gains `<script defer src="/dashboard/assets/dialog.js"></script>` beside the Alpine tag.
Alternative rejected: copying `dialog.js` into `assets/` — duplicates 28 KB and drifts from the
component on upgrade.

### D3: Class-binding discipline for option rows

Every option row (None/Default and each model) is the **same** button component with a static
`class` attribute (the current None/Default styling: `w-full flex items-center justify-between
px-3 py-2.5 rounded-lg border …`). Selected state is expressed only through (a) a `:class`
attribute whose value is a **pure expression** evaluating to the accent classes or `''` —
never static classes baked into the expression — and (b) an `x-show` check icon. Alpine merges
`class` and `:class`, so this renders correctly and degrades to a styled button even if the
binding fails.

### D4: Dialog wiring per slot

Each slot wraps its trigger button and content in
`@dialog.Dialog(dialog.Props{ID: "picker_" + slot.ID})` so `Trigger(ctx)`/`Close(ctx)` resolve
to a deterministic per-slot id (multiple dialogs live on one page; the component's random-ID
default is not acceptable here). The slot button keeps its current styling and spreads
`{ dialog.Trigger(ctx)... }` (replacing `@click="$refs.picker_x.showModal()"`). Each option row
spreads `{ dialog.Close(ctx)... }` **and** keeps its Alpine `@click` that sets
`slots['<id>']`/`oneM` — the two mechanisms are independent attributes and coexist. The
`<dialog x-ref>` elements and the `.model-picker-dialog` CSS in `input.css` are removed.

### D5: Regenerate artifacts through the existing pipeline

`templ generate`, then one-shot Tailwind rebuild
(`npx -y tailwindcss@3.4.19 -i internal/dashboard/assets/input.css -o
internal/dashboard/assets/styles.css`), then `gofmt -w .`. No pipeline changes.

## Risks / Trade-offs

- [Dialog script load order — `data-tui-dialog-*` elements rendered before the script runs] →
  the script is written for SSR'd markup and initializes on load (same pattern as the input
  component); `defer` placement after Alpine avoids blocking. Verified in the manual check.
- [Dialog default theming (vega/stone, `bg-popover`) clashing with the slate page] →
  `ContentProps.Class` overrides background/ring/width; check contrast in the manual pass.
- [Stale CSS regressing again on future templ edits] → pre-existing issue, out of scope here;
  `task dev` watch mode already exists for development.
- [No existing dashboard usage of the dialog component — this is its first consumer] → keep the
  integration surface minimal (Content/Header/Title/Trigger/Close only) so any component defect
  surfaces locally in this page, and cover open/select/close in the manual verification.

## Migration Plan

Single reversible commit: revert restores the native `<dialog>` picker and removes the route,
script tag, and embed. No data or config migrations.

## Open Questions

None — picker approach (shadcn-templ dialog) and helper-text removal (all paragraphs) were
confirmed by the user during exploration.