# Item-local delegation and agent follow-through

- Date: 2026-10-02; revised 2026-10-03.
- Status: Implemented, locally verified, and independently code-reviewed on
  2026-10-03. The owner accepted Item-local delegation records and direct executor
  access to the assigned Item. This revision replaces
  the earlier separate Task, Attempt, and Receipt design in this same document.
- Vocabulary: [glossary](../glossary.md).
- Preserves: [Watcher–Interest model](20260924-watcher-interest-model-spec.md)
  and [Run context and brief](20260930-run-context-and-brief-spec.md).

## Outcome and responsibility

When scanning discovers actionable work, the primary agent should investigate
or delegate it autonomously within existing authority. A new product spec should
lead to a feasibility assessment, evidence, and decisions for the person, without
requiring the person to first request that someone read or analyze it.

The primary agent chooses the work, executor, launch mechanism, result collection,
and repair. It may use internal subagents or external sessions. aicp keeps the
Item and the information needed to find and continue its delegated work. Agent
launching, communication, scheduling, and external tool access remain outside aicp.

The executing agent may read and update its assigned Item. It receives only the
context needed for its assignment; it does not need the global inbox, unrelated
Items, Watcher configuration, other Interests, or source Run context.

```text
Primary agent discovers work -> saves Item
  -> records pending assignment -> delegates through its own tools
  -> records returned external reference on Item
Executing agent -> reads assigned Item -> does work -> updates Item
Later primary agent -> finds Item needing follow-up -> reads result or retrieves it
  -> continues/requests repair through the recorded external reference
  -> updates Item with conclusions and human decision points when needed
```

These are agent activities, not mandatory server-enforced workflow stages.
There is no separate task engine, execution-attempt lifecycle, receipt object,
acceptance operation, or publication queue.

## Item-local delegation record

Add `delegations[]` to the existing Item content. Each entry is a small handoff
record, addressed by the pair `(item_id, delegation_id)`, not an independent
entity with its own lifecycle or revision. One Item may have several assignments.

| Field | Meaning |
| --- | --- |
| `id` | Stable Item-local identifier for the delegated objective; reused for follow-up and repair. |
| `executor` | Execution-system name and executing agent identity/label. |
| `external_ref` | Opaque external session/task reference, plus an optional navigation URL. |
| `instructions_md` | What to do, necessary inputs, expected output, and applicable authority/limits. |
| `status` | `pending` (primary-agent follow-up still needed), `closed` (no follow-up currently needed), or `blocked` (specific impediment). |
| `context_md` | Result/artifact location, progress, limitations, how to retrieve or resume externally, and next steps. |

The field names describe the proposed content contract. Reuse Item content
versioning, mutation retry identities, and existing event/history mechanisms.
Do not introduce another deduplication key, global task ID, or revision counter.
Repeated updates to the same delegation ID must not create duplicate entries.

Omitted delegation entries and fields retain their stored values. Updates merge
by delegation ID; an empty list is a no-op, not removal. There is no deletion
operation in this slice: finish or abandon follow-up using `closed`. Explicit
report/context corrections replace only supplied fields after current-version
reconciliation; omitted content is retained.

`pending` includes work running externally, work whose launch is uncertain, and
results waiting for the primary agent to read. It does not assert that an agent
is running. `closed` does not imply the person's Todo is Done or a subsequent
external action has been approved. Status is a follow-up marker, not a dispatch
state machine; progress and explanations remain prose.

A persisted handoff must let a later primary-agent session identify both who
received the work and where to retrieve or continue it. An agent label alone is
insufficient. If only a temporary handle exists, record that limitation and a
retained result location or alternate retrieval method. aicp cannot make an
expired session resumable. Before handing work off externally, the primary agent
persists the same pending delegation with its ID, intended executor, instructions,
and launch uncertainty. Until a reference is available, context explains the
planned mechanism and how to investigate an interrupted handoff. After launch,
the primary agent fills in the returned reference. This does not make launch and
Item updates atomic or require a separate prepared-attempt record.

## Agent interaction

The primary agent passes the Item ID, delegation ID, bounded instructions, and
needed evidence/tool context to the executor. It supplies Item read/update access
when available. Launching the executor never requires a human dispatch click.

An Item-aware executor reads the current assigned Item, performs the assignment,
and updates that same Item with results, evidence, limitations, and its delegation
context. Results may live in the report or link to an external artifact. It leaves
follow-up pending for the primary agent, or marks the assignment blocked with a
specific explanation. Result delivery and progress do not require a separate
receipt API. Updates must retain existing content outside the assignment and the
other delegation entries.

The executor's Item read/update path takes the explicit Item ID and needs no
brief, global list, Run start/finish, or Watcher findings submission. Work updates
must preserve existing Item identity, sources, provenance, and relevance without
forcing the executor to read unrelated configuration. Extra research evidence
can be added to the assigned Item without claiming new Watcher scan coverage.

