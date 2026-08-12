## Context

The repository is greenfield: only `go.mod` (`github.com/oniharnantyo/tinyroute`, go 1.26.4) exists.

The reference implementations, 9router and OmniRoute, are Node/TypeScript gateways in the
200k–300k-provider-entry range. Their routing logic is the smallest part of either codebase; the bulk
is a bundled provider catalog, a web/PWA/desktop dashboard, MCP/A2A servers, token-compression
layers, and OAuth flows that lift subscription credentials off disk. Every upstream vendor change
requires a release from them.

Constraints driving this design:

- Single static Go binary, **zero third-party dependencies**. Standard library only.
- Terminal-only interface. No web UI, no full-screen TUI.
- `.env` for deployment config; JSON for providers and routes; JSONL for session history.
- Primary client is **Claude Code**, which speaks only the Anthropic Messages API and reaches a
  gateway via `ANTHROPIC_BASE_URL` + `ANTHROPIC_AUTH_TOKEN`.
- Single-user, localhost-first. Multi-key issuance exists for attribution across a user's own tools,
  not as a multi-tenant product.

## Goals / Non-Goals

**Goals:**

- One stable endpoint so client configuration stops changing when providers change.
- Ordered fallback chains with cooldown-aware provider health.
- Adding a provider costs zero lines of Go.
- Adding a wire format (dialect) costs one package plus one registry line.
- Complete, replayable session history suitable for later lookup tooling.
- Flat LOC curve as provider count grows.

**Non-Goals:**

- Subscription/OAuth credential reuse (Claude Max, ChatGPT Plus). Anthropic's gateway documentation
  indicates that configuring a gateway replaces the personal login, so this is likely impossible by
  design and is certainly incompatible with a small codebase.
- Per-provider Go adapters, or `if provider == "x"` anywhere in the codebase.
- Bundled price tables, cost estimation, or dollar-denominated budgets.
- Web/desktop UI, MCP server, A2A, token compression, Bedrock/Vertex, embeddings, BYOK passthrough,
  ACME/TLS automation.
- OpenAI→Anthropic translation ("M2b"). Clients that speak OpenAI are already served by
  OpenAI-dialect providers.

## Decisions

### D1. A provider is data; a dialect is code

The single load-bearing decision. Providers are `config.json` entries (`base_url`, `dialect`,
`api_key`, `headers`). Dialects are Go packages implementing one interface.

```
  Provider = data   (base_url, dialect, api_key, headers)    → config.json
  Dialect  = code   (paths, parsing, auth shape, SSE grammar) → internal/dialect/
```

Eight providers today or forty later, there are still **two** dialect packages. Introducing a
`ProviderAdapter` interface is the exact mechanism by which the reference implementations grew;
it is prohibited.

*Alternative considered:* one adapter type per provider, as OmniRoute does. Rejected — it makes
provider count a code-size multiplier.

### D2. Parse as little as possible

Routing needs `model` and `stream`; nothing else is interpreted. Bodies are decoded into
`map[string]json.RawMessage`, the `model` field is swapped, and the map is re-marshalled.

Unknown fields survive untouched, so a new upstream parameter works without a tinyroute release.
Corollary principle: **do not model what you do not need to change.** The only deliberate mutations
are the `model` swap and the optional `stream_options.include_usage` injection (D9).

Buffering the request body is required anyway — a consumed stream cannot be replayed to the next
provider in a chain. Cap the buffer (32 MB) and reject beyond it.

### D3. Package layout with a one-directional import graph

Go's expression of clean architecture: inward-only imports, wiring in `main`. Not an
`entities/usecases/adapters` onion.

```
  cmd/tinyroute/main.go        flag dispatch + explicit wiring (~40 lines, no DI container)
  internal/core/               types + interfaces ONLY; imports nothing
  internal/config/             .env → Service (immutable); JSON → Topology (swappable)
  internal/dialect/            registry + anthropic/ + openai/
  internal/translate/          registry (empty until the follow-on change)
  internal/route/              glob match, chain resolution, health/cooldown
  internal/proxy/              attempt loop + SSE relay — the only orchestrator
  internal/history/            jsonl recorder, CAS blobstore, compact
  internal/auth/               keystore, token-bucket rate limiter
  internal/preset/             embedded presets.json, `add`

  everything → core.   core → nothing.
  dialect ⊥ translate ⊥ route ⊥ history ⊥ auth   (no sibling imports)
```

