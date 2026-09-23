# aicp — Agent Experience and Heartbeat Contract Specification

- Date: 2026-09-16
- Status: Proposed; this document supersedes conflicting heartbeat, CLI, and
  MCP behavior in the companion specifications. Archive reactivation remains
  an explicit follow-up decision.
- Product: `aicp`
- Companion: [MVP product specification](20260913-agent-control-plane-product-spec.md)
- Technical companion: [MVP technical design](20260913-agent-control-plane-technical-design.md)
- Intended readers: agent-harness, MCP, CLI, portal, and test implementers.

## 1. Decision

aicp is the user's local control plane for managing Interests, Watches,
Attention, and agent runs. It is the durable place where the user expresses
what matters and reviews what the agent found.

Use the server to view and manage that control plane. Use one run to refresh
the agent's work:

```text
aicp serve                         # keep the control plane available to view

Heartbeat or explicit agent refresh
  -> starts the AI agent
      -> MCP start_run               # get the current work packet
      -> inspect due sources
      -> submit Watch findings        # or save an Interest-level Item
      -> MCP finish_run               # check out after all Watches are covered
```

`aicp serve` is long-lived. A user starts it when they want to open or use the
portal; it is not repeated for each heartbeat. A browser refresh is read-only
and must not create a run. When the external agent performs its scheduled
refresh, it calls `start_run`.

The scheduler starts the AI agent. The AI agent uses aicp. aicp stores durable
configuration, context, Items, checkpoints, user changes, and run history. It
does not start another agent, fetch source systems, or receive source
credentials.

The user-facing vocabulary is `aicp run start` and `aicp run finish`. The
agent-facing MCP names remain `start_run` and `finish_run`. There is at most one
active run globally.

This is a local, single-user MVP. It assumes that a started run completes
normally. Crash recovery, cross-run ownership, and replay after a lost
response are outside this heartbeat contract and must not become additional
agent steps.

## 2. Current behavior and change boundary

The checked-in heartbeat wrapper starts `codex exec`; the prompt asks the agent
to call `start_run`, read contexts and captured changes, inspect selected
Watches, submit one final result per Watch, and call `finish_run`.

The current implementation already provides the following useful boundaries:

- `start_run` selects active, due Watches and captures their revisions,
  cursors, and related Item IDs.
- a run result can atomically upsert Items and advance a Watch checkpoint.
- `finish_run` records the summary and computes the run status.
- settings persist `AGENTS.md` and `USER.md` and include them in brief/run
  responses.
- request IDs, content versions, Watch revisions, and event sequence ranges
  provide retry and stale-write protection.

The desired experience changes the work packet, naming, defaults, initialization
path, ID presentation, and completion validation. It does not remove the
transactional Watch-result boundary or the distinction between source access
owned by the harness and state owned by aicp.

## 3. Agent heartbeat journey

### 3.1 Initialization

An agent or harness setting up aicp uses `aicp init` or the equivalent setup
instructions in `aicp --help`. Initialization is idempotent and should:

- ensure the local aicp data/configuration is ready;
- seed the default `AGENTS.md` and `USER.md` text when they do not already
  exist;
- explain that the user should express what matters as an Interest, attach a
  source through a Watch, and add owner context in `USER.md`;
- show how to keep `aicp serve` running and register the MCP adapter; and
- show how an external scheduler invokes the AI heartbeat.

Initialization must not overwrite custom contexts, install an agent, create
Interests without user direction, or claim that monitoring is running. The
portal keeps its existing empty-state guidance rather than adding a separate
agentic onboarding flow.

Preferences provide independent **Reset to default** actions for `AGENTS.md`
and `USER.md`. Reset changes the editable draft first and only persists after
the user saves; it never silently overwrites custom text.

The default `AGENTS.md` must be operational guidance, not merely a description
of aicp. It explains the sequence below, source/tool ownership, stable dedupe
keys, reading existing Items before updates, truthful coverage, and the
requirement to finish only after every selected Watch has a terminal result.
The default `USER.md` explains that the user should record preferences,
priorities, background, and follow-through context there, without requiring a
rigid schema.

### 3.2 Check in and receive work

The scheduler starts the AI agent. The agent calls `start_run` once for the
heartbeat. It does not call `aicp serve`, and it does not need to call
`get_brief` first.

`start_run` atomically:

1. rejects a second active run;
2. selects active due Watches by default, up to the configured page bound;
3. captures the selected Watch and parent Interest revisions;
4. captures the not-yet-consumed event range; and
5. returns the run ID and one normalized work packet.

The work packet is a bounded snapshot. `start_run` returns the first page of
each potentially large collection together with opaque continuation cursors.
The cursors are bound to this run's snapshot; they do not require the agent to
construct or supply a run ID. Pages are bounded by configured record and byte
limits, and the server must never silently truncate a collection.

