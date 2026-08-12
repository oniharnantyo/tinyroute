Implementation order follows design.md's migration plan:
`core → config → dialect/anthropic → route → proxy → history → auth → preset → cmd`

Hard constraints for every task: standard library only, no third-party imports, and no
`if provider == "x"` branching anywhere.

## 1. Core types and seams

- [ ] 1.1 Create `internal/core` with request/response value types: parsed request (model, stream,
      session fingerprint inputs), attempt, usage, failure classification, and outcome
- [ ] 1.2 Define the `Dialect` interface: `Name`, `Paths`, `ParseRequest`, `RewriteModel`,
      `AuthHeaders`, `NewUsageScanner`, `WriteError`
- [ ] 1.3 Define `UsageScanner` with an `Observe`/`Usage` contract documented as "last chunk carrying
      usage wins"
- [ ] 1.4 Define remaining ports: `Translator`, `Recorder`, `BlobStore`, `Selector`,
      `CredentialStore`, `Clock`
- [ ] 1.5 Add a package doc comment asserting `core` imports nothing outside the standard library, and
      verify with `go list -deps`

## 2. Service configuration from .env

- [ ] 2.1 Hand-roll the dotenv parser: `KEY=value`, `#` comments, blank lines, optional quotes,
      tolerated `export` prefix
- [ ] 2.2 Implement discovery order `--env-file` → `./.env` → `~/.tinyroute/.env`, first match only,
      no merging, missing file not an error
- [ ] 2.3 Apply values without overriding variables already present in the process environment
- [ ] 2.4 Parse the recognized settings table into an immutable `Service` struct with defaults; fail
      startup on malformed durations; warn and ignore unknown `TINYROUTE_` variables
- [ ] 2.5 Validate that TLS cert and key are supplied together or not at all
- [ ] 2.6 Tests: precedence, discovery order, defaults, quoted values, malformed duration rejection

## 3. Topology configuration and hot reload

- [ ] 3.1 Define the `config.json` schema types for providers and routes; strict JSON decoding
- [ ] 3.2 Implement uniform `${VAR}` interpolation across all string values, distinguishing unset
      variables from empty values
- [ ] 3.3 Implement `config.Watcher[T]`: mtime check, parse, validate, atomic pointer swap, retain
      previous snapshot on error, log loudly, never exit
- [ ] 3.4 Implement atomic writes: temp file at `0600` in the same directory, canonical marshal with
      two-space indent and sorted keys, then rename
- [ ] 3.5 Enforce and check `0600` on load; warn when group- or world-readable
- [ ] 3.6 Implement the validation rule set: unknown dialect, undeclared provider in a chain, unset
      interpolation variable, malformed glob, and surface/chain dialect mismatch with a message naming
      the route index and the missing translation pair
- [ ] 3.7 Tests: reload takes effect, malformed edit keeps serving the previous snapshot, half-written
      file is never observed, each validation rule fires with the expected message

## 4. Anthropic dialect

- [ ] 4.1 Implement `Paths` as `/v1/messages` and register the dialect
- [ ] 4.2 Implement `ParseRequest` reading only `model` and `stream`, plus system-prompt and first-message
      fingerprint inputs for session derivation
- [ ] 4.3 Implement `RewriteModel` via `map[string]json.RawMessage` swap, preserving unknown fields
      byte-for-byte
- [ ] 4.4 Implement `AuthHeaders` with API-key and version headers, honoring provider `headers`
      overrides including removal via null
- [ ] 4.5 Implement `NewUsageScanner` reading usage from `message_delta` events, last-wins
- [ ] 4.6 Implement `WriteError` emitting the Anthropic error envelope
- [ ] 4.7 Tests: unknown-field preservation round-trip, model rewrite, usage extraction from a recorded
      SSE fixture, error envelope shape

## 5. OpenAI dialect

- [ ] 5.1 Implement `Paths` as `/v1/chat/completions` and register the dialect
- [ ] 5.2 Implement `ParseRequest`, `RewriteModel`, and bearer `AuthHeaders` with header overrides
- [ ] 5.3 Implement `NewUsageScanner` with last-chunk-wins semantics, verified against both a
      final-chunk-only fixture and an every-chunk fixture, with no provider branching
