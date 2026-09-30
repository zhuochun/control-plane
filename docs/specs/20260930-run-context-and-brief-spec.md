# Run context and read-only brief

- Date: 2026-09-30
- Status: Implemented, verified, and independently reviewed on 2026-09-30.
  The new contract replaces the former response.
- Replaces: `20260928-run-read-interface-followup-spec.md`.
- Vocabulary: [glossary](../glossary.md).
- Preserved behavior: [Watcher–Interest model](20260924-watcher-interest-model-spec.md).
- Evidence: [agent evaluation](20260928-agent-efficiency-evaluation-spec.md),
  [former baseline](../perf/20260928-agent-efficiency-baseline.md),
  [implementation results](../perf/20260930-run-context-baseline.md).

## Purpose and decision boundary

A source Run should supply what the agent needs to inspect its selected
Watchers correctly. It should not require rereading the user's entire Attention
backlog on every heartbeat. A brief should help a person or agent understand
current work without claiming an inspection attempt.

Attention means that an Item needs the user's attention. It does not by itself
instruct a background agent to execute a Todo, reopen an acknowledged matter,
or notify the user. Reading more Items is warranted by source evidence, a user
change, or an explicit user task, rather than membership in Attention alone.

The owner authorized implementation of this contract on 2026-09-30. Unchanged
Attention, including an already-set reminder becoming due, remains optional
for source Runs: reminders continue to resurface in the
portal, while brief exposes their count and preview. No notification or task
execution obligation is introduced or removed from the source coverage model.

## Former returns (before this change)

The following describes the superseded code, not the new response. The
primary owners are `internal/app/runs.go`, `internal/mcpserver/server.go`,
`cmd/aicp/main.go`, and `examples/heartbeat-prompt.md`.

| Surface | Current return | Current reading obligation |
| --- | --- | --- |
| `start_run` / `aicp run start` | `{run, brief}`. `run` holds identity, status, timestamps, and captured event boundaries. `brief` contains contexts, active Interests, selected Watchers, Attention, changes, and continuations. | Read both context files, all active Interest pages, all Attention pages, and all captured change pages. Every selected Watcher needs a terminal result. |
| Run Interests | All active Interests, including those unrelated to selected Watchers, with full configuration and instructions. | All pages required. |
| Selected Watchers | Captured ID, revision, source generation, matching policy, applicable Interest IDs/revisions, source, instructions, cursor, cadence, lookback, next due time, and up to 20 recent related Item summaries per Watcher. | Inspect the captured scopes. The related Items are recent examples, not a complete reconciliation index or an open-Todo-only set. |
| Attention | All Items satisfying the shared Attention predicate, including old open Todos and due reminders. Summaries include identity, kind, Interest reasons, `watch_id`/`parent_id`, title, summary, content version, Todo state, and reminder time; they do not include Item source references or links. | All compact summary pages required; full detail only when needed. |
| Changes | Events in `(after_seq, through_seq]`, filtered to `actor='user'` or `entity_type='proposal'`, in sequence order. This is not the entire audit event stream. | Consume the complete captured range before acknowledging it. |
| Fresh `get_brief` / `aicp brief` | Current contexts, all active Interests, due Watcher previews with related Items, Attention, event range and first change page, `now`, and operational health. | Tool guidance currently asks callers to follow requested collections to completion. |
| Brief continuation without a Run ID | An Interest or Attention page recomputed from current state at the cursor's offset. Context files are not repeated. | Changes between calls can cause duplicates or omissions; there is no immutable preview. |
| Continuation containing a Run ID | A page from that Run's stored Interest or Attention snapshot, even though it is read through `get_brief` / `brief --cursor`. | Fixed captured context, not a fresh live preview. |
| Run history list | Paged Run summaries, including selected Watcher snapshots and compact related Items. | Activity mainly uses identity, status, times, summary, and Watcher count. |
| Run detail | Run metadata, captured selected Watchers, and submitted results. | Used to inspect scope, coverage, and outcome; not equivalent to rereading today's configuration. |

