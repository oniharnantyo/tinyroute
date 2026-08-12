## ADDED Requirements

### Requirement: Every request appends exactly one index record to a JSONL file

The service SHALL append one newline-delimited JSON record per proxied request to the history file.
Each record SHALL carry a schema version, a timestamp, a request identifier, the authorizing key
identifier, the session identifier, the inbound endpoint, the requested model, whether the response was
streamed, the ordered list of attempts with provider, model, status and elapsed milliseconds, token
usage, references to the stored request and response bodies, and a final outcome. Recording MUST occur
outside the request's critical path and MUST NOT delay the client's response.

#### Scenario: A served request produces one record

- **WHEN** a request is proxied successfully
- **THEN** exactly one record MUST be appended to the history file

#### Scenario: Attempts are recorded in order with statuses

- **WHEN** a request fails against the first hop and succeeds on the second
- **THEN** the record's attempt list MUST contain both hops in order with their statuses and durations

#### Scenario: Recording does not delay the response

- **WHEN** a response is relayed to the client
- **THEN** history recording MUST NOT block completion of that response

#### Scenario: Mid-stream failure is recorded distinctly

- **WHEN** a response fails after streaming has begun
- **THEN** the record's outcome MUST identify it as a mid-stream failure rather than a success

### Requirement: Record fields required for later lookup are present from the first record

The schema version, session identifier, and key identifier SHALL be written on every record from the
first record onward. Because history is append-only and cannot be retroactively corrected, these fields
MUST NOT be deferred to a later change.

#### Scenario: Version is present on every record

- **WHEN** any record is written
- **THEN** it MUST include a schema version field

#### Scenario: Session and key identifiers are always present

- **WHEN** any record is written
- **THEN** it MUST include both a session identifier and a key identifier

### Requirement: Request and response bodies are stored as content-addressed blobs, never inlined

Bodies SHALL be stored as gzip-compressed blobs addressed by the digest of their content, and records
SHALL reference them by digest. A body MUST NOT be embedded in a JSONL record. Identical content
SHALL resolve to a single stored blob. This separation SHALL allow retention and deduplication policy
to change without altering the record schema.

#### Scenario: Record references rather than contains the body

- **WHEN** a request with a large body is recorded
- **THEN** the record MUST contain a blob reference
- **AND** MUST NOT contain the body content

#### Scenario: Blobs are compressed and content-addressed

- **WHEN** a body is stored
- **THEN** it MUST be written compressed under a path derived from the digest of its content

#### Scenario: Identical bodies share one blob

- **WHEN** two requests carry byte-identical bodies
- **THEN** both records MUST reference the same blob
- **AND** only one blob MUST exist on disk

#### Scenario: Capture mode governs body storage

- **WHEN** capture mode is set to metadata rather than full
- **THEN** records MUST still be written with attempts and usage
- **AND** body blobs MUST NOT be stored

### Requirement: Token usage is captured from streaming responses without buffering

For streamed responses, usage SHALL be observed while chunks are relayed, without buffering the
response. The recorded usage SHALL be taken from the most recent chunk that carried usage information.
This rule SHALL apply uniformly across providers, including providers that emit usage only in a final
chunk and providers that emit usage in every chunk. Provider-specific branching MUST NOT be introduced
to accommodate such differences.

#### Scenario: Usage from a final-chunk provider

- **WHEN** a provider emits usage only in the last chunk of a stream
- **THEN** the recorded usage MUST be the values from that chunk

#### Scenario: Usage from a provider that repeats it in every chunk

- **WHEN** a provider emits usage in every chunk of a stream
- **THEN** the recorded usage MUST be the values from the last chunk carrying usage
- **AND** no provider-specific code path MUST be required

#### Scenario: Observation does not add latency

- **WHEN** usage is observed during a streamed relay
- **THEN** chunks MUST continue to be flushed to the client as they arrive

### Requirement: Usage reporting is requested from OpenAI-dialect providers when the client omits it

When the inbound request targets an OpenAI-dialect provider with streaming enabled and the client has
not requested usage reporting, the service SHALL add the option that causes usage to be emitted. This
behavior SHALL be controlled by a deployment setting that is enabled by default and can be disabled.
When disabled and the client has not requested usage, recorded usage SHALL be absent rather than
estimated. Token counts MUST NOT be computed locally.

#### Scenario: Option is injected when absent

- **WHEN** a streaming request to an OpenAI-dialect provider omits the usage option and injection is
  enabled
- **THEN** the forwarded request MUST include the option
- **AND** the record MUST contain usage

#### Scenario: Client's own setting is respected

- **WHEN** the client already requests usage reporting
- **THEN** the request MUST be forwarded without modification of that option

#### Scenario: Injection disabled yields absent usage