- [ ] 5.4 Implement `stream_options.include_usage` injection when absent and streaming, gated on the
      deployment setting, leaving a client-supplied value untouched
- [ ] 5.5 Implement `WriteError` emitting the OpenAI error envelope
- [ ] 5.6 Implement `GET /v1/models` synthesis from route patterns and chain hops
- [ ] 5.7 Tests: injection behavior in all three states, usage extraction for both fixture styles,
      synthesized model list reflects configuration

## 6. Routing and provider health

- [ ] 6.1 Implement glob matching and route resolution scoped by inbound surface, first match wins
- [ ] 6.2 Implement chain resolution including the `$model` passthrough token
- [ ] 6.3 Implement the health store: cooldown windows keyed by provider, `Clock`-driven, escalating
      strikes for connection and `5xx` failures
- [ ] 6.4 Persist and restore cooldowns via `state.json`, treating elapsed windows as absent
- [ ] 6.5 Implement failure classification per the design table, including `404` retrying without
      cooldown and `401`/`403` cooling down without retrying
- [ ] 6.6 Implement `Selector` with ordered-first selection as the only implementation
- [ ] 6.7 Tests: route precedence, surface disambiguation with identical globs, each classification row,
      cooldown persistence across restart, expired cooldown ignored

## 7. Proxy attempt loop

- [ ] 7.1 Configure the outbound transport with `ResponseHeaderTimeout` and no whole-request deadline
- [ ] 7.2 Buffer the request body with a 32 MB cap, rejecting oversized bodies in the inbound dialect's
      native error format
- [ ] 7.3 Implement the attempt loop: skip providers in cooldown, call `translate.Lookup` for
      cross-dialect hops, send with the hop dialect's auth headers
- [ ] 7.4 Implement commit semantics: mark committed on the first relayed byte and refuse any further
      hop after that point
- [ ] 7.5 Implement the SSE relay with per-chunk flushing and `UsageScanner.Observe`, without buffering
- [ ] 7.6 Propagate mid-stream failures to the client and mark the outcome accordingly
- [ ] 7.7 Emit `WriteError` on chain exhaustion and on no-matching-route
- [ ] 7.8 Ensure the inbound caller's credential is never forwarded upstream
- [ ] 7.9 Tests with a fake provider server: pre-commit failover, post-commit propagation without
      retry, cooldown skip without a network call, chunk-by-chunk flush timing, no credential leak

## 8. History recording

- [ ] 8.1 Implement the JSONL `Recorder` writing one versioned record per request with key, session,
      endpoint, attempts, usage, blob references, and outcome
- [ ] 8.2 Record off the critical path so the client response is never delayed
- [ ] 8.3 Implement the content-addressed gzip `BlobStore` with digest-derived paths and single-blob
      deduplication for identical content
- [ ] 8.4 Honor capture mode: full stores bodies, metadata stores records only
- [ ] 8.5 Implement session identity: honor the session header, otherwise derive from system-prompt and
      first-message fingerprint plus date bucket
- [ ] 8.6 Create the blob directory owner-only and blob files owner-readable only; warn at startup when
      full capture is enabled
- [ ] 8.7 Implement history file rotation at the configured ceiling
- [ ] 8.8 Tests: one record per request, no body inlined, identical bodies share a blob, session grouping
      across growing transcripts, distinct conversations separate, permission modes

## 9. Inbound API keys

- [ ] 9.1 Implement key generation: `tr_live_` prefix plus 32 bytes from `crypto/rand`
- [ ] 9.2 Implement `keys.json` storage of identifier, name, prefix fragment, `sha256` digest,
      timestamps, and scoping fields, with no plaintext and no per-request fields
- [ ] 9.3 Load keys through `config.Watcher[T]` for mtime-based reload with validate-before-swap
- [ ] 9.4 Implement verification before route resolution, rejecting absent, unknown, disabled, and
      expired keys in the inbound dialect's native error format
- [ ] 9.5 Implement allow-list enforcement against `surface:model-glob` patterns, permitting all
      configured routes when absent
- [ ] 9.6 Implement the in-memory token-bucket rate limiter, ensuring rejections never trigger failover
      or record a provider cooldown