If a sibling ever needs a sibling, the shared type belongs in `core`.

### D4. Interfaces only where a second implementation is nameable

The test for every abstraction: *can you name the second implementation?*

| Seam | Second implementation | Justified |
|---|---|---|
| `Dialect` | gemini-native, openai-responses | yes |
| `Translator` | anthropic→openai | yes |
| `Recorder` | sqlite | yes |
| `BlobStore` | s3, none | yes |
| `Selector` | weighted, latency-ranked | yes |
| `CredentialStore` | macOS Keychain, systemd `LoadCredential` | yes |
| `Clock` | fake, for cooldown/expiry tests | yes |
| HTTP server | — | **no**, `net/http` forever |
| Config format | — | **no**, JSON is a requirement |
| Provider | — | **no**, see D1 |

`Dialect` owns everything wire-format-specific:

```
  Name()                    "anthropic"
  Paths()                   ["/v1/messages"]        — owns its inbound routes
  ParseRequest(body)        → model, stream, session hints
  RewriteModel(body, model) → body
  AuthHeaders(cred)         → x-api-key + anthropic-version | Authorization: Bearer
  NewUsageScanner()         → stateful, per request
  WriteError(w, failure)    → error envelope native to THIS client
```

`WriteError` is easy to omit and non-optional: when a chain exhausts, Claude Code must receive
Anthropic's error shape, not tinyroute's.

**Architectural regression test:** if adding a feature requires editing `internal/proxy`, the seam
was placed wrong.

### D5. Failover is only possible before the first response byte

A physical constraint, stated rather than hidden. Once an SSE frame reaches the client, the client
holds partial output and retrying elsewhere would corrupt it.

```
  try hop:
    conn error / timeout / 429 / 5xx  before first byte  → next hop
    404 (model absent at this provider)                  → next hop
    other 4xx (auth, malformed request)                   → return as-is
    stream started, then dies                             → propagate, log, DO NOT retry
```

`404` retries because free-tier model lineups (`zen`, `kilo`) change without notice and the next hop
may still have the model. Other 4xx do not: a malformed request will be malformed everywhere.

*Alternative considered:* buffer the whole response to preserve the failover option. Rejected —
it destroys streaming, which is unacceptable for an interactive coding agent.

Cooldown defaults: `429` honours `Retry-After` else 60s; `5xx`/connection 10s with escalating
strikes; `401`/`403` 15 minutes plus a loud CLI warning, since a chain cannot fix a bad key.

### D6. Configuration split by lifecycle, not by topic

```
  .env          deployment config, no secrets   → read once at startup; restart to change
  config.json   providers (incl. api_key) + routes → mtime-checked, atomic swap
  keys.json     inbound keys, sha256 digests      → mtime-checked, atomic swap
  state.json    cooldowns                         → daemon writes
```

Rule for ambiguous settings: **global → `.env`; per-provider or per-route → `config.json`, where it
overrides the global.** One precedence direction.

`${VAR}` interpolation applies uniformly to every string value in `config.json`, resolved at load
from the process environment. One mechanism rather than a special-cased `api_key_env` field, and it
covers `base_url` and `headers` too (Azure endpoints, OpenRouter attribution headers). Since `.env`
is startup-only, rotating a key requires a restart even though `config.json` hot-reloads —
predictable, and preferable to re-reading `.env` on every reload.

Provider keys live **inline** in `config.json`. Consequences accepted deliberately:

- `config.json` is secret-bearing: enforce `0600`, warn if looser, and have `init` detect `.git` and
  add the file to `.gitignore`.
- The shareable/committable-topology property is forfeited.
- **Strict JSON, no comments.** Forced rather than chosen: `encoding/json` rejects comments, a jsonc
  stripper would be hand-written code, and `add`/`auth set` machine-write the file so comments would
  not survive a rewrite. Machine writes use canonical marshalling (2-space indent, sorted keys) so
  diffs stay readable.
- Machine writes are atomic: temp file at `0600`, then `rename()`. Otherwise the mtime watcher can
  observe a half-written file and reject a config that is actually valid.

`CredentialStore` (D4) means moving to a keychain later is a change in `main.go` wiring, not a schema
migration.