The complete logical work packet contains:

- `AGENTS.md` and `USER.md`, each exactly once;
- every active Interest once across the `interests` pages, including its
  instructions, title, lifecycle state, and ID;
- selected due Watches, each containing its own instructions, source locator,
  cursor, lookback/interval, parent `interest_id`, and compact related Item
  summaries including their IDs;
- compact summaries of all Attention Items not archived by the user across the
  `attention` pages, not only Items related to selected Watches;
- the captured change range and the changes in that range, with continuation
  paging when necessary; and
- run identity and status, but no lease information.

Interest instructions must not be copied into every Watch entry. The Watch
references its parent Interest; the normalized `interests` collection carries
the Interest instructions once. Related Item summaries should contain enough
to decide whether to read more, but not full report bodies. The agent uses
`get_item` or `get_context` when a full finding or history is needed. When an
`interests` or `attention` continuation cursor is present, the agent consumes
the remaining pages before finishing the run.

`get_brief` remains a read-only preview of the same work-packet concepts. It
does not claim a run. With a continuation cursor from `start_run`, it reads the
next page of that run's captured context snapshot. `start_run` is the
authoritative snapshot and adds the run identity needed for submissions and
completion.

There is no agent-facing lease, renewal, or crash-recovery workflow. The agent
does not receive a lease expiry and does not call `renew_run`.

### 3.3 Inspect sources

For each selected Watch, the agent uses its existing source tools. It follows
the Watch instructions together with the parent Interest instructions, uses
the captured source cursor and lookback, and reads relevant existing Items
before deciding whether to create or update a finding.

The agent must not infer that a source was inspected merely because a Watch was
selected. It must report successful, partial, failed, or empty coverage
truthfully.

### 3.4 Save findings

The heartbeat has two valid Item paths.

#### Watch-bound findings

For a selected Watch, the agent calls `submit_watch_findings` exactly once.
This is the renamed agent-facing form of `publish_watch_result` and retains
the existing atomic behavior:

- validate the run and captured Watch revision;
- upsert zero or more Items using stable dedupe keys and expected content
  versions;
- record coverage, source observations, limitations, and cursor movement; and
- advance the Watch checkpoint only for a successful result.

A successful no-change scan submits an empty Item list. A partial or failed
result records its error and limitations and does not advance the successful
source checkpoint. A Watch result is final for that run; retrying the identical
request with the same `request_id` replays its receipt, while replacing an
already committed result in the same run conflicts.

#### Interest-level findings

If the agent finds something relevant to an Interest but not specific to any
selected Watch, it may call `upsert_item` (or the equivalent `aicp item`
command). The Item carries the relevant `interest_id` and omits `watch_id`.
It uses the same stable dedupe-key and expected-content-version rules but does
not advance any Watch checkpoint and does not count as Watch coverage.

This path supports useful cross-source or newly discovered findings without
weakening the requirement that every selected Watch receives a final coverage
record.

### 3.5 Check out

The agent calls `finish_run` after saving all Interest-level findings and
submitting one terminal result for every selected Watch.

`finish_run` must reject completion when any selected Watch lacks a result. A
`success`, `partial`, or `failed` result counts as recorded coverage; an
unreported Watch does not. A run with zero selected Watches may finish without
Watch results.

The finish response records the summary and computed status. If the agent has
fully consumed the captured event range and durably reflected those changes,
it may acknowledge the run's captured `through_seq`. A failed run cannot
acknowledge that range.

## 4. User-facing command behavior

### 4.1 Commands

The normal control-plane lifecycle is:

```text
aicp init
aicp serve
aicp run start
aicp run finish
```

`aicp run start` needs no `start.json`, and `aicp run finish` needs no
`run-id` or `finish.json`. The server assigns request IDs when they are omitted,
uses a default manual runner label for the CLI, and resolves the one active run
for finish. Optional flags may provide a label, explicit Watch IDs, a summary,
or an acknowledgement boundary for manual/debugging use. Complex finding and
Item payloads may use a structured file through `run submit` or `item upsert`,
but start and finish never require file plumbing.

The normal scheduled heartbeat does not require a person to run the CLI; the
AI agent calls the MCP equivalents. The CLI remains useful for manual runs,
fixtures, and debugging.

The Watch submission command should use a clear `run submit`/`run findings`
name rather than making “publish” sound like public distribution. `run publish`
is not part of the supported contract; an implementation may keep an
unadvertised temporary alias only while in-repository callers migrate, and it
must be removed before the implementation is complete.

Interest-level findings have a single `item upsert` concept for creating a new
Item or updating an existing Item. It does not silently claim Watch coverage.

### 4.2 Sensible list defaults

