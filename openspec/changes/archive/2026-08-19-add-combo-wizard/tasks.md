# Tasks: add-combo-wizard

## 1. CLI wizard core

- [x] 1.1 Extract a shared add-core in `internal/cli/combos.go`: validation
      (name charset/uniqueness, members, mode, capabilities) + topology
      read/mutate/write, callable from both the flag path and the wizard path
- [x] 1.2 Add member-option source: derive `provider:model` candidates from
      parsed topology provider whitelists (skip providers with no whitelisted
      models); expose as a helper with a unit test
- [x] 1.3 Implement step 1 (name) with re-prompting validation: charset
      `^[a-zA-Z0-9_.\-]+$`, no `:`, uniqueness against existing combos
- [x] 1.4 Implement step 2 (members): sequential interactive Select prompts
      phrased by position (FIRST/SECOND/…), excluding already-chosen models,
      with a completion option once ≥2 members are chosen (filter enabled)
- [x] 1.5 Implement step 3 (mode) as an interactive Select with descriptions
      (ordered/pool/fused, ordered default)
- [x] 1.6 Implement step 4 (capabilities) as an optional interactive
      MultiSelect over vision/pdf/audio/video (empty selection allowed)
- [x] 1.7 Implement step 5 (review/confirm): numbered member list, summary of
      all fields, interactive Confirm; on accept write via the add-core and
      print the usage hint (`clients request model: <name>`)
- [x] 1.8 Wire dispatch in `cmdCombosAdd`: full flags → flag path (no
      prompts); missing values + TTY → wizard; missing values + no TTY →
      error naming required values with an example invocation; no
      providers/whitelisted models → informational exit pointing at
      `tinyroute provider add`

## 2. CLI tests

- [x] 2.1 Unit-test the add-core: valid combo written to topology, duplicate
      name rejected, invalid mode rejected, unknown member provider rejected
      (via `ValidateTopology` assertions)
- [x] 2.2 Unit-test the member-option helper: whitelisted models offered as
      `provider:model`, providers without whitelists skipped, empty topology
      yields empty list
- [x] 2.3 Wizard-flow tests with fakes per existing `interactive` test
      patterns: pick sequence defines member order; "Done" unavailable before
      2 members; duplicate-name re-prompt; typed-args path prompts nothing
- [x] 2.4 Edge-state tests: non-TTY missing args error copy; no-providers
      informational exit

## 3. Dashboard combos list

- [x] 3.1 Add the Combos nav entry in `view_layout.templ` between Routes and
      History using `icon.Layers`; wire the route/handler registration
- [x] 3.2 Create `view_combos.templ`: page header with New Combo button
      (dialog trigger), one `card` per combo (name, `badge` mode, numbered
      member chips, edit trigger via `dialog.TriggerFor`, delete via a
      confirmation `dialog` with destructive styling), `empty` state with
      `icon.Layers` when no combos exist
- [x] 3.3 Handler: read topology and render the list (combos + resolved
      member display), reusing existing auth/nav patterns

## 4. Dashboard wizard dialog

- [x] 4.1 Implement the wizard dialog shell (`dialog` component) with the
      five-step indicator (composed primitives + `separator`) hosted on the
      combos page
- [x] 4.2 Implement `POST` step endpoints carrying `(step, draft)`: validate
      the submitted step server-side, re-render the page with the dialog
      `Open: true` at the next step and draft values intact; Back returns one
      step; Cancel/✕ performs no write
- [x] 4.3 Step 1 name form: `label` + `input` with server-side validation
      error re-render (charset, no `:`, uniqueness; kit has no field
      component)
- [x] 4.4 Step 2 members form: provider-grouped native `<select>` (optgroup)
      cascade with already-added models disabled, Add button appending to the
      numbered list, per-row ghost icon `button`s
      (`icon.ChevronUp/ChevronDown/X`) for reorder/remove posting list ops,
      minimum-2 enforcement to continue
- [x] 4.5 Step 3 mode form: `radio` group with the three modes and
      descriptions; step 4 capabilities: native checkbox grid (no checkbox
      component in kit; optional)