The first Interest and Attention pages contain up to 50 entries. Continuations
contain up to 150, targeting 64 KiB of items; a single oversized entry is returned
alone. Attention title, summary, and each Interest reason have response limits
of 256, 512, and 256 UTF-8 bytes, with `truncated_fields`. Stored Run snapshots
and retry receipts retain the full captured text. Context files occur once.

This already avoids repeated context and unbounded single responses. It does
not avoid requiring the entire Attention backlog. In the corrected Mature
fixture, 2,000 summaries still cost 229,650 structured reference tokens. This
is synthetic payload accounting, not actual model usage or billed tokens.

## New Run: inspection context, not a global inbox review

The new response is `{run, context}`. The nested `brief` name is removed
because this is a claimed, immutable inspection context, not a live overview.
Field names below define the intended response contract; examples use symbolic
IDs and abbreviated record contents.

| Section | New return | Required? |
| --- | --- | --- |
| `run` | Existing identity, runner label, status, timestamps, `after_seq`, and `through_seq`. | Yes. |
| `context.contexts` | Captured `AGENTS.md` and `USER.md`, once. | Yes. |
| `context.watches` | Captured selected Watchers and their source-reading instructions/checkpoints, plus the latest attempt outcome, last successful coverage, and current retry limitation where applicable. No embedded recent Item lists. | All selected Watchers and their applicable limitations required. |
| `context.interests` | Full instructions for the union of Interest IDs/revisions captured by selected Watchers, once per Interest. | All pages required. A broad Watcher can legitimately require all active Interests. No selected Watchers means an empty set. |
| `context.changes` | Same filtered event range and chronological event contents as today. | All pages required for a claim of complete change handling. |
| `context.overview` | Captured global Attention count, count attributed to selected Watchers, due-reminder count, selected Watcher count, and `more_due_count`. Counts describe overlapping sets, not quantities to add together. | Read to understand scope; counts do not create extra source obligations. |
| `context.available.attention` | Count and a Run-bound cursor starting at the first captured global Attention entry. | Optional. Supports explicit broader review without enumerating IDs or summaries in the default packet. |
| `context.continuations` | Remaining pages for required Interests and changes, distinctly identified. | Follow required pages; optional Attention has a separate entry. |

```json
{
  "run": {"id": "RUN", "status": "running", "after_seq": 120, "through_seq": 150},
  "context": {
    "contexts": {"AGENTS.md": "...", "USER.md": "..."},
    "watches": [{"id": "GMAIL", "revision": 4, "interests": [{"id": "DELIVERY", "revision": 2}], "source": {"kind": "gmail", "locator": "..."}, "cursor": "..."}],
    "interests": [{"id": "DELIVERY", "revision": 2, "instructions_md": "..."}],
    "changes": [{"seq": 125, "actor": "user", "entity_type": "item", "entity_id": "ITEM", "change_type": "...", "payload": {}}],
    "overview": {"attention_count": 2000, "selected_watch_attention_count": 40, "due_reminder_count": 3, "selected_watch_count": 1, "more_due_count": 0},
    "available": {"attention": {"count": 2000, "cursor": "RUN_ATTENTION_FIRST_PAGE"}},
    "continuations": {}
  }
}
```

The example abbreviates Watcher and Run fields; omitted source generation,
instructions, timing, coverage, and event payload fields are not permission to
drop their semantics. A Watcher retains its captured source configuration;
latest outcome/coverage is captured at the same boundary. Do not splice in
later health data or report a superseded-source failure as a failure of the
current source generation. Where coverage is incompatible, label it historical.

### What makes an Item necessary to read

There is no default required Attention collection. Item reads are triggered by
the work being performed:

- A captured user change affects an Item: consume the event even if the Item is
  source-free, unrelated to selected Watchers, now Done, or no longer in
  Attention. Read full Item/context when the event is insufficient to interpret
  the change. An acknowledgement must not vanish because it removed Attention.