### D7. One generic watcher, validate before swap

`config.json` and `keys.json` want identical machinery, so it is written once:

```
  config.Watcher[T]( path, parse func([]byte) (T, error) )
      Get() T     — atomic.Pointer read, lock-free on the hot path

  mtime changed → read → parse → VALIDATE → swap
                                    └─ invalid? keep serving the previous
                                       snapshot, log loudly, do not exit
```

A typo in `config.json` must never take down a running daemon. In-flight requests keep the snapshot
they resolved against. One ~60-line implementation, two uses — this makes total LOC go down.

### D8. No daemon IPC

`serve` writes files; every other command reads them.

```
  serve  ──writes──▶ state.json, requests.jsonl, blobs/
  status ──reads───▶ state.json + config.json
  log -f ──tails───▶ requests.jsonl
  keys   ──writes──▶ keys.json   (daemon picks it up via mtime, D7)
```

No admin endpoint means no IPC code, no admin auth surface, CLI works while the daemon is down, and
everything is debuggable with `cat`. Cost: no live in-flight-request view. Accepted.

This is only sound because `keys.json` is written **on mutation only**. Storing `last_used` there
would churn mtime on every request and defeat the reload cache — so `last_used` is derived by
scanning the tail of `requests.jsonl`, which is free.

### D9. History is a thin index plus content-addressed blobs

Bodies are never inlined into the JSONL. Lines are an index; blobs hold payloads. This makes
retention and deduplication policy decisions that can change later without a schema migration.

```
  requests.jsonl   {"v":1,"ts":…,"id":…,"key":"k_…","session":"a3f9c1",
                    "endpoint":"/v1/messages","model_req":"claude-sonnet-4-6","stream":true,
                    "attempts":[{provider,model,status,ms},…],
                    "usage":{in,out,cache_read},
                    "req_blob":"sha256:…","resp_blob":"sha256:…","outcome":"ok"}

  blobs/9c/1f….json.gz    gzipped, content-addressed
```

`key` and `session` must be present from the first line written; neither can be backfilled into
history already on disk.

**Sizing.** Coding agents resend the entire transcript every turn, so a 50-turn session is roughly
10 MB of request bodies raw, ~2.5 MB gzipped. At ten sessions a day that is ~9 GB/year.

**Cold-path compaction.** Turn *N*'s request is normally a superset of turn *N−1*'s, so
`tinyroute compact` can delete superseded request blobs — an O(n)→O(1) collapse per session, roughly
20× — after *verifying* the prefix-superset property. Context compaction and Task-tool sub-agents
break that property, hence verification rather than blind deletion. Deliberate split: the hot path
stays dumb and never deeply parses; all cleverness lives in an offline command where a bug cannot
break request serving.

**Session replay falls out.** Because each request carries the full transcript, replay needs the
newest request blob plus the ordered response blobs — a read, not a reconstruction.

**Session identity.** Coding agents do not send a session ID. Honour `X-Tinyroute-Session` when
present; otherwise derive `hash(system_prompt_prefix + messages[0] + day_bucket)`. Claude Code's
first user message is stable while the transcript grows, so this key is stable across a session. The
day bucket bounds collisions between sessions that open identically.

### D10. Usage capture: last chunk that carried usage wins

`UsageScanner` observes SSE lines while they are being piped — no buffering, no added latency.

Its contract is *"usage is whatever the most recent chunk carrying usage said."* That single rule is
correct for spec-compliant OpenAI (last chunk only), Anthropic's `message_delta`, and Gemini's
compatibility layer, which returns usage in **every** chunk in violation of the OpenAI spec.

This is the reference example of the anti-quirk-patch rule: **absorb provider deviations by
generalising an existing rule; never by adding a branch.** A quirk that cannot be absorbed
generically does not get a preset.

OpenAI-dialect streams omit usage unless the client sends `stream_options.include_usage`, which
Cursor and Cline generally do not. Inject it when absent, behind `TINYROUTE_INJECT_USAGE` (default
on) so it can be disabled if a strict client mishandles the trailing chunk with an empty `choices`
array. Without injection, usage is permanently `null` for those requests.

### D11. Inbound keys: hashed, scoped, no dollar budgets

Replaces a single static `auth_token`; one auth concept, not two.

