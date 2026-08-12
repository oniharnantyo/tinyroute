## Context

tinyroute's OAuth login runs through two generic flows — `runPKCEFlow` and
`runDeviceCodeFlow` in `internal/cli/auth.go` — shared by all OAuth presets.
Both were ported from a reference router but the port is incomplete and has
decayed: preset data is wrong or missing for several providers, and a handful of
providers use flows the generic runners cannot express. The result is that
`auth login` fails for 8 of the OAuth providers, across four root-cause tracks
(see proposal). The runtime resolves presets from the embedded
`internal/preset/presets.json`; `catalog.go` is an unreferenced stale mirror.

## Goals / Non-Goals

**Goals:**
- Make `auth login` succeed for codex, iflow, antigravity, gemini-cli, cline,
  kimi, qoder, and gitlab.
- Express per-provider differences as preset *data* wherever the flow is standard
  (redirect shape, extra authorize params, client_type, device-header profile).
- Isolate genuinely non-standard flows (qoder, trae) behind dedicated runners so
  they do not pollute the generic PKCE/device paths.
- Keep unaffected providers (claude, github, xai/grok-cli, qwen) byte-for-byte
  unchanged.

**Non-Goals:**
- Long-term refresh-token longevity for qoder/trae beyond the initial login.
- A broader client_secret hardcoding policy (handled per-secret, per
  `security.md`).
- Re-evaluating whether gemini-cli should be PKCE vs device (its reference uses
  authorize); this change corrects its data within the current device flow.
- A client_id-acquisition UX beyond interactive prompting for user-registered
  apps.

## Decisions

**D1 — Generic preset fields, not provider switches.**
Add `callback_host`, `callback_path`, `extra_params`, and
`device_header_profile` to `Preset`. Rejected: a `switch p.Name` inside the
runners — leaks provider logic into shared code and forces edits per provider.
Data-driven fields fix codex (redirect+params), iflow (params), cline
(`client_type`), and antigravity/gemini-cli (data) with one mechanism.

**D2 — Emit `localhost` in the redirect_uri while binding `127.0.0.1`.**
The listener keeps binding `127.0.0.1` (localhost resolves to it); only the
string sent upstream must read `localhost` for codex, because Hydra compares the
registered URI as a string. `callback_host` controls that string without
changing where the socket listens.

**D3 — Encode authorize-query spaces as `%20`, not `+`.**
Go's `url.Values.Encode()` produces `+`; strict providers (OpenAI/Hydra) can
reject `+` in `scope`. After encoding, replace `+` with `%20` across the query.
Safe for the known param set (no authorize value contains a literal `+`;
`code_challenge` is base64 RawURL). Documented by comment + test.

**D4 — Edit `presets.json` as source of truth; mirror `catalog.go`.**
Runtime reads `presets.json`; `catalog.go` is unreferenced and already divergent
but is updated here to avoid a contradictory definition, and flagged for removal.

**D5 — Kimi device headers via a named profile + stable device_id.**
Kimi needs `X-Msh-Platform/Version/Device-Name/Device-Model/Device-Id`. The
`device_id` must be stable for the whole session (device-auth → poll → refresh),
so it is generated once, sent on every kimi request, and persisted with the
credential. `device_header_profile: "kimi"` on the preset selects a header
builder in the device runner (and refresh path). Rejected: free-form header
templates — they cannot express the generated device_id and OS/hostname logic.

**D6 — qoder and trae get dedicated flow types, not generic routing.**
qoder (poll `openapi.qoder.sh` for a `dt-` token after the user authorizes at
`qoder.com/device/selectAccounts`) and trae (login-guidance → `ExchangeToken`)
are not OAuth authorize/device flows. Routing them through `runPKCEFlow`/
`runDeviceCodeFlow` is the bug. New `flow_type` values `qoder` and `trae`
dispatch to `runQoderFlow`/`runTraeFlow`, mirroring the reference. This keeps the
generic runners clean.

**D7 — User-supplied client_id via interactive prompt (gitlab).**
GitLab Duo (and any preset with no `client_id`) cannot ship credentials. Per the
interactive-first CLI rule, `auth login` SHALL prompt for the user's OAuth app
`client_id` (and `client_secret` when the refresh profile needs it) when the
preset declares none and a TTY is attached; non-TTY yields a clear error. The
supplied values are stored with the credential.

## Risks / Trade-offs

- **Large heterogeneous change** → one change spans data, code, and a UX feature.
  Mitigation: artifacts and tasks are organized by track; each track is
  independently testable and committable.
- **Borrowed client_id/secret revocation** → codex/antigravity/gemini-cli reuse
  another tool's client. Mitigation: values live in `presets.json` data, so
  correction is a one-line edit; mirrors the existing claude/github risk.
- **`%20` global replace** → a future authorize param with a literal `+` would be
  mangled. Mitigation: documented assumption; covered by the encoding test.
- **Custom-flow drift** → qoder/trae upstream behavior may change. Mitigation:
  isolated runners make fixes local; reference is the parity source.
- **Persisted device_id / nonce** → new credential fields. Mitigation: optional
  fields; absent values degrade gracefully; no migration of existing credentials.

## Migration Plan

No persisted-format break. Existing credentials remain valid (new fields are
optional). Users re-run `tinyroute auth login <provider>` for the fixed
providers; gitlab users are prompted for their app credentials on first login.
Rollback is reverting the `presets.json`, `auth.go`, and `credential` edits — no
data migration.