- New source evidence concerns a continuing matter: look up the existing Item
  before publishing a replacement or update. Exact dedupe-key lookup and bounded
  Watcher/text queries must be accessible through the relevant adapters. Current
  `ItemFilters`, HTTP, and CLI support dedupe-key filtering. MCP currently exposes
  Item reads by ID, but no Item lookup/list tool; adding bounded lookup is a
  prerequisite for this journey. Do not compensate by loading every Item.
- The user's explicit task asks for a broader review: read the optional captured
  Attention pages or query current Items as appropriate, with the distinction
  between historical and current state made explicit.

An unchanged open Todo, relevance to the same Interest, or a shared source URL
does not independently require a read. For example, a broad Gmail Watcher
matching `team-leadership` must read that Interest's instructions; it does not
therefore have to read every Slack Item labelled `team-leadership`.

Full Item reads remain current-state reads. Captured context identifies what
was known at Run start; an update uses the latest content version and existing
conflict rules. A current Item read cannot silently replace the Run's captured
Interest instructions. When a mutation conflicts, reread and reconcile rather
than overwriting user state or replaying stale content.

### Changes, failures, and finishing

The existing event acknowledgement model is retained. Do not narrow event
selection to selected Watchers or acknowledge past unread events. Reading an
event means incorporating its implications; if required context cannot be
retrieved, do not claim complete handling. A Run may finish without event
acknowledgement, leaving the range for a later Run. Failed Runs cannot acknowledge
changes; abandonment also leaves them unacknowledged.

Every selected Watcher still needs a truthful terminal success/partial/failed
result before finish. Partial coverage does not advance a source cursor. A
smaller packet must not create false success or hide why a prior attempt needs
retry. Failures on unselected Watchers are summarized in brief/health, not turned
into extra source work for this Run.

Reading optional Attention pages is not a finish prerequisite and does not
acknowledge Items. Conversely, skipping those pages is never grounds for saying
all user work was reviewed. Run summaries describe selected source coverage and
changes handled, not completion of the global Todo backlog.

## New brief: inexpensive, live, read-only orientation

Use brief for onboarding, deciding whether source work is due, checking progress
or failures, orienting an explicit configuration conversation, or obtaining a
small current Attention preview. Configuration editing uses configuration tools;
source inspection starts a Run. Brief does neither.

| Content | Old fresh brief | New fresh brief |
| --- | --- | --- |
| Time and consistency | `now`; live continuation behavior implicit. | `now` and `consistency: "live"`. Separate calls/pages may drift. |
| Setup and operation | Operational health, potentially large Watcher health lists. | Derived setup facts, active Run summary, latest Run summary, counts of due Watchers and unresolved latest failures. Full health available separately. |
| Focus map | Full active Interest records and instructions, paged. | Count plus up to 20 handles: ID, slug, title, revision; a continuation for more handles. Full instructions through configuration reads. |
| Due Watchers | Execution records with source, cursor, instructions, and recent related Items. | Count plus up to 20 handles: ID, slug, applicable Interest IDs, next due time, due reason, latest attempt status and limitation. No execution instructions, opaque source cursors, or related Item bodies. |
| Unresolved failures | Watcher health within the larger health payload. | Count plus up to 20 failure handles, with continuation: Watcher ID/slug/state, source generation, last attempt time, partial/failed status, and compact limitation. Includes not-due, paused, and expired Watchers. |
| Attention | First page plus continuations implicitly inviting complete consumption. | Exact count, due-reminder count, and at most 5 compact previews in existing Attention order. Explicitly labelled a sample; current Item listing supplies broader review. |
| Unhandled changes | Captured range and raw first event page. | Pending filtered-event count and `(after_seq, through_seq]` boundaries only. No acknowledgement and no raw event backlog in the default overview. |
| Global context files | Full `AGENTS.md` and `USER.md`. | Omitted. Available from existing settings/context reads when the task requires them. |
| Remaining work | `more_due_count` beyond the default Run selection limit. | Due count independent of a future Run's selection; paged handle lists are previews, not reservations. |

