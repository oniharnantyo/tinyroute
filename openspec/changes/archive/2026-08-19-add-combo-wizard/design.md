# Design: add-combo-wizard

## Context

The routing engine already implements priority combos end-to-end:
`config.Combo` (name/members/mode/capabilities) parses from topology.yaml,
`Router.Resolve` expands a combo name into ordered hops (`internal/route/router.go`),
and the proxy's hop loop attempts members sequentially with health, quota,
account, and translation fallback. `ValidateTopology` rejects invalid combos.
What exists on the UX side is a flag-driven `combos add` (hard-errors without
typed args, violating `.claude/rules/cli-interactivity.md`) and no dashboard
combos surface. The wizard primitives (`internal/cli/interactive/`: Select,
MultiSelect, Input, Confirm, plus `RunInitWizard` as the step-header pattern)
and the shadcn-templ component kit (`internal/dashboard/components/`) are in
place.

Providers are also multi-connection now (`add-multi-account-connections`):
`Provider.Accounts[]` holds named accounts, the credential store is keyed
`provider/account`, and the engine already parses account-pinned combo
members — `expandCombo` splits `provider@account:model`
(`internal/route/router.go`), and the proxy restricts a pinned hop to that
account's credential while unpinned hops use the provider's selection
strategy (`internal/proxy/proxy.go`). The `--members` flag help already
documents `provider@account[:model]`. What is missing: the member-option
source (`config.GetMemberCandidates`) emits bare `provider:model` only, the
`model-combos` spec's member-validation requirement is unimplemented
(`ValidateTopology` checks route chains but not combo members), and the
account lifecycle handlers (rename, disconnect, `providers account remove`)
rewrite `Accounts[]` without touching combo members that pin those accounts.

## Goals / Non-Goals

**Goals:**

- `combos add` becomes the interactive-first reference for combo creation,
  compliant with `cli-interactivity.md` (args-as-escape-hatch,
  prompts-as-default)
- Dashboard gains a combos section: list, create/edit wizard in a dialog,
  delete with confirmation — all from existing components
- Both surfaces teach the same mental model: numbered members = member order
- A member can be drawn from a specific connection: pickers on both surfaces
  offer `provider@account:model` options for multi-account providers, and
  account rename/removal keeps existing pins consistent

**Non-Goals:**

- No engine, config-schema, validation, or model-discovery changes
- No ordering strategies (lkgp/weighted/round-robin), weights, tags, or
  `auto/*` computed combos — future changes
- No combo test panel, history integration, or usage stats on the dashboard
- No combo rename or member editing via the CLI wizard beyond remove/re-add
- No per-account model subsets — the whitelist is provider-level, so every
  whitelisted model is offered for every account of the provider
- No account management (add/connect/rename/remove UIs) — that is
  `add-multi-account-connections`; this change only keeps combo members
  consistent when those surfaces act

## Decisions

### D1: Reuse the engine; ship UX only — with one validation carve-out

No changes to `internal/proxy` or `internal/route`; execution, hop
resolution, and account pinning already work. `mode: ordered` (the default)
is exactly the requested semantics — try member 1, on failure member 2, etc.
Alternative (new "priority" mode or strategy field now) rejected: it adds
schema surface for behavior that already exists and would be reworked by the
future strategies change anyway.

