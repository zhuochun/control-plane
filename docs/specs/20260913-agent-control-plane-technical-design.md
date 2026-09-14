# aicp - Agent Control Plane — MVP Technical Decisions and Implementation Plan

- Date: 2026-09-13
- Status: Implementation baseline, not a survey of alternatives.
- Product contract: [MVP product specification](20260913-agent-control-plane-product-spec.md)
- Repository/module: `github.com/zhuochun/control-plane`
- Executable: `aicp` (`aicp.exe` on Windows)
- Naming: `aicp` is the product, command, and MCP registration name. Keep the existing repository/module and spec filenames; the data directory remains `control-plane` as specified below.

## 1. Architecture decisions

Build a small Go monolith with an embedded React SPA and SQLite. The CLI and an MCP stdio adapter are clients of the running local server. The harness owns source access, intelligence, and heartbeat scheduling.

```text
External cron / launchd / Task Scheduler / harness heartbeat
                         |
                         v
                 Codex or another harness
                  |                  |
          existing source tools      +-- aicp CLI / MCP stdio adapter
                  |                                  |
          Slack / email / Docs                       | HTTP/JSON
                                                     v
Browser -- HTTP/JSON --> aicp serve --> domain commands --> SQLite
             React SPA       |
                  ^          +-- embedded static assets and migrations
                  +----------+
```

Only `aicp serve` opens the database. There is one long-lived server, plus transient CLI processes and an optional harness-managed MCP adapter. There is no internal agent launcher, cron library, background source poller, or durable message broker.

### 1.1 Selected stack

| Concern | P0 choice | Decision |
| --- | --- | --- |
| Backend | Go, `net/http`, `encoding/json`, `log/slog` | Standard library routing, JSON, logging, and graceful shutdown. |
| CLI | Cobra | Command tree, help, flags, completion; no TUI. |
| Database | SQLite via `modernc.org/sqlite`, `database/sql` | Embedded, CGO-free driver; handwritten SQL, no ORM. |
| Migrations | `pressly/goose/v3`, embedded SQL | Run pending migrations at server startup. |
| MCP | Official `modelcontextprotocol/go-sdk` | Stdio adapter first; no second HTTP MCP server in P0. |
| Web | React + TypeScript + Vite | Build-time SPA; no Next.js/SSR or production Node server. |
| Components | MUI Core with its documented styling dependencies | Cards, forms, dialogs, tabs, and standard tables; no paid grid. |
| Routing | React Router in library mode | Browser routing only; no server framework. |
| Server state | TanStack Query | Mutations and query invalidation; poll visible views every five seconds. |
| Reports | `react-markdown` + `remark-gfm`, Markdown renderer and fixed action handlers | No arbitrary generated HTML/JS/React. |
| Date handling | Native date/time inputs; server-side Go timezone conversion | Avoid a separate date-picker framework in P0. |
| Testing | Go tests/`httptest`; Vitest/Testing Library; Playwright | Unit, integration, adapter, and browser coverage. |
| Packaging | Go `embed`, GoReleaser, GitHub Actions | Platform archives plus checksums; package-manager publishing later. |