`interest list` and `watch list` show active records by default. An explicit
state filter or `--all` is required to inspect paused and deprecated
configuration. Direct `get` by ID remains able to retrieve inactive records.

Run selection already uses active Interests and active Watches; normal start
does not include paused or deprecated configuration.

### 4.3 Short IDs

Full UUIDs remain canonical in storage, HTTP/MCP machine payloads, JSON output,
links, dedupe references, and receipts. Human-facing UI and non-JSON CLI
output display only the first UUID section.

Human-facing CLI commands accept a unique UUID prefix. Resolution rules are:

- zero matches: return not found;
- one match: use the canonical full ID;
- multiple matches: return an ambiguity error listing the short candidates;
- JSON output: retain full IDs for reliable automation.

MCP calls use canonical IDs supplied in the work packet or previous tool
responses; they do not guess among prefixes.

## 5. Agent-facing MCP contract

The following descriptions are the canonical language shown to an agent. Tool
inputs use full IDs when an ID is supplied. `request_id` is optional for normal
local calls; callers that need replay supply the same request ID, while the
server assigns one when it is omitted. The one-active-run rule lets normal
calls omit `run_id`. Responses use the shared HTTP/domain contracts rather than
introducing a second data model.

| Tool | Description and behavioral contract |
| --- | --- |
| `get_brief` | Preview the current active Interests, due Watches, unarchived Attention summaries, pending changes, contexts, and health. Read-only; does not claim a run. An opaque continuation cursor from `start_run` reads the next bounded page of that run's captured context snapshot. |
| `start_run` | Check in for one heartbeat, claim the single active run slot, and receive the authoritative normalized work packet. Normal use supplies no file, run ID, or explicit Watch IDs; active due Watches are selected. A second active run returns a conflict. No lease data is required in the response. Large Interest and Attention collections return continuation cursors rather than being silently truncated. |
| `get_changes` | Read pages from the exact captured event range returned by `start_run`. Consume every page before relying on older assumptions. |
| `get_item` | Read one complete Item, including current content, source references, versions, and independent user state before updating it. |
| `get_context` | Read bounded recent versions, notes, and related context for one Item when a summary is insufficient for reconciliation. |
| `submit_watch_findings` | Submit one final success, partial, failed, or empty result for one selected Watch. The active run is inferred when `run_id` is omitted. Atomically save its Items, coverage, limitations, and successful checkpoint. |
| `upsert_item` | Create or update an Interest-level Item that is not specific to a selected Watch. Use a stable dedupe key and expected content version; do not advance Watch coverage. |
| `finish_run` | Check out the sole active run after every selected Watch has a terminal coverage record. Store an optional summary and optionally acknowledge the exact captured change range. |
| `propose_change` | Suggest creation, revision, or deprecation of one Interest or Watch for human review. Never apply configuration changes implicitly. |
| `apply_item_action` | Apply an explicitly authorized Todo, reminder, or acknowledgement action with state-version fencing. Archive is deferred with the archive-dependent slice; it is not part of this tool contract yet. |

`renew_run` is not part of the canonical heartbeat contract and is not part of
the final supported MCP surface.

Mutations with an explicit `request_id` remain idempotent. Stale
Interest/Watch revisions, stale Item content/state versions, missing Watch
coverage, duplicate Watch results, and a second active run must produce
explicit conflicts rather than silent overwrites. Replay after a lost response
is not a required local-MVP behavior when the caller omitted `request_id`.

## 6. Attention and archive boundary

The heartbeat packet includes Attention summaries for every Item that the user
has not archived, regardless of which Watch produced it. It must not filter
Attention down to the selected Watches.

Archiving is a user-controlled visibility state, not deletion. An archived
Item remains available in Library/history and can be read by ID, but is omitted
from Attention and future default heartbeat context. Archive must not silently
change the Item's content, Todo state, reminder, or source history.

The current implementation has acknowledgement, Todo, Done, reminders, and
notes but no distinct archive state or archive action. Until the archive-
dependent slice is decided and implemented, the current Attention projection
is the complete input to the heartbeat; there is no archive field to preserve
or mutate. Whether new substantive content on an archived Item automatically
reopens Attention is unresolved and must be decided before implementing that
slice. It must not be silently equated with acknowledgement or Done.

## 7. Default agent contexts

The default `AGENTS.md` should communicate this concise operating contract:

```markdown
# Working with aicp

You are an external inspection agent. aicp stores Interests, Watches, Items,
user changes, and source checkpoints; it does not fetch sources or start an
agent for you.

For each heartbeat, start one run, read all returned contexts and captured
changes, consume any continuation pages, inspect only the selected due Watches,
read existing Items before updating them, submit one truthful final result for
every selected Watch, save any Interest-level findings separately, and finish
only after all Watch coverage is recorded. Use stable dedupe keys and preserve
user Todo, reminder, acknowledgement, and note state. Do not invent or mutate
archive state; that slice is not implemented yet.
```