- Format `tr_live_` + 32 bytes from `crypto/rand`. The fixed prefix lets secret scanners match it.
- Store `sha256` digests plus a 4-char prefix for display. Never store the key.
- `keys create` prints the plaintext once, together with the exact `export ANTHROPIC_BASE_URL` /
  `ANTHROPIC_AUTH_TOKEN` lines. `auth set` reads secrets from stdin, never argv — flags leak into
  shell history and `ps`.
- Per key: `allow` (surface + model globs), `rate` (in-memory token bucket), `expires`, `disabled`.
- **No dollar budgets.** They require per-model price tables — precisely the maintenance treadmill
  being avoided. The JSONL carries usage, so spend can be computed offline against a private price
  file.
- A tinyroute rate-limit `429` is an inbound rejection *before* routing and must not enter the
  failover path (D5).

### D12. Zero dependencies, and the details that get botched

- `.env` parsing hand-rolled (~40 lines): comments, `KEY=value`, optional quotes, tolerate a leading
  `export`. Multiline values and escapes are out of scope. Avoids godotenv.
- **The real environment wins over `.env`** (`if os.Getenv(k) == ""` — never override). This is what
  makes containers, systemd, and secret managers work; `.env` supplies local-dev defaults.
- Discovery: `--env-file`, else `./.env`, else `~/.tinyroute/.env`. First found wins; files are not
  merged.
- Do **not** set `http.Client.Timeout` — it is a whole-request deadline and will sever long streams
  mid-generation. Use `Transport.ResponseHeaderTimeout`, which doubles as the failover window, and
  leave body duration uncapped.
- TLS, if configured, is static cert/key via `ListenAndServeTLS` (5 stdlib lines). No ACME; a
  reverse proxy does that better.
- CLI via `flag` plus subcommand dispatch; tables via `text/tabwriter`.

### D13. Presets are scaffolding, never runtime

One embedded `presets.json` (~60 lines for eight providers), read **only** by `init` and `add`. The
daemon reads `config.json` exclusively.

| Preset | Dialect | Base URL |
|---|---|---|
| `anthropic` | anthropic | `https://api.anthropic.com` |
| `zai` | anthropic (+openai) | `https://api.z.ai/api/anthropic` |
| `minimax` | anthropic (+openai) | Anthropic-compatible endpoint |
| `kimi` | openai (+anthropic?) | `https://api.moonshot.ai/v1` |
| `openai` | openai | `https://api.openai.com/v1` |
| `gemini` | openai | `https://generativelanguage.googleapis.com/v1beta/openai` |
| `zen` | openai | `https://opencode.ai/zen/v1` |
| `kilo` | openai | `https://api.kilo.ai/api/gateway` |

Only the four anthropic-dialect entries are reachable from Claude Code in this change; the rest wait
on the translator. A stale preset costs the user one line of editing rather than a release, and
`tinyroute test --all` surfaces a wrong base URL in seconds.

### D14. Request flow — where every seam meets

```
  inbound
    ├─▶ auth.Lookup(bearer)             401 / rate 429 — never triggers failover
    ├─▶ dialect.ByPath(url)             which wire format am I speaking?
    ├─▶ d.ParseRequest(body)            model, stream
    ├─▶ route.Resolve(surface, model)   ordered chain
    │
    └─▶ for hop := range chain:
          health.Available(hop)?  no → skip
          hop.dialect != inbound? yes → translate.Request  (follow-on change)
          send with hop.Dialect.AuthHeaders
          ┌ first byte?  → COMMIT: relay + usageScanner.Observe
          └ failed?      → classify → health.Penalize → next hop

        exhausted → d.WriteError
        always    → recorder.Record + blobs.Put   (deferred, off the hot path)
```

### D15. Extensibility acceptance criteria

Not a sentiment — a table to check the implementation against.

| Add | Cost | Touches |
|---|---|---|
| Provider #9 | **0 lines of Go** | `config.json` (+6 lines preset, optional) |
| A dialect (gemini-native) | 1 package + 1 registry line | `dialect/` |
| A dialect pair | 1 package + 1 registry line | `translate/` |
| History in SQLite | 1 `Recorder` impl + 1 wiring line | `history/`, `main.go` |
| Weighted selection | 1 `Selector` impl + 1 wiring line | `route/`, `main.go` |
| Keychain credentials | 1 `CredentialStore` impl + 1 wiring line | `main.go` |
| A CLI verb | 1 file | `cmd/` |