- [x] 4.6 Step 5 review + Create: final `button` submits through the same
      add-core (topology read/validate/write); on success re-render the list
      with an SSR success `toast` naming the combo (no secrets in content)
- [x] 4.7 Edit flow: per-card Edit (`dialog.TriggerFor`) opens the same
      wizard pre-filled with current values at step 1; final action saves
      changes through the shared core with a success `toast`

## 5. Dashboard delete

- [x] 5.1 Wire the delete action behind a confirmation `dialog` (destructive
      styling) naming the combo; confirm removes it via the config write
      path; cancel is a no-op; success `toast` on the re-rendered list

## 6. Dashboard tests

- [x] 6.1 Handler tests: list renders combos with numbered members and mode
      badges; empty state renders when no combos exist; nav entry present
- [x] 6.2 Wizard POST tests: step advancement preserves draft values; invalid
      name re-renders step 1 with error; <2 members cannot advance; create
      writes topology and renders success toast content; cancel writes
      nothing
- [x] 6.3 Delete tests: confirm removes the combo from topology; dismiss
      keeps it; toast copy names the combo
- [x] 6.4 Edit tests: pre-filled wizard renders current values; save updates
      topology (order preserved)

## 7. Verification

- [x] 7.1 `templ generate` + `gofmt -w .` clean; `go build ./...` passes
- [x] 7.2 `go test ./...` green, including new CLI and dashboard suites;
      coverage on new combo functions ≥ 80%
- [x] 7.3 Manual pass: `tinyroute combos add` wizard in a TTY creates a
      working priority combo (request it via the gateway, observe ordered
      fallback); dashboard create/edit/delete round-trips the same combo;
      typed-args shortcut still works

## 8. Account-aware member selection (multi-connection providers)

- [x] 8.1 Extend `config.GetMemberCandidates` to emit account-pinned
      candidates: for providers declaring ≥2 accounts, every whitelisted
      model is offered unpinned (`provider:model`) and once per declared
      account (`provider@account:model`); providers with 0–1 account emit
      unpinned only; deterministic sort; unit tests for all three shapes
      (including an email-style account name surviving verbatim)
- [x] 8.2 Add combo-member checks to `config.ValidateTopology`, mirroring
      the route-chain rules: reject malformed members (no `:`), undeclared
      providers, and pins of accounts the provider does not declare
      (`default` always allowed; names matching another combo pass through);
      unit tests including error copy naming combo/provider/account