Go 1.27.1 is the verified baseline for this document; the official release history lists its 2026-09-01 release. Use that toolchain or a later verified patch of the same supported line. Do not infer future SDK/protocol versions from dates. [Go release history](https://go.dev/doc/devel/release)

Use Node 24.x as the build/CI baseline, subject to the chosen Vite and package engine requirements. Node is not a runtime requirement of the installed application. Select mutually compatible, non-prerelease dependency versions during scaffolding; commit `go.mod`, `go.sum`, `package-lock.json`, and the build-tool version configuration. CI must not resolve floating `latest` dependencies. [Node releases](https://nodejs.org/en/about/previous-releases), [Vite requirements](https://vite.dev/guide/)

### 1.2 Deliberate reductions from the earlier architecture

Use five-second polling instead of SSE; interval due queries instead of a scheduler; substring search instead of FTS; small report/context JSON and Markdown in SQLite instead of a separate artifact store; a normal item list instead of drag-and-drop; foreground `serve` instead of an OS-service management framework.

No Redis, Postgres, Temporal, GraphQL, vector database, plugin system, ORM, event-sourcing framework, or Electron/Tauri. No authentication/authorization project: this is a single-user loopback application.

Do not confuse removal of permission features with removal of data correctness. JSON validation, parameterized SQL, bounded input, valid link types, and conflict handling are still ordinary product requirements.

## 2. Repository and module boundaries

```text
cmd/aicp/main.go
internal/
  app/                 # commands, queries, validation, transactions
  model/               # domain records and transport-neutral contracts
  store/               # SQLite queries and embedded migrations
  httpapi/             # versioned HTTP handlers and errors
  client/              # typed local HTTP client, shared by CLI and MCP
  cli/                 # Cobra commands and JSON/file input
  mcp/                 # stdio tools -> internal/client
  report/              # report/action validation
web/
  assets.go            # embeds dist/ after frontend build
  src/
  package.json
  package-lock.json
schemas/
  report-v1.schema.json
  watch-result-v1.schema.json
examples/
  heartbeat-prompt.md
  heartbeat.sh
  heartbeat.ps1
  slack-watch.json
  slack-result.json
  cost-result.json
  mcp-config.toml
  scheduling.md
testdata/
  scenarios/
tools/
  fixture-harness/
docs/specs/
.github/workflows/
.goreleaser.yaml
README.md
```

Dependency direction: adapters -> app -> store/model. CLI and MCP -> client -> HTTP API, never -> store. Web actions and agent writes reach the same app commands. Avoid an interface for every struct; define interfaces where an alternative is actually used, particularly the clock and HTTP client.

The server owns configuration, schema migration, state, and logs. It never imports an LLM provider SDK or executes a configured agent command.

## 3. Local runtime and installation contract

Defaults:

```text
Listen: 127.0.0.1:7331
Data directory: os.UserConfigDir()/control-plane
Override: --data-dir or AICP_DATA_DIR
Client base URL: --server or AICP_SERVER_URL
Settings timezone: UTC until selected in UI
```

Persist settings, including the display timezone, in SQLite. Include Go `time/tzdata` so named timezones work without relying on a separately installed zoneinfo database.

Core lifecycle commands:

```sh
aicp serve
aicp open
aicp doctor --json
aicp version
```

`serve` runs in the foreground and stops cleanly on an interrupt. Hold an OS-backed exclusive lock for the selected data directory before opening SQLite; a second server using that directory, even on a different port, must fail with a clear message. The lock must release on process exit/crash rather than relying on a stale PID file. `open` opens the configured portal URL or prints it when browser launch is unavailable. `doctor` checks server version, database/schema health, last received run, and Watch coverage; it does not test Slack credentials or claim an external scheduler is installed.

A stopped server produces a clear CLI/MCP connection error and the suggested `aicp serve` command. Adapters must not silently open the database or start competing daemons. A port conflict is an actionable startup error, not a reason to choose a random port.

Create the data directory on first use. A schema from a newer incompatible application fails with an upgrade message rather than being reset. Do not copy a live WAL database as a backup; document stopping the server before copying the data directory for the first release.

Release installation means downloading and extracting the appropriate archive, placing the binary on PATH, and running `aicp serve`. Homebrew/Scoop/winget names must not be advertised as available before they are actually published. `go install` is not the primary installation promise because a source checkout needs a frontend build.

## 4. Storage design

### 4.1 SQLite setup

Use one `database/sql` connection initially (`SetMaxOpenConns(1)`), short transactions, foreign keys, a busy timeout, and WAL on a local filesystem. Set connection-local pragmas on every connection that can be created, not just once on an arbitrary pooled connection. Suggested initial settings are `foreign_keys=ON`, `busy_timeout=5000`, `journal_mode=WAL`, and `synchronous=FULL`.

The CGO-free driver is selected for packaging convenience. SQLite WAL supports concurrent readers but still has one writer and should not be placed on a network filesystem. [modernc driver](https://pkg.go.dev/modernc.org/sqlite), [SQLite WAL](https://sqlite.org/wal.html)

SQL migrations and frontend assets are embedded into the binary. Before a migration that changes existing data, take a consistent backup or use the documented stopped-server backup procedure; never silently destroy an existing database. [Embedded migrations](https://pressly.github.io/goose/blog/2021/embed-sql-migrations/), [Go embed](https://pkg.go.dev/embed)

### 4.2 Minimal tables

**Column admission rule:** every structured field must name the P0 query, relationship, constraint, state transition, or rendering action that consumes it. Use Markdown for meaning interpreted by agents. Do not introduce topic taxonomies, threshold objects, evidence graphs, outcome metrics, or source-status enums, including inside JSON. A field being easy to extract is not a reason to store it separately.

The table list separates durable operational responsibilities; it is not an instruction to normalize every content attribute. JSON below is a bounded document stored in SQLite TEXT, not a family of child tables. Keep the minimal transport envelope typed; treat prose and source-specific checkpoints as opaque. Do not duplicate content between columns and payloads.

| Table | Main fields / purpose |
| --- | --- |
| `settings` | Key/value app settings, including timezone. |
| `interests` | ID, title, one `instructions_md`, lifecycle state, revision, created/updated timestamps. Title supports display; state/revision support eligibility and safe edits. |
| `watches` | ID, Interest ID, primary source JSON (`kind`, `locator`), one `instructions_md`, interval/lookback seconds, lifecycle state, revision, opaque cursor JSON, `next_due_at`, created/updated timestamps. These support source identity, inspection windows, eligibility, and checkpoint fencing. Derive last attempt/success/error from `watch_results`. |
| `items` | ID, globally unique dedupe key, kind, optional Interest/Watch/parent IDs, title/summary, current content JSON, content version/hash, `todo_state`, `remind_at`, reminder timezone, acknowledged content version, `user_note`, state version, created/content-updated/state-updated timestamps. Columns support identity, filtering, navigation, no-op detection, and independent user actions. Content JSON owns report, sources, and optional context; no duplicate body/context/status columns. |
| `item_versions` | Immutable snapshots of substantive item content by item ID and content version. |
| `proposals` | ID, unique proposal key, target type/ID, expected target revision, operation, configuration payload, one `rationale_md` containing reason and evidence links, resolution state. Payload reuses the minimal Interest/Watch configuration contract. |
| `runs` | ID, runner label, status, start/end/lease expiry, selected Watch and parent Interest revisions, brief event watermarks, summary. |
| `watch_results` | Run/Watch IDs (unique pair), status, recorded timestamp, bounded result JSON containing coverage, error text, and item IDs. Supports per-Watch health and run completion; retry responses belong only in `command_receipts`. |
| `events` | Monotonic sequence, timestamp, actor label, entity, change type, and compact payload. |
| `command_receipts` | Unique request ID, normalized body hash, response; implements retry idempotency. |

Store the singleton acknowledged event cursor as a server-owned settings key, updated transactionally by run completion and excluded from user-editable settings. `GET/PATCH /settings` exposes only user-editable keys; the cursor is neither returned nor accepted there. No separate `runner_state` table is needed. Keep version snapshots, events, and receipts: they respectively support content history, change delivery, and retry safety.

Required relationships belong in columns only where the application navigates, filters, or constrains them. Parent item IDs support direct handoff navigation. Other related IDs and external agent/session references can be links or labeled text in `context_md`; no delegation or relationship tables. Item `context_md` is optional free-form handoff text inside content JSON, without prescribed headings or subfields.

Source references use only `id`, `url`, `label`, and `observed_at` for action resolution, navigation, attribution, and freshness. Keep source-specific IDs in the dedupe key or context when needed; do not require a second normalized source identity on every reference. Watch source identity is the exact `kind`/`locator` pair; compare it to decide when to clear a checkpoint. Cursors remain source-specific opaque JSON.

Use UUIDs with readable prefixes where helpful; IDs are opaque, and clients must not parse ordering from them. Store instants consistently as UTC integers in SQLite, expose RFC3339 timestamps in JSON, and store the reminder's IANA timezone separately.

Store small report bodies in the DB in P0; an item content snapshot is capped at 512 KiB and a publication request at 4 MiB. Larger file artifacts and attachment uploading are deferred. Preserve Markdown/JSON exportability through item read APIs.

Index Watch eligibility, item dedupe keys, Todo/reminder filters, Interest relationships, event sequence, and Run/Watch results (including Watch ID/recorded time for health reads). Basic search is a parameterized literal substring over title, summary, and `context_md` extracted from content JSON; escape SQL LIKE wildcards. Do not maintain a duplicate context column for this bounded P0 query. This intentionally supports simple Chinese substring queries without claiming semantic search. FTS5 is a later optimization, with tokenizer behavior evaluated before adopting it. [SQLite FTS5](https://sqlite.org/fts5.html)

### 4.3 Separate content from local state

Agent-owned content includes the title, summary, report body, source references, and optional context text. Source status and external IDs are described in that content or represented by the dedupe key when the agent needs them; they are not additional required item columns. User interaction state includes Todo, reminder, acknowledged content version, and user note.

`content_version` and `state_version` are independent. A report refresh must not conflict merely because the user set a reminder, nor overwrite that reminder. Agent ingestion may specify initial local state on creation only; subsequent publications ignore no state fields silently—they reject attempts to set existing local state through the content contract.

Use normalized content hashing for no-op detection. Exclude last-inspection timestamps and run metadata from the substantive hash; keep them as freshness metadata. Normalize ordered versus unordered collections deliberately. An unchanged item creates neither a new content version nor an unacknowledged update. An agent should also avoid gratuitous paraphrasing when source content has not changed.

Specifically exclude `sources[].observed_at` from the substantive hash. A valid publication with otherwise identical content updates these timestamps in the current content JSON without changing `content_version`, `content_updated_at`, acknowledgement, or immutable history; its Watch-result record still records the inspection. For a retained source ID, keep the later observation instant. Historical snapshots retain the observation times present when created. Source IDs/URLs/labels, title, summary, kind, relationships, context, Markdown, and actions remain substantive. Canonicalize JSON object keys and source order by source ID; preserve Markdown text and action order. Treat omitted actions as `[]` and omitted context as empty text. No-op detection does not bypass revision or lease checks.

Deduplication keys identify the continuing matter, not the run or current summary. Example: `slack:T_EXAMPLE:C_PLATFORM:thread:1726000000.000001`. Include stable workspace/account/source identity to avoid collisions; do not rely on the display title. A new version of the same thread uses the same key.

### 4.4 Transaction and retry rules

All mutating requests include `request_id`; the CLI generates one unless supplied, and callers reuse it for retries. Persist its body hash and response with the mutation. Identical repeats return the stored response; reusing the ID with a different body returns `409 idempotency_conflict`.

Check an existing receipt before checking current lease/version state, so retrying a previously committed request still succeeds after its run has finished. Do not automatically retry a failed mutation with a newly generated request ID.

User-state commands require `expected_state_version`; content updates require `expected_content_version` when updating an existing item; config updates require `expected_revision`. A new item uses expected content version zero and a unique dedupe key. An unexpected existing item returns its ID/version as a conflict for reread, rather than being overwritten.

Update current state, content history where applicable, the event, and the receipt in one transaction. This is ordinary relational storage with an append-only activity log, not event sourcing.

## 5. Mode A: brief, runs, and source checkpoints

### 5.1 Eligibility, not scheduling

A Watch is eligible when it and its parent Interest are active and `next_due_at <= now`. A new Watch is due immediately. After a successful committed scan, advance `next_due_at` along its existing interval grid to the first slot strictly after the run start, not after scan completion. If a forced run starts before the current due time, retain that future slot. Skip missed slots without queuing catch-up runs. Computing the next due time as completion time plus interval would make a two-hour external cron skip the following tick whenever a scan takes time. A failed/partial scan does not advance its successful cursor and remains eligible for the next external heartbeat.

Accept intervals from 1 to 31,536,000 seconds; use 7,200 seconds by default. There are no cron expressions or automatic catch-up executions in P0. Pausing or changing a Watch increments its configuration revision. An explicit interval/instruction change or resume makes it due now and reanchors its interval grid. Changing the primary source identity also clears its successful cursor and coverage baseline; never apply one source's cursor to another. Preserve prior run history. Do not run background timers just to mark records due; eligibility is queried against the clock.

For interactive debugging and the fixture demo, `aicp run start --watch <id> --force` may select an active Watch before its due time. It still does not launch a harness, and normal completion follows the same checkpoint rules.

### 5.2 Brief and run start

`GET /api/v1/brief` is read-only. It returns eligible Watch summaries, pending human/config changes, open related items, reminders, and operational health. Large reports are referenced by ID, not embedded by default.

`POST /api/v1/runs` registers work and returns the authoritative run brief: selected Watch IDs and revisions, source cursors, relevant context IDs, and an event range `(after_seq, through_seq]` captured at run start. Default selection is up to 20 eligible Watches; return `more_due_count`, not silent truncation. Events are pageable within the captured range.

Capture each selected Watch's parent Interest ID, revision, and instructions in the same transaction. Publication checks the current parent revision against this server-held snapshot as well as checking the Watch revision. Any intervening Interest edit returns `409 interest_changed` and commits no items or checkpoint. Interest edits increment its revision; instruction changes or reactivation make its active Watches due now without clearing their source cursors. This prevents a previously completed Watch from delaying inspection under the revised instructions. Pausing/deprecating still makes the Watches ineligible. No extra client-supplied Interest revision is needed in each publication.

P0 has one active periodic run globally. Serialize start in a transaction, expire any overdue lease, then acquire the run. A competing start receives `409 run_in_progress`. Use a 30-minute lease and a renew command for longer analyses. The lease is durable; late publications from an expired run are rejected.

An actor/runner label is attribution, not authentication. Multi-runner independent scheduling and delegation leases are not part of P0.

### 5.3 Publish a Watch result

A result contains a request ID, Run/Watch IDs, expected Watch revision, coverage outcome, cursor before/after, source timestamps/limitations, and bounded item upserts. A publication can include zero items for a successful no-change scan.

Publish a successful Watch result atomically: validate lease and Watch revision, validate items, save content versions/events, record coverage, advance that Watch's cursor and due time, and save the receipt. P0 permits one final result per Run/Watch pair.

For `partial` or `failed`, record the attempt and error/limitations but do not advance the successful cursor. A failed result contains no item upserts. Partial findings may be saved only if explicitly marked partial in their content; they never imply full source coverage. A later run may reread the same window; item deduplication makes replay harmless.

Every committed result (`success`, `partial`, or `failed`) is terminal for that Run/Watch pair. A partial result cannot be upgraded in the same run. Reusing its request ID and identical request replays the receipt; a new request ID for an already committed pair returns `409 watch_result_exists`. Validation/conflict failures commit no result, so a corrected request may use a new request ID while the run remains valid. Partial item writes, coverage, events, and receipt commit atomically, with cursor and due time unchanged. To complete partial coverage, finish the current run and inspect again in a later run. The existing "one final result" rule does not permit incremental chunks within one Watch.

The request body uses the following envelope; Run and Watch IDs are path parameters. This valid example is a successful empty scan. Item entries use the shape in section 7. `status` is `success | partial | failed`; include an `error` string for partial/failed results and omit or null `cursor_after` unless successful.

```json
{
  "request_id": "req-example-empty-scan-1",
  "expected_watch_revision": 1,
  "status": "success",
  "coverage": {
    "cursor_before": null,
    "cursor_after": {"last_seen_version": "source-version-2"},
    "observed_through": "2026-09-13T10:04:00+08:00",
    "limitations": []
  },
  "items": []
}
```

`cursor_before` must match the captured Watch checkpoint. Treat source cursors as opaque JSON; the Control Plane cannot prove the harness consumed an external source completely. Validate the envelope and preserve limitations without calling that independent source verification.

Never advance the cursor past unprocessed source content merely because the publication limit was reached. The harness must choose a smaller fully processed window or report partial coverage.

If Watch configuration changed after the brief, return `409 watch_changed`; do not overwrite the new config or advance its checkpoint using the old instructions. A paused/deprecated Interest also blocks new result commits for its Watches.

### 5.4 Finish and acknowledge changes

Finish closes the run and records a summary. All selected Watches succeeding yields `completed`; at least one success or partial result with other unsuccessful/missing work yields `partial`; otherwise use `failed`. A missing result is not a success. A zero-Watch run may complete after processing its captured changes. An abandoned run eventually becomes `expired`.

`ack_through_seq` may advance the singleton change cursor only after the agent has consumed that event range and durably recorded any resulting work/context. It cannot exceed the run's captured `through_seq`, and cannot skip an unconsumed page. A failed run defaults to no cursor advance.

Select human-state changes and configuration/proposal changes for the brief; do not feed ordinary agent report publications back as new user instructions. Human edits after `through_seq` necessarily appear in a later run. Deliver changes at least once: replay is acceptable; loss is not. The activity feed can use the same monotonic event log, but it does not control the runner's acknowledgement cursor.

## 6. HTTP, CLI, and MCP surface

The HTTP prefix is `/api/v1`. IDs in examples are illustrative. Preserve shared request/response semantics across adapters; do not expose generic SQL or arbitrary field mutation tools.

| HTTP capability | CLI command family | Notes |
| --- | --- | --- |
| `GET /healthz`, `GET /status` under API | `doctor`, `version` | Liveness versus source/run health. |
| `GET /brief`, `GET /changes?after_seq=&through_seq=` | `brief`, `changes` | Bounded, pageable reads. |
| `GET /runs`, `GET /runs/{id}` | `run list/get` | History, selected work, and per-Watch results. |
| `POST /runs` | `run start` | Register/claim; never spawn an agent. |
| `POST /runs/{id}/renew` | `run renew` | Extend a live lease. |
| `PUT /runs/{id}/watches/{watch}/result` | `run publish --file ...` | Atomic result and checkpoint. |
| `POST /runs/{id}/finish` | `run finish` | Close run and optionally acknowledge captured changes. |
| `GET/POST /interests`, `GET/PATCH /interests/{id}` | `interest list/get/create/update` | Config revisions; no delete needed. |
| `GET/POST /watches`, `GET/PATCH /watches/{id}` | `watch list/get/create/update` | Source spec and lifecycle. |
| `GET /items`, `GET /items/{id}`, `POST /items` | `item list/get/create` | Direct local creation as well as run publication. |
| `PATCH /items/{id}/content` | `item update` | Explicit content/context correction with version check. |
| `POST /items/{id}/actions` | `item action` | Local state mutations, shared with generated UI. |
| `PATCH /items/{id}/note` | `item note` | User note with request ID and expected state version; not agent content. |
| `GET /items/{id}/context`, `GET /items/{id}/history` | `context get`, `item history` | Bounded context and versions. |
| `GET/POST /proposals`, `POST /proposals/{id}/resolve` | `proposal list/create/resolve` | Accept/reject with target revision check. |
| `GET/PATCH /settings` | `config get/set` | Timezone and small app settings. |

Except `/healthz`, paths above are relative to `/api/v1`. Item listing supports an exact `dedupe_key` filter so agents can read the existing ID/content version before publishing. The note endpoint accepts `user_note` and uses the same state command layer; it does not expand the generated-report action allowlist. Lists have deterministic ordering and an opaque continuation cursor; a page includes `next_cursor` or null. Reads must not silently omit records needed for event acknowledgement.

Creation JSON is accepted with `--file <path>` or `--file -` for stdin. All agent-facing commands support `--json`. JSON output goes to stdout; diagnostic logging goes to stderr. Avoid ANSI/prompting in noninteractive mode. Define exit codes: 0 success, 2 validation/usage, 3 version/idempotency conflict, 4 server unavailable, 5 run in progress, 1 other failure.

Common error envelope:

```json
{
  "error": {
    "code": "state_conflict",
    "message": "The item state changed. Read it again before applying this action.",
    "retryable": false,
    "details": {"current_state_version": 4}
  }
}
```

Use `400` for malformed JSON, `404` for unknown IDs, `409` for conflicts/overlap, `413` for size limits, `422` for valid JSON with invalid fields, and `500` for unexpected failures. Return validation paths. Use `503` only for a genuinely temporary unavailable server dependency.

### 6.0 Shared command payloads

These are transport contracts, not new SQL fields. All mutation bodies contain `request_id`. Path IDs remain outside the HTTP body. Reject unknown envelope fields; opaque cursors and Markdown remain unconstrained by domain schemas. A successful mutation returns the resulting record (with ID and relevant revisions), except publication, which returns `{run_id, watch_id, status, items: [{id, content_version, state_version, changed}]}`. Store the complete response in the receipt. Bind receipt identity to HTTP method, path, and normalized body, so the same body sent to a different item cannot replay another item's command.

| Operation | Body fields in addition to `request_id` |
| --- | --- |
| Start run | `runner_label`; optional `watch_ids` (omitted selects due Watches, `[]` selects none), `force` (default false). Force requires explicit IDs and never selects inactive Watches. Return the run and captured `brief`. |
| Renew run | No additional fields. Extend a live lease to server now plus 30 minutes; expired/finished runs conflict. |
| Finish run | `summary`, optional `ack_through_seq` (omission leaves cursor unchanged). Server computes status under section 5.4; expired runs cannot advance acknowledgement. |
| Create Interest | `title`, `instructions_md`, optional `state` (default active). |
| Create Watch | `interest_id`, `source: {kind, locator}`, `instructions_md`, optional `interval_seconds` (7200), `lookback_seconds` (604800), `state` (active). |
| Update Interest/Watch | `expected_revision` and changed editable fields from creation. Omitted fields are unchanged. Reject null for required configuration fields; server-owned cursor/due fields cannot be supplied. Watch parent is fixed after creation. |
| Create item | The item shape in section 7, with `expected_content_version: 0` and optional `initial_todo_state`. |
| Update item content | `expected_content_version` and changed agent-owned fields from section 7. Dedupe key is immutable; omitted fields are unchanged. `report` and `sources` replace their complete values when supplied; `context_md: null` clears context. No existing local-state fields are accepted. |
| Item action | `expected_state_version`, `action` as specified below. Return the updated item. |
| User note | `expected_state_version`, `user_note` (empty text clears). |
| Propose change | `proposal_key`, `target_type`, `operation`, `rationale_md`; create uses `payload` with creation fields and no target ID/revision; update uses `target_id`, `expected_revision`, and an editable-fields `payload`; deprecate uses target ID/revision without payload. |
| Resolve proposal | `resolution` is `accepted` or `rejected`. The pending-to-resolved transition is atomic; repeat same resolution returns the resolved record, opposite resolution conflicts. Acceptance checks the proposal's stored target revision. |
| Settings | `timezone` (valid IANA name); no internal keys. This single display preference uses last committed write wins, unlike versioned Interest/Watch configuration. |

An item action is one of `{type: set_todo, state: none|todo|done}`, `{type: clear_reminder}`, `{type: acknowledge, content_version}`, or `{type: set_reminder, date, time?, timezone, utc_offset?}`. These are shape notations; actual JSON strings must be quoted. Acknowledge requires `1 <= content_version <= current_content_version` and advances acknowledgement monotonically. The reminder fields follow section 7. `open_link` is navigation and has no mutation endpoint. Generated reminder descriptors contain no date: the fixed editor constructs the mutation body.

### 6.1 MCP subset

`aicp mcp` runs the official SDK's stdio transport and delegates to the same typed HTTP client as the CLI. It must not open a second SQLite connection or serve the web app.

Expose a small initial tool set: `get_brief`, `get_changes`, `start_run`, `renew_run`, `publish_watch_result`, `finish_run`, `get_item`, `get_context`, `propose_change`, and `apply_item_action`. Bootstrap/configuration and less frequent management remain available through the CLI. Full one-to-one MCP coverage of every CRUD endpoint is not a P0 requirement.

MCP reuses the HTTP request/response types above; it does not define another domain schema. `get_brief` takes `{}`; `get_changes` takes `after_seq`, `through_seq`, and optional `cursor`; `get_item` and `get_context` take `item_id` (context also accepts optional `cursor`). `start_run` and `propose_change` take their HTTP bodies directly. `renew_run` and `finish_run` add `run_id` to their HTTP bodies. `publish_watch_result` adds `run_id` and `watch_id` to the section 5.3 body. `apply_item_action` adds `item_id` to the item-action body. The adapter removes path IDs before sending HTTP. Reads return the corresponding HTTP result; mutations return the command result above. Tool failures preserve the common error code/message/details and mark the tool result as an error. Commit exact tool schemas and shared request/result fixtures with the implementation; schema generation must use these shared types.

Use structured typed arguments/results and document each tool's effects. Stdout contains only MCP messages. The official SDK provides stdio transport and schema-aware tools; pin a tested stable tag rather than promising a particular future protocol version. [Official Go SDK](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp)

The target Codex registration after installing `aicp` is:

```sh
codex mcp add aicp -- aicp mcp
```

Use an absolute executable path where PATH is not inherited by the harness. This follows Codex's documented stdio registration form; test against the installed Codex version. [Codex MCP](https://developers.openai.com/codex/mcp/)

## 7. Report and action contract

Commit `schemas/report-v1.schema.json` and `schemas/watch-result-v1.schema.json` with matching Go types, TypeScript discriminated unions, and valid/invalid shared fixtures. Use strict decoding and explicit domain validation in Go; schema documents are also part of the agent-facing interface. Tests must detect schema/implementation drift.

The containing Item owns its summary, sources, and local state. Its `report` field contains `schema_version: 1`, `body_md`, and optional `actions` (default empty). Markdown provides lists and tables; no separate block schema is required. The current Todo/reminder values are never copied into report JSON. Validate the envelope, source references, and executable action descriptors; do not validate prose into a domain ontology.

All four item kinds use this same required `report` envelope, including notes, tasks, and outcomes. `kind` controls labeling/filtering and the creation-time Todo default only; it does not select a content schema. Creation requires `dedupe_key`, `expected_content_version: 0`, `kind`, `title`, `summary`, `sources` (an array, possibly empty for local items), and `report`. Optional fields are `context_md`, `interest_id`, `watch_id`, `parent_id`, and `initial_todo_state` (creation only). For Watch publications, the server assigns the publishing Watch/Interest association and rejects conflicting supplied IDs. Source-backed findings require at least one source reference. Each source ID is unique within the item; action source references must resolve there. The API exposes these fields directly; storage places only report/sources/context in content JSON and assembles the full item from its columns. `body_md` may be plain prose and does not require headings; no kind-specific body fields or tables are needed.

Example item in a Watch-result publication (all values are synthetic):

```json
{
  "dedupe_key": "slack:T_EXAMPLE:C_PLATFORM:thread:1789264800.000001",
  "expected_content_version": 0,
  "kind": "report",
  "title": "Confirm the migration rollout sequence",
  "summary": "The thread asks for a decision on the rollout order. No decision has been observed yet.",
  "sources": [
    {
      "id": "thread",
      "url": "https://example.slack.com/archives/C_PLATFORM/p1789264800000001",
      "label": "Migration discussion",
      "observed_at": "2026-09-13T10:04:00+08:00"
    }
  ],
  "context_md": "Confirmed: a rollout decision is outstanding. Next check: inspect replies in this thread. Do not treat an acknowledgement as a decision.",
  "report": {
    "schema_version": 1,
    "body_md": "## What changed\nThe proposed order changed after the dependency review. Source status: open; no decision was observed in the linked thread.\n\n| Option | Trade-off |\n| --- | --- |\n| Small cohort first | Slower expansion, easier validation |\n| Full cohort | Faster expansion, larger validation surface |\n\n## Suggested next step\nRead the options and reply in the original thread.",
    "actions": [
      {"id": "open-thread", "type": "open_link", "label": "Open Slack thread", "source_ref": "thread"},
      {"id": "todo", "type": "set_todo", "label": "Set Todo", "state": "todo"},
      {"id": "remind", "type": "set_reminder", "label": "Remind me"}
    ]
  }
}
```

A source reference resolves only within this item's source list. `open_link` uses a normal browser anchor with `target="_blank"` and appropriate `rel`; clicking it performs no server mutation. Support HTTP(S) links in P0, including Slack web permalinks, not custom executable URI schemes.

`set_todo`, `clear_reminder`, and `acknowledge` invoke the shared item-action command. `set_reminder` first opens the normal reminder form; generated content cannot silently choose a hidden date or execute arbitrary payloads. The user can always access standard controls outside the report.

Example local action request:

```json
{
  "request_id": "req-example-reminder-1",
  "expected_state_version": 3,
  "action": {
    "type": "set_reminder",
    "date": "2026-09-14",
    "time": "09:00",
    "timezone": "Asia/Singapore"
  }
}
```

Date and optional time are interpreted on the server in the supplied valid IANA timezone; omitted time is 09:00. The response includes the resulting RFC3339 UTC instant, selected timezone, and new state version. Validate a round trip to reject nonexistent local times. For an ambiguous local time, return `422 ambiguous_local_time` with candidate `utc_offset` values (`+HH:MM` or `-HH:MM`). The fixed editor asks the user to select an occurrence and resubmits with the selected offset and a new request ID. An offset inconsistent with the named zone/date/time is rejected. P0 uses this one input form; a separate RFC3339 `at` input is deferred.

Changing the app display timezone later does not move stored reminder instants. Done clears `remind_at`; reopening does not resurrect it. Removing Todo (`none`) does not clear an independent reminder. Acknowledge includes the displayed `content_version` and must not acknowledge newer unseen content.

Use a server-supplied `now` for reminder eligibility, not a browser-only guess. No persistent reminder job is necessary: query `remind_at <= now`, with the product's Done/clearing rules. Clock-dependent logic uses an injected clock in tests.

Render Markdown with raw HTML disabled. Unknown report versions/action types are rejected at ingestion. For old stored documents with unsupported report versions, show an explicit fallback containing the fixed summary and source links; keep standard local controls functional. Avoid making schema validation a general plugin engine.

A single Markdown renderer and a small action map/switch over a discriminated union are sufficient. No block registry, dynamic imports selected by the model, remote component execution, or generic form builder. [react-markdown](https://github.com/remarkjs/react-markdown)

## 8. Proposals and context

### 8.1 Configuration proposals

Proposal operations are `create | update | deprecate`; targets are `interest | watch`. A proposal has a stable key incorporating target/change intent and source evidence version, one `rationale_md` explaining the reason and linked evidence, and a configuration payload using the same minimal fields as a direct configuration edit. Update/deprecate includes `expected_revision`. Evidence interpretation belongs to the agent; the server deduplicates the supplied key and validates the configuration operation.

Acceptance applies the configuration change and resolves the proposal in one transaction. A changed target yields `409 proposal_stale`; the user can reject it or the agent can submit a revised proposal. Repeating acceptance returns the existing result, not another object. Rejection records an event and keeps the evidence/key for repeat suppression.

Deprecation is soft. It stops eligibility; it does not delete reports, Todo state, reminders, or context. Interest state and Watch state are checked together. Do not implement cascaded record destruction or automatically close local work.

### 8.2 Context packets

`GET /items/{id}/context` returns the item, source references, optional `context_md`, local state/user note, parent ID, and a bounded recent history. Related item links and external delegation/session references remain readable within context text; do not require extracted fields. Include content/state versions and item/version references rather than dumping every historical report.

Allow explicit context updates through the versioned item-content endpoint. Background updates must incorporate the user's note rather than overwriting it. No chain-of-thought storage is required; save usable facts, conclusions, attempted approaches, open questions, and next steps.

Return byte/item limits, a `truncated` flag, and continuation links when bounded. Do not claim to have the complete context after silently truncating it. A full memory engine and graph scheduler are out of scope.

## 9. Frontend behavior and implementation

Use a small client-side router for Attention, Interests, Library, Activity, and stable `/items/:id` links. The Go SPA handler must serve the real `index.html` for valid client routes but not swallow unknown `/api/...` routes or missing static assets.

Use MUI Core's standard controls and tables. Keep layout and action semantics fixed; only the report Markdown body and action descriptors are agent-specified. No custom design system, drag library, chart library, or premium component is necessary to deliver P0. [MUI installation](https://mui.com/material-ui/getting-started/installation/)

Configure TanStack Query explicitly: poll active views every five seconds, refetch on focus, stop unnecessary polling in hidden views, invalidate related queries after mutations, and do not retry mutations blindly. An operation shows success only after the server acknowledges it. Conflict errors offer refresh/reapply rather than pretending the click succeeded. [TanStack Query defaults](https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults)

A card's source-observed time, last successful scan, content update time, and reminder time are different fields. Do not label all of them “updated.” Human edits remain visible while a new report arrives. The portal must also expose a server-offline state when polling fails.

Due reminders, Todos, and unacknowledged content form one deduplicated attention projection. Keep filter-specific counts separate from the total distinct-item count. An acknowledged Todo stays visible as Todo. A previously acknowledged report with substantive new content becomes an unacknowledged update without resetting Todo/reminder state.

## 10. Harness integration and examples

Ship a concise `examples/heartbeat-prompt.md` instructing the harness to:

1. Start a run and consume its complete captured change range before acting on stale assumptions.
2. Inspect selected Watch sources using its existing tools, using cursors and revisiting linked open items where appropriate.
3. Read existing items/context before updating; use stable dedupe keys and expected content versions.
4. Publish one truthful result per selected Watch, including successful empty results or failure/partial coverage.
5. Submit supported Interest/Watch proposals when justified, rather than endlessly adding topics.
6. Finish with a summary and acknowledge only the consumed captured change range.

This is a run instruction, not an embedded autonomous framework. Source adapters remain in the harness. The application never parses credentials or assumes Codex has a particular Slack/Drive integration installed.

Codex supports noninteractive `codex exec` and reading a complete prompt from stdin with `codex exec -`. [Codex noninteractive mode](https://developers.openai.com/codex/noninteractive/)

A Unix invocation after configuring the harness is:

```sh
codex exec - < /absolute/path/to/control-plane/examples/heartbeat-prompt.md
```

A PowerShell counterpart is:

```powershell
Get-Content -Encoding UTF8 -LiteralPath 'C:\path\to\control-plane\examples\heartbeat-prompt.md' -Raw | codex exec -
```

The checked-in wrappers must set an explicit working directory, preserve the exit code, use configurable absolute binary/prompt paths where needed, and document log redirection. They must not auto-edit the user's scheduler or silently assume a noninteractive process inherits the interactive shell's PATH and tool settings.

`examples/scheduling.md` should describe scheduling these wrappers externally every two hours: cron/launchd on Unix-like systems and Task Scheduler on Windows. The server must be running independently. Report an unavailable source or noninteractive environment issue as a run failure, not as an empty successful scan.

Provide a deterministic fixture harness in `tools/fixture-harness`. It talks only to public CLI/API contracts, creates fixture Interests/Watches, publishes the first Slack/cost results, and publishes later source changes. It must support two explicit cycles with user actions in between and use an isolated data directory. It is test/demo infrastructure, not a production agent runtime.

## 11. Implementation sequence and exit criteria

Implement in thin working slices; run the relevant tests at each step.

| Slice | Build | Exit condition |
| --- | --- | --- |
| 1. Foundation | Go entrypoint, SQLite/migrations, HTTP client, settings, embedded SPA shell. | Fresh binary starts, persists a setting, reloads, and serves a deep link. |
| 2. First value | Item creation, Markdown report, source links, Todo/reminder actions, Attention/Library. | Publish a Slack fixture through CLI; click link, set Todo/reminder, restart, and verify persistence. |
| 3. External loop | Interests/Watches, brief, events, run lease, atomic publications/checkpoints. | Two fixture cycles preserve user edits and deduplicate results; no server-side agent execution exists. |
| 4. Adaptive/context layer | Proposals, context packets, history, thin MCP adapter. | Accept/deprecate a Watch; continue from context in a new session; run MCP contract tests. |
| 5. Delivery | Browser regression tests, external heartbeat examples, build scripts, release config, README. | All P0 scenarios pass and packaged binaries contain the complete UI. |

Do not stop after API scaffolding or placeholder UI. Do not expand an intermediate slice into a general workflow platform. When a choice is reversible and not specified, choose the smallest conventional implementation and record it briefly.

## 12. Verification plan

### 12.1 Automated tests

Backend unit tests cover report/action validation, reminder conversion and due evaluation, deduplication, content/state version separation, proposal transitions, due eligibility, and event acknowledgement boundaries. Inject time; do not sleep for reminders in tests.

SQLite integration tests use a temporary file-backed database and real migrations. Exercise transaction rollback, foreign keys, restart persistence, idempotent response replay, same-key/different-body conflicts, concurrent run starts, expired lease fencing, and Watch revision changes during a run.

The critical failure-injection test crashes/fails between logical publication steps: after restart, item updates and source checkpoints must either both exist or both be absent. Include a receipt replay after run completion to verify receipt lookup precedes lease checks.

HTTP/CLI/MCP tests assert consistent result/error contracts. MCP tests perform real initialize/list-tools/tool-call exchanges over the SDK transport. A small subprocess test verifies that stderr logging does not corrupt stdout MCP or CLI JSON.

Playwright tests exercise the product acceptance IDs, particularly source navigation, setting Todo, choosing/clearing reminders, Done/reopen, reload persistence, new report versions retaining state, stale Watch health, proposals, and fallback report rendering. Mock external source content, not the local application API. [Playwright](https://playwright.dev/docs/intro)

Shared valid and invalid JSON fixtures must be checked against the committed JSON schemas and both frontend/backend validators. Browser tests must demonstrate that clicking generated actions changes persisted state through the same route as ordinary controls.

### 12.2 Build and package checks

Provide documented targets/scripts that perform the equivalent of:

```sh
npm --prefix web ci
npm --prefix web run typecheck
npm --prefix web run test -- --run
npm --prefix web run build
go vet ./...
go test ./...
CGO_ENABLED=0 go build -trimpath -o dist/aicp ./cmd/aicp
npm --prefix web run test:e2e
```

The exact test scripts belong in `web/package.json`. Install Playwright's test browsers in CI/development only. Run `go test -race` in a separate supported CI job with the required toolchain; do not confuse that job's CGO requirements with the CGO-free release build.

Build the frontend before compiling the Go package that embeds it. A clean checkout intentionally has no generated `web/dist`; document and automate this dependency. Do not ship a placeholder `index.html` to make Go compilation pass. The release check must confirm real JS/CSS assets and an interactive portal are present.

P0 release targets: macOS arm64/amd64, Linux amd64/arm64, and Windows amd64. Perform native smoke tests on available Windows/Linux/macOS runners; clearly identify compile-only targets rather than claiming they were executed. At minimum test the Windows amd64 and macOS arm64 install flow when corresponding runners are available, and do not hide an unavailable test environment.

GoReleaser packages the prebuilt SPA and Go binary into archives with checksums. A snapshot build validates packaging without publishing a release. Add a tag-triggered/manual release workflow, but do not create a tag, publish a release, or alter external package repositories as a side effect of normal builds. [GoReleaser](https://goreleaser.com/getting-started/quick-start/)

### 12.3 Final handoff evidence

The implementation agent's final report must include the commit, commands executed and actual outcomes, the packaged artifact locations, P0 acceptance coverage, and exact limitations. A fixture demo passing is not a live Slack integration passing. Missing external credentials must not prevent completion and verification of the local application.

Required repository deliverables are implementation code, migrations, locked dependencies, JSON schemas, fixtures, automated tests, heartbeat/MCP examples, release configuration, and a README with install/run/configure/demo/build instructions. Keep the repository clean of runtime databases, source credentials, local reports, and generated build artifacts.

## 13. Reference notes and deferred decisions

The linked primary documentation was checked on 2026-09-13. It establishes library capabilities and current setup syntax, not proof that this application has already been implemented or tested. Verify selected version compatibility in CI and record any necessary change from this baseline.

Additional selected-library references: [Cobra](https://cobra.dev/), [MUI](https://mui.com/material-ui/getting-started/installation/), [Vite](https://vite.dev/guide/), [Go embed](https://pkg.go.dev/embed).

Defer SSE, HTTP MCP transport, FTS5, large artifact files, advanced outcome objects, Kanban/drag-and-drop, automatic source-to-Todo completion, recurring or OS reminders, service installers, and preauthorized adaptive scheduling. These are extension points, not incomplete parts of P0.

The defining end-to-end acceptance is: **external heartbeat -> useful source-backed report -> working link/Todo/reminder actions -> durable user changes -> next external run reconciles new information without losing local state.**