An unresolved failure here means the latest attempt for the current source
generation is partial or failed, without a later success for that generation.
A source replacement does not make an old generation's failure current. A
paused Watcher can remain unresolved without becoming eligible for inspection.
Failure handles are ordered by last attempt time descending then ID; long
limitation text is compacted with truncation indicators. Existing full health
is available at `GET /api/v1/status` / `aicp doctor`; MCP must also be able to
retrieve an omitted full limitation without starting a Run.

```json
{
  "now": "2026-09-30T01:00:00Z",
  "consistency": "live",
  "setup": {"user_context_set": true, "has_interest": true, "has_watcher": true, "done": true},
  "active_run": null,
  "last_run": {"id": "PREVIOUS_RUN", "status": "completed", "summary": "No new findings"},
  "focus": {"count": 21, "items": [{"id": "DELIVERY", "slug": "delivery-risk", "title": "Delivery risk", "revision": 2}], "next_cursor": "LIVE_FOCUS_NEXT_PAGE"},
  "due_watches": {"count": 1, "items": [{"id": "GMAIL", "slug": "gmail-inbox", "interest_ids": ["DELIVERY"], "next_due_at": "2026-09-30T00:00:00Z", "due_reason": "scheduled"}]},
  "unresolved_failures": {"count": 0, "items": []},
  "watcher_counts": {"paused": 1, "expired": 0},
  "attention": {"count": 2000, "due_reminder_count": 3, "sample": [{"id": "ITEM", "title": "Awaiting decision", "summary": "...", "todo_state": "todo"}], "sample_only": true},
  "pending_changes": {"count": 4, "after_seq": 120, "through_seq": 150}
}
```

The example abbreviates list entries; `focus.items` would contain the full
first page, and Attention previews use the existing compact Item shape. No
context files, Interest instructions, source cursor, or raw change payloads
are embedded in this overview.

The 20-handle and 5-Item limits are presentation defaults. Their exact
values are tunable without changing required Run context. They are not caps on
work eligibility or retained history. No caller must enumerate brief previews
before starting a Run. Within a single overview, counts, samples, and pending
event boundaries use one read boundary; cross-request consistency is not promised.

Due previews are ordered by next due time then immutable ID; focus handles by
creation time then ID. Brief exposes counts for paused/expired Watchers as
orientation but excludes them from due eligibility. Expiry, broad/explicit
matching, and source access remain governed by the existing model.

Live handle continuations identify their collection and remain read-only. They
do not create stored preview sessions or an expiry/cleanup subsystem. If records
change between pages, refresh or query the intended configuration directly.
Completeness-sensitive source inspection uses a captured Run, not a live brief.

`get_brief(cursor)` / `brief --cursor` may continue to read a Run-bound context
cursor for compatibility with the existing transport path. Such responses must
say `run_id`, collection, and `consistency: "captured"`; they never recalculate
that collection from live state. The fresh brief schema and the captured page
schema are distinct. A cursor for an unknown Run or invalid collection fails
explicitly rather than falling back to live state.

## History and retained state

Run history lists return identity, runner, status, timestamps, summary, selected
and submitted Watcher counts, and event boundaries. They omit selected Watcher
instructions, related Item lists, and captured Attention. Run detail retains
captured selected scope and results; required and optional context pages remain
accessible through `context.interests.cursor` and `context.attention.cursor` in
Run detail. The event range remains on `run` for `get_changes`. Counts come from
that Run, not current configuration.

This change is primarily about what callers receive and must read. It does not
delete full captured Interest/Attention text from existing Run storage, rewrite
history from current Items, or introduce another snapshot entity. Keeping the
current full snapshot initially also preserves optional historical Attention
reads. Therefore this spec makes no promise of eliminating global Attention
capture cost at Run start; storage/query reduction is a separate measured change.