- [ ] 9.7 Derive last-use information from history rather than storing it in `keys.json`
- [ ] 9.8 Reject requests when no keys exist, directing the operator to create one
- [ ] 9.9 Tests: verification paths, scope enforcement, revocation effective on next request, expiry
      without a file change, key file mtime unchanged while serving, rate-limit isolation from failover

## 10. Presets and provider scaffolding

- [ ] 10.1 Embed `presets.json` with the eight providers, their dialects, base URLs, and conventional
      credential variable names
- [ ] 10.2 Implement `add` writing a plain provider entry, `--list`, and `--dialect` selection for
      dual-protocol providers
- [ ] 10.3 Implement `auth set` reading from stdin only, and `auth list` with masked values and unset
      markers
- [ ] 10.4 Verify by inspection and test that no preset is reachable from the request path
- [ ] 10.5 Confirm the Kimi Anthropic-compatible endpoint (open question 1); if absent, ship `kimi` as
      an `openai`-dialect preset only and note it is unreachable from Claude Code until the follow-on
      change
- [ ] 10.6 Tests: added entry is usable without code changes, unknown provider in a chain fails
      validation, preset-only provider is not routable

## 11. CLI surface

- [ ] 11.1 Implement subcommand dispatch with `flag`, and table output with `text/tabwriter`
- [ ] 11.2 Implement `init`: scaffold `.env` and `config.json` at `0600`, detect `.git` and write
      `.gitignore` before creating the config, mint the first key, print the client export lines
- [ ] 11.3 Implement `serve` with static TLS when both cert and key are configured
- [ ] 11.4 Implement `validate` reporting every error with route index or provider name, non-zero exit
      on any error, and a count summary when clean
- [ ] 11.5 Implement `test` and `test --all`, distinguishing unreachable host, rejected credential, and
      unknown model per hop
- [ ] 11.6 Implement `status` reading `state.json` and `config.json` only, surfacing active cooldowns
      and authentication warnings
- [ ] 11.7 Implement `log` with follow, failures-only, time window, and session or key filters, reading
      the history file directly
- [ ] 11.8 Implement `sessions` with rollups and key filtering, and `session <id>` replay from blobs,
      reporting clearly when bodies were not captured
- [ ] 11.9 Implement `keys create|list|revoke`, printing plaintext once with client export lines
- [ ] 11.10 Implement `compact` with leading-subsequence verification before deleting a superseded
      request blob, never deleting response blobs, and never running inside the request path
- [ ] 11.11 Verify every read-only command works with the daemon stopped and performs no IPC

## 12. Architecture conformance and acceptance

- [ ] 12.1 Assert the import graph: `core` imports nothing; `dialect`, `translate`, `route`, `history`,
      and `auth` import no siblings; only `proxy` orchestrates and only `main` wires
- [ ] 12.2 Confirm zero third-party dependencies via `go mod graph` and an empty `require` block
- [ ] 12.3 Grep-based check that no `if provider ==` or equivalent provider branching exists
- [ ] 12.4 Confirm no prohibited abstractions: no `ProviderAdapter`, no `Server` interface, no DI
      container, no single-call `usecase` wrapper, no `ConfigLoader` interface
- [ ] 12.5 Walk the design's extensibility table: adding a provider touches only `config.json`; adding a
      dialect touches only a new package plus one registry line; neither edits `proxy`
- [ ] 12.6 Add a fake `Clock`, an in-memory `Recorder`, and a fake `Dialect` so cooldown, expiry, and
      attempt-loop tests need no network or wall-clock waits
- [ ] 12.7 End-to-end check with Claude Code pointed at the daemon: confirm streaming works, chain
      failover works when the first provider is made to fail, and history records the session

## 13. Follow-on change preparation

- [ ] 13.1 Leave `internal/translate` as a registry returning "no translator" so `proxy` needs no edit
      when the translator lands
- [ ] 13.2 Capture recorded SSE fixtures from both dialects during end-to-end testing, for use as
      translator test inputs
- [ ] 13.3 Record decisions on open questions 2 and 3 (compaction trigger, blob retention rule) once a
      week of real history exists
- [ ] 13.4 Open the follow-on change for `Translator` plus `translate/anthropicopenai`, unlocking
      `openai`, `gemini`, `zen`, and `kilo` from Claude Code