- **WHEN** injection is disabled and the client omits the usage option
- **THEN** the record's usage MUST be absent
- **AND** no locally computed token estimate MUST be recorded

### Requirement: Requests are grouped into sessions by an explicit header or a derived identifier

The service SHALL use a client-supplied session header when present. Otherwise it SHALL derive a
session identifier from a stable fingerprint of the conversation opening combined with a date bucket,
so that successive turns of one agent conversation share an identifier as the transcript grows. The
derivation MUST NOT depend on the client transmitting a session identifier.

#### Scenario: Explicit header is authoritative

- **WHEN** a request carries the session header
- **THEN** that value MUST be used as the session identifier

#### Scenario: Successive turns group together

- **WHEN** an agent sends successive requests that share the same system prompt and first user message
  while later turns are appended
- **THEN** all those requests MUST receive the same derived session identifier

#### Scenario: Distinct conversations separate

- **WHEN** two conversations begin with different first user messages
- **THEN** their requests MUST receive different derived session identifiers

### Requirement: Sessions can be listed and replayed from stored history

`tinyroute sessions` SHALL list recorded sessions with turn counts, models and providers used, and
token totals, and SHALL support filtering by key. `tinyroute session <id>` SHALL reconstruct a
conversation from stored blobs. Reconstruction SHALL rely on the fact that an agent request carries the
transcript to that point, so replay is a read of stored blobs rather than a reassembly of deltas.

#### Scenario: Sessions are listed with rollups

- **WHEN** `tinyroute sessions` is run
- **THEN** each session MUST be listed with its turn count, providers used, and token totals

#### Scenario: Sessions can be filtered by key

- **WHEN** sessions are listed filtered to one key identifier
- **THEN** only sessions authorized by that key MUST be shown

#### Scenario: A session is replayable

- **WHEN** `tinyroute session <id>` is run for a session captured in full mode
- **THEN** the conversation MUST be reconstructed from stored blobs

#### Scenario: Replay is unavailable without bodies

- **WHEN** replay is requested for a session recorded in metadata-only mode
- **THEN** the command MUST report that bodies were not captured rather than emit a partial transcript

### Requirement: History can be queried and followed from the terminal

`tinyroute log` SHALL read the history file directly and support following new records as they are
appended, filtering to failures only, restricting to a time window, and filtering by session or key.
These commands MUST NOT require the daemon to be running and MUST NOT communicate with it.

#### Scenario: Following new records

- **WHEN** `tinyroute log -f` is run while requests are being served
- **THEN** new records MUST appear as they are appended

#### Scenario: Filtering to failures

- **WHEN** history is filtered to failures
- **THEN** only records whose outcome is not success MUST be shown

#### Scenario: Works with the daemon stopped

- **WHEN** the daemon is not running
- **THEN** history commands MUST still read and report recorded records

### Requirement: Superseded request blobs are reclaimed only after verification, and only offline

`tinyroute compact` SHALL reclaim space by deleting request blobs that are superseded by a later
request in the same session, and MUST delete a blob only after verifying that the later request's
message sequence contains the earlier one as a leading subsequence. Compaction MUST be an offline
command and MUST NOT run within the request path, so that a defect in it cannot affect request serving.
Response blobs MUST NOT be deleted by compaction.

#### Scenario: Superseded blob is reclaimed

- **WHEN** a later request in a session verifiably contains an earlier request's messages as a leading
  subsequence
- **THEN** the earlier request blob MUST be deleted
- **AND** its record MUST indicate the body is no longer available

#### Scenario: Unverifiable supersession is left intact

- **WHEN** a later request does not contain the earlier one as a leading subsequence, such as after
  context compaction or a sub-agent invocation
- **THEN** the earlier blob MUST be retained

#### Scenario: Response blobs are preserved

- **WHEN** compaction runs
- **THEN** no response blob MUST be deleted

#### Scenario: Compaction never runs in the request path

- **WHEN** requests are being served
- **THEN** no compaction work MUST occur as part of handling a request

### Requirement: History storage is permission-restricted and bounded

The blob directory SHALL be created with owner-only access and blob files SHALL be owner-readable only,
because captured bodies contain complete agent transcripts including source code and any secrets the
agent encountered. The history file SHALL respect a configured size ceiling by rotation. The service
SHALL warn when full capture is enabled so the exposure is explicit rather than silent.

#### Scenario: Restrictive permissions on creation

- **WHEN** the blob directory and history file are created
- **THEN** the directory MUST be owner-only accessible and files MUST be owner-readable only

#### Scenario: History file is rotated at its ceiling

- **WHEN** the history file reaches its configured size ceiling
- **THEN** it MUST be rotated rather than grow without bound

#### Scenario: Full capture is surfaced to the operator

- **WHEN** the service starts with full capture enabled
- **THEN** it MUST report that complete request and response bodies are being archived