- [x] 8.3 CLI wizard: add one intro line to the members step explaining the
      `@account` suffix (pinned = that connection, unpinned = any
      connection via the provider's selection strategy); confirm exclusion
      already keys on the full member string; tests: pinned candidate
      offered and stored verbatim, pinned and unpinned forms of the same
      model separately selectable, typed `--members=glm@work:...` accepted,
      typed unknown-account member rejected at write
- [x] 8.4 Dashboard members step: confirm `groupCandidatesByProvider`
      renders one `<optgroup>` per provider spec (`glm`, `glm@work`, …) with
      the full member string as option value and already-added members
      disabled on full-string comparison; add the same one-line hint;
      handler tests: account groups render for a multi-account provider,
      pinned member added verbatim, pinned/unpinned duplicate rule holds
- [x] 8.5 Edit round-trip: dashboard edit flow preserves pinned members
      verbatim (pre-fill and save); extend the existing edit test with a
      pinned member
- [x] 8.6 Account rename keeps pins: `handleProviderAccountRename` rewrites
      `provider@old:` → `provider@new:` across all combo members in the same
      write as the `Accounts[]` rename; test the rewrite and that untouched
      combos are unchanged
- [x] 8.7 Account removal downgrades pins: shared helper (in
      `internal/config`) mapping `provider@account:model` → `provider:model`
      with dedup-against-existing and remove-combo-below-2 rules, used by
      CLI `providers account remove` and dashboard
      `handleProviderCredentialDelete`; both outputs name every combo
      modified or removed; tests for the helper and both call sites
      (including the both-members-pinned → combo-removed case)
- [x] 8.8 Verification: `templ generate` + `gofmt -w .` clean; `go build
      ./...` and `go test ./...` green with ≥80% coverage on new functions;
      manual pass — build a combo with a pinned member via the wizard,
      request it through the gateway and confirm the pinned account served
      the hop (history), then rename and disconnect that account and confirm
      the combo stays valid

## 9. Dashboard members step: model + connection dropdowns

- [x] 9.1 Add `config.GetModelCandidates(topo)` (unpinned `provider:model`
      only, each model once per provider) and `config.GetAccountOptions(topo)`
      (`provider@account` for providers declaring ≥2 accounts, sorted);
      recompose `GetMemberCandidates` from both so the CLI source is
      unchanged; unit tests for all three
- [x] 9.2 Rework the members-step form in `view_combos.templ`: Model
      `<select>` (optgroup per provider, value `provider:model`) +
      Connection `<select>` (Any connection option + one optgroup per
      multi-account provider, value `provider@account`, defaults to the
      first account option — Any when none exist); replace
      `groupCandidatesByProvider` with model/account grouping helpers;
      update the hint line to describe composing the two
- [x] 9.3 Handler `add_member`: read `selected_model` + `selected_account`;
      compose (`any`/empty → `provider:model`, account →
      `provider@account:model`); reject with inline step-2 error when the
      account's provider ≠ the model's provider (error names both) or the
      composed member is already in the list; on success re-render with both
      dropdowns reset to defaults
- [x] 9.4 Update dashboard tests: model dropdown lists each model once per
      provider (no `@account` options), connection dropdown groups +
      first-account default + Any-only default when no multi-account
      providers, pinned composition stored verbatim, unpinned composition
      via Any, mismatch error, duplicate composed member error, reset after
      add; migrate the optgroup-per-provider-spec assertions (groups
      `glm`/`glm@work`/`glm@personal`) to the new rendering
- [x] 9.5 Verification: `templ generate` + `gofmt -w .`; `go build ./...`
      and `go test ./...` green; manual pass — add a pinned member via
      Model+Connection, add an unpinned member via Any, confirm no model
      duplication in the picker

## 10. Wizard flow refinement: gated Connection dropdown, one-member minimum, neutral copy

- [x] 10.1 Rework the members-step Connection dropdown in `view_combos.templ`:
      render it visible but disabled with a "Select a model first" placeholder;
      on model selection (inline sync) enable it scoped to that model's
      provider only — Any connection plus that provider's accounts when it
      declares ≥2 (first account preselected); keep it disabled on Any for
      providers with <2 accounts
- [x] 10.2 Fix the account data island: templ emits `<script>` content
      literally, so the per-provider account map moves to a `data-accounts`
      attribute read via `dataset` (options rebuilt with DOM methods +
      `textContent`, never string-built innerHTML, since provider/account
      names originate from user-edited YAML)
- [x] 10.3 Drop the two-member minimum everywhere: dashboard `next_step_2`
      and `submit_create`, CLI `AddComboCore`, and the CLI wizard Done
      gating now require at least one member (combos serve ordered, pool,
      and fused modes)
- [x] 10.4 Simplify `config.DowngradeComboAccount` to two returns (updated
      combos + modified names): the remove-below-two branch is dead under
      the one-member minimum (downgrade never empties a combo); update
      `providers account remove` and the dashboard credential-delete call
      sites and copy ("adjusted", no "removed combos")
- [x] 10.5 Sweep fallback terminology from wizard copy on both surfaces
      (page subtitles, card labels, step copy, hints, prompts, review
      labels, duplicate-member error) so the wizard reads mode-neutral
- [x] 10.6 Update tests: one-member advancement + empty-list rejection on
      both surfaces, disabled dropdown + data-island assertions, Done
      availability after the first pick, downgrade-survival expectations on
      all three call sites
- [x] 10.7 Verification: `templ generate` + `gofmt -w .`; `go build ./...`
      and `go test ./...` green; manual pass — pick a model, confirm the
      connection list shows only that provider's accounts defaulting to the
      first, create a single-member combo end-to-end