The existing-Item work update changes report, context, delegation entries, and
adds evidence. The server loads and preserves identity, origin, Watcher provenance,
and Interest relevance. It cannot create an Item, change these ownership fields,
or advance source coverage. Existing Item creation and Run publication gates
remain intact; this is a distinct bounded update entry point.

The primary agent reads the Item and, when necessary, follows `external_ref` to
retrieve results or ask questions. An executor unable to update aicp can return
through its original mechanism; the primary agent writes the result to the Item.
Both paths use the same delegation record. Direct Item updates do not remove
external references needed for later repair.

The primary agent judges whether the output answers the assignment and whether
further work is useful. It can consolidate conclusions and close the delegation
in one Item update, retaining the result summary and artifact references needed
for later repair in delegation context. Checking quality is agent reasoning,
not a required server acceptance step. Receiving output alone need not close
follow-up.

For repair, reuse the Item and delegation ID, send feedback through the original
external reference, and set follow-up to pending. If the original executor is
unavailable, reassign using retained context. Record previous executor/reference
and the reason for transfer in delegation context/history so later readers can
find the original work. Reassignment does not imply the old executor stopped.
No new Attempt record is required for repair or reassignment.

## Continuation, conflicts, and failures

The primary agent needs a bounded, paged Item lookup filtered by delegation
status (`pending`, `blocked`), executor, or external reference. Results identify
the Item and matching delegation IDs; full Item reads provide details. Lookup
must not depend on Attention membership, Todo, acknowledgement, due Watchers, or
an active source Run. Closed assignments remain retrievable for later repair.

On a later invocation, the primary agent uses that lookup to resume relevant
unfinished work. aicp does not wake an agent or poll external systems. Work can
be followed up even when no Watcher is due, without a dummy source Run.

All Item content updates use the current expected content version and existing
request-ID retry behavior. On conflict, reread and merge; do not replace another
agent's or the person's newer work. Before updating, an executor checks that its
assignment/reference still applies. If reassigned or materially changed, it
records/reports its old result as such rather than replacing the current result.
Newer Item versions caused by unrelated content need not invalidate its research;
changes to the actual input or assignment require explicit reconciliation.

At handoff, retain the input Item content version and relevant source/instruction
snapshot or references in assignment context. Every delivered result identifies
the input version and requirements actually used, including same-executor repair.
The executor and primary agent compare that basis with current inputs before
merging conclusions. Reading the latest Item establishes mutation freshness,
not research freshness; no new Attempt or version counter is introduced.

If launch or result retrieval is uncertain, keep follow-up pending or blocked and
explain what is known and how to investigate. The primary agent reconciles using
its external tools before retrying. No server claim of exactly-once external
execution, automatic cancellation, or durable external-session recovery is made.
A failed Item update leaves the assignment available for later follow-up; agent
instructions require persisting results before declaring the handoff closed.

## Autonomous guidance and preserved boundaries

Put general behavior in the application's configured `AGENTS.md`, distinct from
the repository's contributor file. Keep assignment facts and external references
on the Item. The primary agent reads current applicable guidance; an executor
receives only the rules and constraints needed for its assignment.

> Within the authorized goal and scope, autonomously investigate, delegate,
> retrieve results, check evidence, and request repair. Persist the Item and
> delegation identifiers, executor, and continuation reference when handing off
> work. On later continuation, follow up unfinished assignments. An executor may
> read and update its assigned Item, preserving unrelated content and user state.
> Resolve questions through available tools and evidence before involving the
> person. Submit conclusions, limitations, a recommendation, and the exact
> decision required when human judgment or additional authority is needed.

This is an agent context/tool-scoping contract, not a new per-Item authentication
system. The delegating agent/harness supplies the bounded context and tools;
existing aicp access alone is not a claim of enforced Item-level isolation.
Credentials and external authority remain with the existing execution environment.

- A source Run and its checkpoints still describe inspection coverage. Research
  and delegation can continue after that Run finishes.
- Todo, reminders, acknowledgement, and user notes stay user-owned. Delegation
  updates and closure never change them implicitly.
- Attention remains its existing Item projection. Intermediate source findings
  can remain visible; no human action is needed to initiate autonomous work.
- Configuration Proposals retain their meaning. Findings and decisions use Item
  report/context rather than becoming configuration proposals.
- Already-authorized work needs no new approval step. Missing authority limits
  the dependent action while independently authorized investigation continues.
- Explicit later restrictions apply to ongoing work; an old assignment is not
  permission to ignore them.

## Product-spec example and acceptance

The primary agent discovers a new spec and saves an Item. It delegates feasibility
research, giving the executor that Item and its research scope. The executor reads
it, investigates with its tools, and updates the Item with supported options and
unresolved questions. The primary agent reads those results, asks for missing
analysis through the recorded session if useful, and consolidates a recommendation
with human decision points. Repair uses the same Item and delegation record.

- **ID-01 — Autonomous follow-through:** This example needs no human dispatch,
  collection, or repair step within existing authority.
- **ID-02 — Bounded executor context:** The executor can read/update its assigned
  Item without global inbox/configuration reads or starting a source Run.
