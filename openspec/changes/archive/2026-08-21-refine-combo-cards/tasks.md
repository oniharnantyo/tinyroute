# Tasks: refine-combo-cards

## 1. Config schema

- [x] 1.1 Add `Disabled bool` (`yaml:"disabled,omitempty"`) to `Combo` in
      `internal/config/topology.go`, with `IsEnabled()` helper
- [x] 1.2 Tests: absent flag parses as enabled; `disabled: true` round-trips
      through write/read; `IsEnabled()` polarity

## 2. Router semantics

- [x] 2.1 Direct resolution: disabled combo (both entry points in
      `internal/route/router.go` `Resolve`) returns
      `combo %q is disabled`; no fall-through
- [x] 2.2 Member enumeration: disabled sub-combo member is skipped like a
      failed member; all-disabled parent fails with the normal
      all-members-failed error
- [x] 2.3 Confirm `Models()` still lists disabled combo names
- [x] 2.4 Tests: direct disabled error; provider-position disabled error;
      parent-skip scenario; all-members-disabled failure

## 3. Switch component

- [x] 3.1 Vendor the shadcn-templ switch into
      `internal/dashboard/components/switch/` (templ source + generated go),
      matching the existing component layout and bundle wiring
- [x] 3.2 Component renders `role=switch` button with `aria-checked` and
      checked/unchecked variants; works as a form submit control

## 4. Dashboard handler

- [x] 4.1 Add POST `/dashboard/combos/toggle` to `internal/dashboard/combos.go`:
      load raw topology, flip the named combo's `Disabled`, persist via
      `config.WriteTopology`, redirect with flash; unknown name → clear error
- [x] 4.2 Tests: enable→disable and disable→enable round-trip persists the
      flag and only the flag; unknown combo name errors; CSRF/origin
      protection applies (inherits the mutating-action guard)

## 5. Card UI

- [x] 5.1 `internal/dashboard/view_combos.templ`: title `text-sm` → `text-base`
- [x] 5.2 Member display: render at most 3 chips as `provider:model`
      (strip `@account`); add `+N more…` overflow row with hover `title`
      listing hidden members
- [x] 5.3 Footer: toggle form (switch, left of Edit) POSTing to the toggle
      endpoint with hidden name field; reflect current state via
      `aria-checked`
- [x] 5.4 Disabled card styling: muted treatment (e.g. `opacity-60`) when
      the combo is disabled
- [x] 5.5 `ComboItem` carries `Enabled` from topology; wizard/edit
      round-trip unaffected (members still verbatim)
- [x] 5.6 Tests: truncation at 2/3/4/5 members; pin stripping incl.
      duplicate rendering; overflow count and title contents; disabled
      markup present when combo disabled

## 6. Verification

- [x] 6.1 `go build ./...`, `go test ./...`, `gofmt -w .`
- [x] 6.2 Manual: serve the gateway, toggle a combo from the dashboard,
      confirm the next request for that combo errors without restart
- [x] 6.3 `openspec validate --change refine-combo-cards` passes