The carve-out: `ValidateTopology` gains the combo-member checks (malformed
member, undeclared provider, unknown account) that the `model-combos` spec
already requires but the implementation lacks — today it validates route
chains with exactly these rules and skips combo members entirely. Without
it, account pins typed by hand or submitted through the typed shortcut could
reference accounts that do not exist. Both wizard write paths (`AddComboCore`
and the dashboard's `submit_create`) already gate on `ValidateTopology`, so
one fix covers every surface. See D9.

### D2: Ordered member selection by sequential single-selects (CLI)

Members are picked one at a time — "Select the FIRST member:" then
SECOND, THIRD… — with each prompt excluding already-chosen models plus a
"Done — enough members" option once ≥1 is chosen. Alternatives considered:

- *MultiSelect then a separate reorder step*: two steps to express one fact,
  and users must notice that multiselect order is meaningless before fixing
  it — worse error surface.
- *MultiSelect with pterm preserving selection order*: pterm does not
  guarantee an order-preserving multi-select contract; building on that is
  fragile.

Sequential picks make priority explicit at input time and are self-validating
(a one-member minimum enforced by when "Done" appears; combos are a general
multi-model construct — ordered, pool, fused — so no two-member floor).

Exclusion works on the **full member string**: `glm:glm-4.7` and
`glm@work:glm-4.7` are distinct members (different credential pools — one
pools every account, one uses only `work`), so picking the unpinned form
does not remove the pinned form from later prompts and vice versa. Only
byte-identical members are excluded.

### D3: Wizard structure mirrors `RunInitWizard`

Five steps with the same step-header treatment (`step N of 5` + section
title): name → members → mode → capabilities → review/confirm. The typed
shortcut (`combos add <name> --members=...`) bypasses prompts entirely and
shares the same validation + write path (single `runAdd` core, two callers —
prompt-collector and flag-collector). Name validation re-prompts inside the
wizard (charset `^[a-zA-Z0-9_.\-]+$`, no `:`, uniqueness).

### D4: Dashboard wizard lives in a dialog, not routes

Creating/editing happens in a dialog over the list page (no
`/dashboard/combos/new` navigation). Each step transition is a plain
`POST /dashboard/combos/wizard` carrying `(step, draft)`; the server
validates the step and re-renders the combos page with the dialog re-opened
at the next step (`dialog` root `Open: true`). Alternatives considered:

- *Dedicated step routes*: more routes to test, list context lost, browser
  back/forward semantics get awkward mid-wizard.
- *Client-side step state machine (JS)*: violates the "built-in components
  only" constraint — we'd be writing wizard JS the bundle doesn't ship.

Server-driven steps keep all state in one place, make every step testable as
a plain POST handler, and use only the dialog component's open/close
behavior. Per-card Edit triggers use `dialog.TriggerFor(id)` since the
trigger lives outside the dialog subtree. Draft state lives in the POSTed
form values only (no server session); Cancel/✕ discards it.

### D5: Built-in components only — mapping

| Surface | Component |
|---|---|
| Dialog shell, step container | `dialog` (+ `TriggerFor` for edit) |
| Name field, errors | `label` + `input` |
| Members: add | two `<select>`s — model (optgroup by provider) + connection (any / account) — + `button` |
| Members: reorder/remove | `button` (ghost, icon size) + `icon.ChevronUp/ChevronDown/X` |
| Mode step | `radio` |
| Capabilities step | native styled checkbox (no `checkbox` component in kit) + `label` |
| Step indicator | composed from primitives — **no stepper component exists in the kit**; this is layout-only composition (text + `separator`), permitted since no component fits |
| Combo cards, mode badge, empty state | `card`, `badge`, `empty` (+ `icon.Layers`) |
| Delete confirm | `dialog` (destructive variant pattern with warning copy, matching key revocation) |
| Feedback | `toast` (SSR `toast.Toast` success/error rendered on the post-mutation render) |
| Nav entry | `icon.Layers` between Routes and History |

Variant/size props are typed constants from each component's package; never
raw strings. The only behavior JS used is what the hashed bundle already
ships (dialog open/close, toast). Delete confirmation uses `dialog` with
destructive button styling and warning copy (consistent with the rest of
the dashboard). Step 2 uses standard `<select>` with `<optgroup>` to provide
a native provider-to-model cascade inside the dialog container.

### D6: topology.yaml stays the single source of truth

Both surfaces read and write through the config service exactly as
`provider add` does (`LoadService` → read → parse → mutate →
`ValidateTopology` → `WriteTopology`). The watcher picks up changes; no
database is introduced for combos. Concurrent CLI/dashboard edits follow the
same last-write-wins reality as the existing provider pages — the write path
re-reads the file immediately before mutating.

### D7: Option source is provider whitelists × declared accounts

CLI member options and the dashboard provider→model cascade both derive from
the parsed topology's provider model whitelists (providers with no whitelisted
models are skipped). For a provider declaring **two or more accounts**, each
whitelisted model is offered twice-per-meaning: unpinned (`provider:model` —
any account; the provider's `selection` strategy applies at request time) and
once per declared account (`provider@account:model` — that connection's
credential only). Providers with zero or one account offer unpinned options
only: with a single account the pin is semantically inert (the proxy's
unpinned path over one account selects exactly that account), so offering it
adds options without adding meaning — the interactive-first "single candidate
auto-selects" principle applied in reverse. Options are valid by construction;
the dashboard additionally disables already-added members in the cascade.

Sub-combo members (a combo referencing another combo by name) are resolvable
by the engine but remain outside the pickers — unchanged from the base change.

### D8: Account pins ride inside the member candidate, not a new wizard step

The wizard stays five steps. The members step simply offers richer options:
the option string *is* the member string that gets written, so
`glm@work:glm-4.7` is self-describing at pick time and requires no follow-up
prompt. Alternatives considered:

- *A per-pick account sub-prompt when the chosen provider is
  multi-account*: an extra prompt per member, and it must also offer "any
  account" — recreating the flat list one level deeper with more keystrokes.
- *A dedicated "accounts" step before members*: wrong altitude — accounts
  are chosen per member, not per combo.

CLI pick lists keep pterm's filter for the larger candidate set; the members
step intro line gains one sentence explaining the `@account` suffix ("pick
`provider@account:model` to draw from a specific connection; `provider:model`
uses any connection").

The dashboard members step composes the member from **two dropdowns** —
"Model" (`provider:model`, optgrouped by provider, each model listed
exactly once) and "Connection". The Connection dropdown renders **disabled
with a "Select a model first" placeholder**; once a model is chosen it is
enabled and **scoped to that model's provider only**: Any connection plus
that provider's accounts (when it declares ≥2, sorted), **defaulting to the
first account** so the common case is an explicit pin. A provider with <2
accounts keeps the dropdown disabled on Any connection (the pin would be
semantically inert). Scoping happens client-side on model change: the
per-provider account map ships with the page as a `data-accounts` island
and a small inline sync rebuilds the option list with DOM methods and
`textContent` — never string-built `innerHTML`, since provider and account
names originate from user-edited YAML. The Add button posts both values;
the server composes the member — `model` + `any` → `provider:model`,
`model` + `work` → `provider@work:model` — validates the pairing (an
account from another provider is rejected with an inline error naming both:
a guard for crafted posts, since the scoped dropdown cannot produce the
pairing), rejects duplicates, then re-renders with both selects reset to
their defaults. Because a disabled `<select>` never submits, a
not-yet-chosen connection composes safely as Any on the server.

Implementation note that forced the island design: templ emits the contents
of `<script>` elements as literal text — `{ templ.Raw(...) }` inside a
`<script>` is NOT evaluated, so a `<script type="application/json">` island
renders the literal expression and `JSON.parse` silently degrades to `{}`
(this shipped broken in the first cut). A `data-` attribute is templ-escaped
in the HTML and entity-decoded by the DOM on read, which is what
`dataset.accounts` consumes.

Alternatives considered:

- *One optgroup per provider spec (`glm`, `glm@work`, …) — the first-cut
  implementation*: correct but duplicates every model once per account in
  one list; with a handful of accounts the picker becomes mostly repeats.
- *Two independent dropdowns, no scoping (the second cut)*: N+M options and
  no client sync, but the Connection list shows every provider's accounts
  at once, pairing errors surface only after Add, and the default pin
  belongs to an unrelated provider half the time.
- *Server-driven cascade (the model select posts a step transition)*:
  purest D4, but a full POST + re-render per model pick and draft state
  for the selected model.
- *Default Any connection*: zero mismatch friction, but buries the pin —
  the feature's whole point — behind an extra selection for every pinned
  member.

The scoped sync is a deliberate, narrow carve-out from D4's "no client-side
step state machine": it is a presentational option-list rebuild only — no
step state, validation, or writes move client-side, and every decision
still round-trips through the server. The mismatch branch stays
server-side as the correctness anchor. The CLI is unchanged (one filterable
list) — a terminal filter makes the flat form usable there, and splitting
the CLI flow into two prompts per member would add keystrokes without
removing confusion.

Parsing is unambiguous by construction: the router splits the provider spec
at the **first** `@` and account names may contain `@` (emails) but never `:`
or `,` (the account-name charset), so `glm@jane@example.com:glm-4.7` resolves
to provider `glm`, account `jane@example.com`, model `glm-4.7`.

### D9: `ValidateTopology` gains combo-member checks the spec already requires

Inside the existing combo loop, each member SHALL be rejected when it: has no
`:` (malformed), references an undeclared provider, or pins an account not in
that provider's `Accounts[]` (with `default` always allowed). Sub-combo
members (a name matching another combo) pass through — the engine resolves
and cycle-checks them at request time. The rules mirror the route-chain
checks line-for-line (`topology.go` `ValidateTopology`), so error copy stays
consistent ("combo %q references unknown account %q for provider %q").

This is an implementation catch-up, not a spec change: the `model-combos`
specification already states `ValidateTopology` SHALL reject such members.
It also makes the wizard's own guarantees enforceable for the typed shortcut
(`--members=glm@ghost:glm-4.7` fails at write instead of failing per request
later). The spec's remaining clause — a combo name colliding with a
provider-whitelist model on the same surface — stays out of scope; it is
unrelated to account pins and gets its own catch-up if wanted.

### D10: Account lifecycle keeps pins consistent

Two existing surfaces rewrite `Accounts[]` and must now also update members
that pin the affected account:

- **Rename** (dashboard `handleProviderAccountRename`): rewrite
  `provider@old:` → `provider@new:` across all combo members, in the same
  write as the `Accounts[].Name` rewrite. Mechanical and lossless.
- **Remove / disconnect** (CLI `providers account remove`, dashboard
  `handleProviderCredentialDelete`): downgrade `provider@account:model`
  members to unpinned `provider:model` — the user's provider+model choice
  survives and the hop keeps working through the remaining accounts (or the
  legacy key if it was the last). If the unpinned form is already a member of
  the same combo, drop the downgraded entry instead of duplicating it. No
  combo is ever removed: downgrade preserves provider and model and the
  dedup only drops exact duplicates, so every combo keeps at least one
  member (the earlier remove-below-two rule existed only to satisfy the
  two-member minimum, which this change dropped). The action's output names
  every combo touched ("downgraded pin in …") so nothing changes silently.

Alternative for remove — *block while pinned* — rejected: it couples account
lifecycle to combo editing across three surfaces, and a strict validator
(D9) would otherwise make the block permanent (an unfixable config that
fails every write). Downgrade is the one rule that leaves the topology
valid without guessing more than necessary.

## Risks / Trade-offs

- [Wizard POST flow loses user input on accidental dialog dismiss] → Draft
  values ride in the re-rendered form for Back; forward transitions
  re-submit collected fields. Mitigated scope: creating a combo is a
  <1-minute flow; accepted trade-off vs. session state.
- [Large model whitelists make the CLI pick list long] → pterm Select ships
  with filter enabled (`[type to search]`); no custom search UI.
- [Dialog re-open on step POST depends on correct `Open` propagation] →
  covered by handler tests asserting the dialog renders at the expected step
  with draft values intact.
- [Server-driven steps mean no instant client validation] → step POSTs
  validate server-side and re-render with `field.Error` content; acceptable
  at this scale, consistent with the rest of the dashboard.
- [Account-aware options multiply the candidate list (models × accounts)] →
  pinned options exist only for providers with ≥2 accounts, bounding growth;
  the CLI pick list keeps pterm's filter and the dashboard cascade keeps
  `<optgroup>` grouping (one group per provider / provider@account).
- [Stricter `ValidateTopology` rejects configs that previously saved] → only
  configs whose combo members already reference undeclared providers or
  unknown accounts (hand-edited YAML); route chains have failed validation
  under the same rules all along, and the error names the combo and account.
  Fix paths: reconnect the account, rename back, or re-pick the member.
- [A declared account with no stored credential can still be pinned] →
  options derive from topology `Accounts[]` only (single source of truth; no
  credential-store coupling in `internal/config`). A pin without a token
  behaves like any misconfigured hop: it fails at request time and ordered
  mode falls through to the next member.
- [Downgrade-on-remove edits combos the user did not open] → the downgrade
  preserves provider+model, keeps the topology valid under D9, and every
  touched combo is named in the action's output. Blocking the removal
  instead would deadlock: D9 would make the config unwritable until the user
  edited combos by hand.
- [templ generated files churn] → `templ generate` + `gofmt -w .` stay part
  of the task checklist as with every dashboard change.

## Migration Plan

Additive only: no config format, flag, or route changes. Existing typed
invocations keep working. Existing combos with unpinned members are
untouched. Three forward-only notes: combo members referencing undeclared
providers or unknown accounts now fail write-time validation (previously
accepted, then broken at request time); rename/remove of an account now
rewrite/downgrade pinned members in the same write; and the two-member
minimum is gone — single-member combos now write on every surface (the
remove-account path consequently no longer deletes combos that downgrade
to one member). Rollback is reverting
the commit; combos created through the wizard are ordinary config entries.