- **ID-03 — Recoverable assignment:** Another primary-agent session finds pending
  or blocked delegations and locates the executor and external work, including
  after Item acknowledgement or when no Watcher is due.
- **ID-04 — Direct and indirect results:** Executor Item updates and results
  imported by the primary agent use the same record and preserve continuation refs.
- **ID-05 — Repair continuity:** Repair retains Item/delegation identity; transfers
  retain the previous executor/reference and do not erase original work.
- **ID-06 — Preserved content and state:** Concurrent updates reconcile Item
  versions; research never overwrites unrelated content or user-owned state.
- **ID-07 — Honest limitations:** Uncertain launches, expired sessions, partial
  work, and stale results remain explicit; recorded status does not fabricate
  execution, quality, authorization, or successful recovery.

## Scope and readiness

The concrete shared interfaces are:

| Interface | Contract |
| --- | --- |
| `PATCH /api/v1/items/{id}/work` | Existing-Item report/context replacement, evidence additions, and delegation patches, fenced by `expected_content_version` and optional `request_id`. |
| `aicp item work <id> --file <json>` | Calls the same work endpoint. |
| MCP `update_item_work` | `{item_id, update}` where `update` is the same work payload; no Run required. |
| Item list / MCP `list_items` | `delegation_status` accepts comma-separated pending/blocked/closed; `executor` and `external_ref` are exact matches on the same delegation entry. |

CLI list flags are `--delegation-status`, `--executor`, and `--external-ref`.
Filtered list results include `matching_delegation_ids` alongside Item content.
`executor` combines system and agent identity as caller-defined text; an optional
navigation URL can be retained in `context_md` with the opaque `external_ref`.
Source publication retains report/context on Items with delegation records;
subsequent work updates reconcile new source evidence with those conclusions.
Existing evidence IDs cannot be replaced through the work endpoint; additions
need new IDs and linked evidence metadata.

For example, before external dispatch:

```json
{
  "expected_content_version": 1,
  "request_id": "spec-feasibility-handoff",
  "delegations": [{
    "id": "feasibility",
    "executor": "codex:researcher",
    "instructions_md": "Assess spec v1 and retain evidence and decision points.",
    "status": "pending",
    "context_md": "Input Item v1 and source spec v1; launch not yet confirmed."
  }]
}
```

The change is limited to Item-local delegation content, Item read/update and
bounded lookup across HTTP/CLI/MCP, and agent guidance. Persist it using existing
Item ownership/versioning and repository migration conventions as needed. Old
Items have no delegation records; old Todos are not automatically delegated.
Source finding updates must preserve delegation records not explicitly changed,
and reconcile/preserve delegated research and consolidated conclusions in the
Item report/context. A scan cannot replace them merely because its finding
payload omitted them. Materially superseded conclusions may be revised with an
explanation while retained result references remain available for later repair.
Preserve owner-edited application
`AGENTS.md`; expose recommended guidance without overwriting it.

No separate Task, Attempt, Receipt, review/publish state machine, agent launcher,
scheduler, mandatory human queue, or new agent runtime is introduced. Portal
redesign and per-Item access-control infrastructure are outside this slice.

The owner authorized implementation, verification, and independent code review
on 2026-10-03, including these clarified update and input-attribution rules.
External mechanisms' actual retrieval/resume capabilities require live validation;
local contract checks cannot establish those capabilities. This document records
the accepted implementation scope and does not authorize a release.

## Implementation evidence

The shared Item work endpoint and delegation filters are implemented across HTTP,
CLI, and MCP. Schema version 8 upgrades untouched default agent guidance while
preserving owner-edited guidance; existing Item JSON needs no record conversion.

The [Luna smoke test](../perf/20261003-luna-delegation-smoke.md) exercised an
actual external thread, conflict recovery, filtered retrieval, and same-thread
rework. Schema version 9 adds practical handoff guidance based on that test:
autonomous defaults for reversible details, concise assignments, safe merge/retry,
and retained continuation instructions. Owner-edited guidance remains untouched.
The reusable [handoff example](../../examples/delegation-handoff.md) describes
the existing interfaces without introducing new workflow state.

- `scripts/verify.ps1` passed: locked web install, typecheck/build, pinned Go
  formatting/lint (0 issues), vet, all Go tests, executable build, 16 Playwright
  tests, and the two-cycle isolated fixture demo.
- Work-path tests cover direct source-Item updates, unchanged provenance/user
  state/checkpoints, stale versions, retry replay/conflict, merged delegation
  fields, empty-list retention, explicit corrections, same-entry filtering,
  restart/paged recovery, and untouched/owner-edited guidance upgrades.
- Independent code review identified a retained report-link validation ordering
  defect. Evidence now merges before final validation; a regression test submits
  only original scan sources after research adds a linked source. Focused tests
  and full verification passed, and independent fix review found no remaining
  actionable findings.

These are local application/adapter and fixture checks, not evidence that an
external executor can access sources, persist a session, or resume after expiry.