The default `USER.md` should remain a short invitation for the owner to state
what matters, preferred evidence and tone, constraints, priorities, and
follow-through context. Both are ordinary Markdown; agents interpret them as
guidance and aicp never executes them.

## 8. Acceptance claims

- **CHG-heartbeat-flow** — One scheduled heartbeat starts an external AI agent,
  which checks in, inspects, submits findings, and checks out; aicp never
  launches the agent.
- **CHG-one-active-run** — A second `start_run` cannot create an overlapping
  active run.
- **CHG-normalized-packet** — `start_run` returns each active Interest once,
  due Watches with non-duplicated parent references, all unarchived Attention
  summaries through bounded continuation pages, captured changes, contexts,
  and run identity without requiring lease management.
- **CHG-bounded-context** — Large Interest and Attention collections are
  delivered through bounded, run-consistent continuation pages. No page
  duplicates Interest instructions and no collection is silently truncated.
- **CHG-watch-coverage** — `finish_run` rejects a run with any selected Watch
  lacking a terminal result; failed and partial results count as coverage.
- **CHG-interest-item-path** — An Interest-level Item can be created or
  updated without a Watch and does not advance Watch coverage.
- **CHG-atomic-watch-submit** — A Watch submission preserves the existing
  atomic Item/checkpoint behavior and replays an identical retry with the same
  `request_id`.
- **CHG-init-guidance** — `aicp init` is idempotent, seeds missing defaults,
  explains the basic setup, and never overwrites custom contexts.
- **CHG-list-defaults** — Interest and Watch list commands show active records
  by default and require an explicit request for inactive records.
- **CHG-short-identifiers** — Human UI/CLI uses short UUIDs and resolves unique
  prefixes while machine payloads retain canonical full IDs.
- **CHG-mcp-language** — MCP descriptions explain agent intent and effects in
  heartbeat language; `submit_watch_findings` is the canonical replacement
  for `publish_watch_result`.
- **CHG-simplify-prune** — Once the new workflow is adopted, remove obsolete
  mandatory start/finish request-file plumbing, positional run-ID requirements
  in the normal CLI/MCP path, `renew_run`, and the old `run publish`/MCP naming
  aliases. Complex finding/item payload files may remain only for the
  explicitly supported `run submit`/`item upsert` escape hatch. Remove dead
  wrappers and documentation that only describes superseded commands. Remove
  tests that assert only removed interfaces, while retaining coverage for their
  replacement invariants: one active run, explicit-request idempotency,
  stale-write fencing, atomic Watch submission, complete Watch coverage,
  sensible list defaults, and unique-prefix errors.

## 9. Scope and non-goals

This specification covers the agent experience, command vocabulary, MCP
contract, context initialization, list/ID defaults, Attention input, and run
completion semantics.

It does not authorize implementation, choose a source connector, add a server
side scheduler, make aicp an LLM runtime, add arbitrary HTML/chart rendering,
delete historical records, or create a general workflow engine.

For the heartbeat slice, this document supersedes conflicting provisions in the
companion specifications about run lifecycle, lease renewal, CLI request files,
MCP tool names, work-packet shape, and completion coverage. The existing
product and technical specifications remain authoritative for source cursors,
content versioning, proposal state, SQLite persistence, security boundaries,
and the external harness model.

## 10. Readiness and follow-up

The heartbeat vocabulary, bounded normalized packet, dual Item paths,
active-only defaults, initialization direction, short-ID behavior, single-run
rule, simplified final interface, and MCP tool direction are ready for
implementation planning.

The archive-dependent slice is not implementation-ready until the owner
decides whether new substantive content automatically reopens an archived
Item. That decision affects Item state transitions, Attention projection,
agent context, portal actions, and tests. The remaining changes can be
implemented independently if archive behavior is kept out of the first slice.

## 11. Simplification and cleanup at the end

The implementation must finish with one authoritative workflow. The supported
final CLI/MCP path has no start/finish request files, no user-supplied run ID in
the normal path, no `renew_run`, and no `publish_watch_result`/`run publish`
names. After the new path and its replacement tests are green, and the
reference heartbeat prompt, CLI help, MCP tool list, and contract fixtures
expose only the final names:

- remove compatibility aliases and command helpers for the superseded names;
- remove `start.json`/`finish.json` flags, examples, fixtures, and tests;
- retain structured payload support only for the explicitly supported complex
  `run submit`/`item upsert` path; and
- prune tests that encode deleted behavior, not tests that happen to fail
  during migration.

Keep tests for durable behavior that survives the simplification. A smaller
interface is not permission to reduce coverage of idempotency, concurrency,
revision conflicts, atomicity, failed/partial coverage, or user-state
preservation.
