# Design: refine-combo-cards

## Context

The combos page (`internal/dashboard/view_combos.templ`) renders one card per
combo: header (layers icon, name at `text-sm`, mode badge), content (full
member list as numbered rows, capabilities badges), footer (Edit / Delete).
Members are stored as `provider[@account]:model`. The gateway serves with a
`config.Watcher` on `config.yaml`; the dashboard persists via
`config.WriteTopology`, so config writes hot-reload the live router. No
`enabled`/`disabled` concept exists anywhere today.

## Goals and Non-Goals

**Goals**

- Cards stay scannable at any member count.
- One-click, reversible parking of a combo, effective on the live gateway.
- Explicit (debuggable) behavior when a client requests a disabled combo.

**Non-Goals**

- CLI display of enabled state, discovery-list filtering, wizard changes
  (see proposal Non-Goals).

## Decisions

### Decision 1: `disabled` flag, negative polarity

```go
type Combo struct {
    Name         string   `yaml:"name"`
    Members      []string `yaml:"members"`
    Mode         string   `yaml:"mode,omitempty"`
    Capabilities []string `yaml:"capabilities,omitempty"`
    Disabled     bool     `yaml:"disabled,omitempty"`
}

// IsEnabled reports whether the combo participates in routing.
func (c Combo) IsEnabled() bool { return !c.Disabled }
```

Absent = enabled, so every existing config is unaffected and the YAML zero
value is the safe default. A negative field avoids `Enabled *bool`
nil-checks at every call site; the `IsEnabled()` helper keeps predicate call
sites reading positively.

**Alternatives considered:** `Enabled *bool` with nil-means-true (pointer
nil-checks everywhere); `Enabled bool` (zero value silently disables every
existing combo on upgrade — rejected outright).

### Decision 2: explicit error on disabled resolution; parents skip

Two resolution entry points check the flag (`internal/route/router.go`):

- **Direct request** (`Resolve` step 0, and the provider-position lookup):
  a disabled combo resolves to an explicit error —
  `combo %q is disabled` — which surfaces to the client as a 404-style
  "unknown model" response. No silent fall-through: a `combo-*` route
  pattern could otherwise swallow the request and produce baffling
  behavior.
- **As a member of a parent combo** (member enumeration during chain
  building): a disabled sub-combo is treated like any member whose
  resolution fails — skipped, and the parent falls back to its next member
  (ordered mode). A parent whose members are all disabled/unusable fails
  with its normal all-members-failed error. This mirrors the existing
  failure semantics rather than inventing a new one.

`Models()` continues to include disabled combo names: they still exist, and
the explicit request error is clearer than a name that silently vanishes
from discovery.

### Decision 3: toggle is a form POST; the switch is the submit

```html
<form method="POST" action="/dashboard/combos/toggle">
    <input type="hidden" name="name" value="coding-priority"/>
    <button type="submit" role="switch" aria-checked="false" aria-label="..."/>
</form>
```

shadcn's Switch renders as a `role=switch` **button**, so it submits its
enclosing form natively — no fetch, no JS, consistent with Edit/Delete and
the dashboard's CSRF form-POST protection. The handler flips the flag,
persists via `config.WriteTopology`, and redirects back with a flash;
the topology watcher picks up the write (already-proven hot-reload path).

**Alternatives considered:** fetch + JSON endpoint (adds a JS surface the
page otherwise avoids); toggle-in-header (state control crowding identity;
footer groups all actions).

### Decision 4: member display — cap 3, plain pin strip

Display transform at render time only (stored strings are never rewritten):

```
stored:   antigravity@main:gemini-flash-2.5
display:  antigravity:gemini-flash-2.5
```

- At most 3 member rows render; a 4th "overflow row" shows `+N more…`
  where N = total − 3, with a `title` attribute listing the hidden members
  for hover inspection.
- The `@account` pin is stripped **plainly** — same-model-different-account
  members may render as apparent duplicates. Accepted tradeoff (decided in
  exploration): the redundancy is intentional when it occurs, and
  collision-aware suffixes complicate the row↔member invariant that keeps
  the overflow count trivial.
- Wizard/edit/review keep showing members verbatim, so round-trip fidelity
  is unaffected.

### Decision 5: card visuals

- Title: `text-sm` → `text-base`.
- Disabled card: muted treatment (`opacity-60` on the card, muted icon
  treatment) so state is legible at a glance, not only from the switch
  position.

## Risks / Trade-offs

- **Plain-strip duplicates** could look like a rendering bug on
  redundancy pools; mitigated by the wizard still showing pins, and the
  overflow row's `title` tooltip carrying full strings if we later choose
  to include them.
- **Skip-on-disabled in parents** adds one branch to member enumeration;
  kept aligned with existing failed-member handling to avoid divergent
  fallback semantics.

## Migration Plan

None needed: `disabled` is `omitempty`; configs without the field parse and
behave exactly as today.

## Open Questions

None — all four decisions (collision display, routing semantics, toggle
placement, single-change scope) were settled during exploration on
2026-08-19.