**Prohibited patterns:** a `ProviderAdapter` interface; a `Server` interface; a DI container or
`wire`; `usecase` structs wrapping a single call; a `ConfigLoader` interface; and
`if provider == "x"` anywhere, ever.

## Risks / Trade-offs

- **The SSE translator is the project's whole risk** → excluded from this change. Anthropic's
  `content_block_*` indexing and partial-JSON tool-argument accumulation versus OpenAI's
  `choices[].delta` require a stateful transformer, not a mapper. Isolating it behind `Translator`
  means it arrives as one package with recorded-fixture table tests, and `proxy` never learns it
  exists beyond `translate.Lookup(from, to)`.

- **`blobs/` becomes a permanent plaintext archive of everything Claude Code ever read** — source,
  any `.env` contents, secrets, customer data → `0700`, never in a synced or backed-up directory by
  default, documented explicitly, and a retention ceiling before it grows unbounded. This is a
  larger real exposure than the provider keys.

- **`config.json` holds plaintext provider keys** → `0600` enforced on load, `.gitignore` written by
  `init`, and `CredentialStore` already in place so a keychain backend is a wiring change.

- **Mid-stream provider failure is unrecoverable** (D5) → propagate and log loudly rather than
  pretend. Applies to every gateway; most do not document it.

- **Free-tier model lineups shift** (`zen`, `kilo` are beta) → `404` is a failover-triggering error,
  and `test --all` verifies chains on demand.

- **Free endpoints may log or train on submissions** → routing proprietary source to free models is
  a governance decision, not a technical one. Surface it at `add` time rather than silently.

- **Session-ID heuristic will mis-group** — Task-tool sub-agents start with different transcripts and
  will fragment or absorb into the parent → ship it, then tune against real logged traffic. The
  `X-Tinyroute-Session` header is the deterministic escape hatch.

- **Buffering request bodies costs memory** under concurrency → 32 MB cap; acceptable for
  single-user localhost, would need revisiting if ever exposed.

- **Handing a key to another person changes the threat model entirely** → default bind stays
  `127.0.0.1`; TLS is static-cert only; exposure is documented as "put Caddy in front," and the
  consent implications of archiving someone else's transcripts are called out.

- **Presets can go stale** → accepted by design. Data, not code; one line to fix locally; `test --all`
  finds it. This is the trade that keeps the LOC curve flat.

## Migration Plan

Greenfield — no migration. Implementation order, each stage independently testable:

```
  core → config → dialect/anthropic → route → proxy → history → auth → preset → cmd
```

`internal/translate` stays an empty registry until the follow-on change; `proxy` is written against
`translate.Lookup` from the start so adding it requires no edits to `proxy` (D15).

Rollback is trivial: the binary is the only artifact, and every piece of state is a file under
`~/.tinyroute/`.

Follow-on change (**required** for the full eight-provider goal): `Translator` plus
`translate/anthropicopenai`, unlocking `openai`, `gemini`, `zen`, and `kilo` from Claude Code. Kept
separate so config, key, and history surfaces settle against real traffic first.

## Open Questions

1. **Kimi/Moonshot Anthropic-compatible path** is unconfirmed. Its OpenAI base URL is verified; the
   `/anthropic` variant is widely reported but was not verified during design. If absent, `kimi`
   moves to the follow-on change and this one ships three anthropic-dialect providers.
2. **Does `compact` run on a timer inside `serve`, or stay manual?** Manual is simpler and safer, but
   it will be forgotten and `blobs/` will grow.
3. **Blob retention rule.** `max_size_mb` on the JSONL is straightforward; blobs need their own
   budget. Candidate: delete blobs older than N days except the newest request per session.
4. **Passthrough auth for subscriptions** (`"api_key": "$passthrough"`, forwarding the client's own
   `Authorization` header) is a ~5-line spike. Anthropic's gateway documentation implies gateway mode
   replaces the personal login, so expect it to fail; worth ten minutes to confirm, not worth
   planning around.
5. **Session-heuristic tuning** cannot be settled before real traffic exists. Revisit once the first
   week of `requests.jsonl` is available.