Repeated `start_run` with the same request ID yields the same captured response
and continuation boundaries. Later edits and new events affect later Runs only.
Source-generation fencing, user-state preservation, explicit abandonment, and
the single active Run rule remain unchanged. If stored data needs adjustment,
use a versioned migration; runtime still serves one canonical shape.

## Representative outcomes and acceptance claims

- **RC-01 — Quiet cost:** Adding 10,000 unchanged open Todos does not add
  mandatory Item summaries or detail reads to an otherwise identical quiet Run.
  The captured overview changes; retained Attention and portal behavior do not.
- **RC-02 — Correct source reconciliation:** Evidence for an older Item outside
  the default packet reaches that existing Item and preserves identity and user
  state. Distinct matters sharing a source URL remain distinct.
- **RC-03 — Complete change handling:** A user note on an unrelated/source-free
  Item, an acknowledgement, and a configuration revision remain visible in the
  required event range. No skipped page may be acknowledged.
- **RC-04 — Captured applicability:** A focused Run omits unrelated Interest
  instructions; a broad Run includes every captured applicable Interest. Later
  edits cannot change either packet or its replay.
- **RC-05 — Honest limitations:** A selected Watcher's partial/failed attempt
  is visible without enumerating Attention; all selected Watchers still receive
  terminal results and unsuccessful coverage does not advance checkpoints.
- **RC-06 — Preview independence:** Brief works with an active Run, starts no
  Run, reserves no Watcher, advances no source cursor or event acknowledgement,
  and leaves all Item state unchanged. Live pages may drift; captured pages do not.
- **RC-07 — Explicit scope:** An old Todo or due reminder is discoverable from
  brief/current Item queries and optional captured Attention, but is not silently
  represented as handled because a source Run finished.
- **RC-08 — Bounded reads:** Required Interests and changes remain paged without
  silent omission. Optional summaries retain truncation indicators; callers read
  omitted text when needed. Counts/samples do not enumerate the entire backlog.
- **RC-09 — Compact history:** History list size does not scale with captured
  Item text per Run; detail still explains its historical scope and coverage.

## Affected consumers, scope, and readiness

The smallest coherent change spans the shared Run/brief projections, HTTP,
CLI, MCP descriptions and parameters, heartbeat prompt, packaged guidance, and
deterministic evaluation. The portal Activity consumes the compact history list;
Attention/Library retain their current user-facing inclusion and action rules.
The evaluation must compare identical source outcomes and user state under the
new required-reading contract, rather than treating an unread optional backlog
as either a failure or a free correctness win. Scripted token savings alone do
not establish real-agent reconciliation quality.

The `{run, brief}` to `{run, context}` change, brief fields, and history list
shape are public contract changes. HTTP, CLI, MCP, Activity, the heartbeat
performance client, packaged guidance, and deterministic evaluators are updated
together. There is one supported runtime contract after this update; external
consumers of the former schema must update with the executable. Version 7
migrates old Run-start retry receipts and the untouched seeded guidance before
requests are served. Full optional Run snapshots and owner-edited guidance are
retained. Earlier already-issued Interest cursors had offsets into the global
set; callers must restart required Interest reading from Run detail's new
context cursor after upgrading. Old Attention cursors retain their captured
collection and offset.

The owner accepted optional-Attention/reminder handling, new response shapes,
and coordinated consumer transition through the implementation instruction.
MCP `list_items` supports bounded exact-key, Watcher, Interest, text, and view
queries; `get_settings` and `get_status` expose omitted context/full health on
demand. The implementation results record executed verification, review, and
the limits of the synthetic agent evaluation.

No new planner, source index, automatic Todo executor, notification service,
cross-Item source grouping, retrospective Interest assessment, persistent brief
session, or discovery workflow is included.
