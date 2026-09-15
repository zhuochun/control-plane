# aicp - Agent Control Plane — MVP Product Specification

- Date: 2026-09-13
- Status: Ready for implementation; P0 is the delivery boundary.
- Repository: `zhuochun/control-plane`
- Product/command name: `aicp`; the repository and spec filenames remain `control-plane`.
- Companion: [Technical decisions and implementation plan](20260913-agent-control-plane-technical-design.md)
- Intended reader: An agent implementing, testing, and packaging the product end-to-end.

## 1. Product decision

Build a local server, browser UI, and CLI that external agents use as durable shared state. An agent periodically reads what to inspect, checks the user's existing tools, and publishes useful findings, reports, follow-ups, and proposed changes to the user's interests. The user reviews these in a stable portal and makes small, immediate updates.

**Mode A only: the harness owns its heartbeat.** Codex, another harness, or an external OS scheduler starts the agent. This application neither starts agents nor schedules their execution. A Watch's interval describes when another inspection is due, not a promise that a runner has been scheduled.

**First useful experience:** an agent publishes a short report about a Slack thread; the user opens the original thread, marks the report as Todo, or chooses a reminder date; the next agent run sees these changes without needing the previous conversation.

This specification deliberately replaces the broader conceptual design with an implementable MVP. Do not reintroduce permission systems, a workflow engine, or a large ontology from earlier discussions.

## 2. Goals, boundaries, and success

The MVP must establish this loop:

```text
User expresses interests in their harness
  -> interests and watches persist locally
External heartbeat wakes an agent
  -> agent obtains a brief and prior context
  -> agent reads Slack / email / Docs / other existing tools
  -> agent publishes findings and reports
User reviews the portal
  -> opens sources / changes Todo / sets reminders / adjusts watches
Next heartbeat
  -> agent consumes local changes and observes changes in original systems
  -> agent updates existing items instead of creating duplicates
```

Success means the owner can use this loop for real work without maintaining a second task system. Useful findings, preserved user edits, understandable freshness, and fewer duplicate reminders matter more than the number of generated cards.

The server must work without an LLM key, a cloud account, Node.js, Docker, or an external database at runtime. The external harness and its source integrations are separate prerequisites for real-world monitoring.

Out of scope: accounts, authentication/RBAC, multi-user collaboration, cloud sync, connector credential management, native Slack/Jira/Docs integrations, outbound Slack replies or Jira mutations, internal agent execution, automatic OS notifications, email/push delivery, arbitrary generated code, full event sourcing, vector memory, dependency-graph scheduling, and a desktop shell.

## 3. P0 scope

| Capability | Required behavior |
| --- | --- |
| Local application | One downloadable executable serves the portal and provides CLI/MCP adapters. Data survives restarts. |
| Interests and Watches | Store why something matters, where to inspect, inspection instructions, an interval, and lifecycle state. |
| External run loop | Brief, run registration, incremental checkpoints, durable user changes, result publication, overlap protection, and run history. |
| Items | Source-backed notes, reports, tasks, and lightweight outcome updates in one consistent model. |
| Simple interaction | Open source, set/unset Todo, mark Done/reopen, acknowledge, and set/change/clear a reminder. |
| Generated reports | Markdown (including lists and tables) and optional typed actions rendered inside a fixed shell. |
| Adaptive attention | Agents propose creation, revision, or deprecation of Interests and Watches. Users accept or reject. |
| Context and handoff | Small persisted context notes, parent item references, source references, and external agent/session references in context text. |
| Delivery | Offline demo, automated tests, installation/build instructions, and an external-heartbeat example. |

P0 is intentionally not a drag-and-drop Kanban. Todo and Done filters provide the first work view. A Kanban projection may follow after the attention loop is useful.

## 4. Small domain model

**Structure only what the application operates on.** A structured field must support a concrete P0 query, relationship, constraint, state transition, or rendering action. Agents can interpret text: purpose, topic boundaries, thresholds, findings, evidence explanations, and handoff notes do not need separate schemas. Moving an unnecessary schema into JSON does not make it minimal. Preserve the operational fields needed for reliable persistence, reminders, deduplication, checkpoints, and concurrency.

### Interest

An Interest states **why the user cares**: title, one `instructions_md` document, and `active | paused | deprecated` state. The document describes purpose, relevant/excluded topics, and background in ordinary prose; no mandatory sections or separate topic fields.

Examples: merchant migration risk, infrastructure cost, or personal commitments. An Interest does not need an Outcome or a task hierarchy.

### Watch

A Watch states **where and how to inspect**. P0 uses one primary source per Watch to keep checkpoints and failures simple. Several Watches can share an Interest. A cost investigation may cite secondary sources without making them separate scheduled integrations.

Store a source kind and exact locator/query, one `instructions_md` document, interval, optional baseline lookback, lifecycle state, and the last successful source checkpoint. Instructions include context, thresholds, and how to inspect in ordinary prose. Initial default interval is two hours; initial baseline lookback is seven days, both editable.

The locator must identify the actual channel, folder, document, mailbox query, Epic, or generic source. The server does not resolve vague source names or fetch their contents; the harness does.

A source is configuration, not a server-side connector. No separate Sources product is required.

### Item

An Item is something worth retaining and showing. Its kind is `note | report | task | outcome`. It has a stable ID and deduplication key, title, summary, source references, generated content, optional context, and an optional parent item ID. A source reference only needs an item-local ID, an HTTP(S) URL, a label, and an observation time for navigation and freshness. Other source metadata, related item links, and external session IDs belong in free-form context text.

All four kinds also use the same Markdown report envelope; the word `report` names their shared content field, not a restriction to the report kind. Kind changes labeling/filtering and the initial Todo default, not the content schema.

All kinds share independent local interaction state:

- `todo_state`: `none | todo | done`;
- an optional one-time `remind_at` and its timezone;
- an acknowledged content version;
- a user note.

A task starts as Todo when first created. Other kinds start with no Todo state unless an explicit initial value is supplied. **Marking a report as Todo modifies that same Item; it does not create a duplicate task.**

An outcome is a lightweight item describing the intended result, latest evidence, and next check in its content/context. P0 does not calculate progress from completed tasks or implement an outcome metrics engine.

### Proposal

A Proposal suggests creating, updating, or deprecating one Interest or Watch. It contains the proposed minimal configuration, one rationale document with the reason and evidence links, and the target revision it was based on. State is `pending | accepted | rejected`.

Acceptance is an ordinary local configuration edit, not a permission framework. New background discoveries are proposals; explicit user instructions in the harness may directly create or edit configuration through the CLI.

### Run and Event

A Run records an external agent inspection attempt and its results. An Event records a durable content/configuration/user-state change. Events support history and the next brief; current tables remain the source of current state.

These are the core concepts. Do not create separate services for observations, insights, evidence, memory, artifacts, decisions, and agent teams. Use item content, source references, version history, context, and events until a real use case requires more.

## 5. Main user journeys

### 5.1 Install and configure

The user starts `aicp serve`, opens the portal with `aicp open`, and gives the harness instructions such as:

> Every two hours, inspect these two Slack channels, recently modified documents in this Drive folder, and infrastructure cost emails. Track my explicit commitments, delivery risks, and unusual cost changes.

The harness creates an Interest and the required Watches. The portal also offers simple forms to inspect and edit those records. It shows the concrete sources and inspection intervals before the first run.

The separate Preferences screen asks for the display timezone and defaults to the browser/system timezone. Persist an explicit IANA selection, while retaining browser-default mode until the user chooses one. The first adopter's expected setting is `Asia/Singapore`.

Installing MCP or setting a Watch interval must not display “monitoring is running.” Show “No agent run received yet” until a runner actually connects.

### 5.2 First and subsequent scans

The agent gets a brief, registers a run, reads the specified source window, and publishes useful items plus coverage metadata.

The first scan establishes a baseline. Historical promises are labeled as historical until their current state is checked; not every old mention becomes an overdue Todo.

Later scans use source cursors/versions where available. The agent also revisits linked open threads/items when needed: checking only newly created messages is insufficient for detecting replies to old threads. Unsupported source capabilities must be recorded as coverage limitations.

A repeat scan of the same Slack thread updates the existing item using its stable deduplication key. No-op results do not create new content versions or attention entries.

### 5.3 Slack report with actions

The portal shows a card such as:

```text
Migration decision needs your input
The thread asks you to confirm the rollout sequence.
Source: #merchant-platform, thread inspected at 10:04

[Open Slack thread] [Set Todo] [Remind me]
```

The report can contain more context, a table of options, and supporting source links. Opening the thread is browser navigation only. It does not imply the thread was read, the task was completed, or a reply was sent.

Set Todo is immediate and does not call a model. The card appears in Todos. Done marks the local follow-up complete and clears its active reminder. Reopen returns it to Todo without restoring an old reminder.

Remind me opens a fixed date/time form. Saving changes the same item's reminder and shows the chosen time. Actions remain available even when a generated report omits them or a report cannot render.

### 5.4 Reminder behavior

A reminder is **a local portal reminder**, not a scheduled agent run and not an OS notification.

The user may select a date or a date and time. A date-only selection defaults to 09:00 in the displayed timezone; the form shows this before saving. An explicitly chosen past time becomes due immediately, with a visible warning before save. Do not silently move it to tomorrow.

There is one active reminder per Item. Setting another replaces it; clearing removes it. A reminder does not automatically turn a note into Todo.

When due, the item appears in the Due reminders section based on server time. No heartbeat is needed. If the application/browser was closed, it appears when reopened. Acknowledging report content does not clear a reminder; the user clears/reschedules it or completes the Todo.

Deadline management, recurring reminders, snooze presets, calendar events, and push delivery are deferred. Do not invent a separate deadline field for P0.

### 5.5 Cost email analysis

A Watch instructs the harness to compare equivalent cost periods and investigate increases above a threshold. Example fixture: SGD 100,000 to SGD 118,000 is an 18% increase.

The harness checks period, currency, scope, actual versus forecast, and baseline comparability before publishing an analysis. The report distinguishes observed amounts, hypotheses, missing information, and suggested actions.

The Control Plane stores and displays this analysis; it does not implement a billing connector or a general threshold-analysis engine. The same anomaly is updated under the same key. A new reporting period can have a new key.

A useful report need not automatically be a task. The user can set Todo or a reminder when follow-up is needed.

### 5.6 User acts in the original system

The user opens Slack and answers the thread. A subsequent scan records the observed source status and evidence in the existing Item.

**For P0, source reconciliation updates report content, not existing local Todo/reminder state.** Write “Resolved in source” with the relevant evidence in the report and retain the standard Done action. Observed source status is prose, not a required enum, badge, or query field. Automatic closure of local Todos is deliberately deferred.

This distinction prevents a new report or an uncertain inferred reply from erasing a user's reminder. Conversely, local Done does not claim that a Jira issue was closed.

### 5.7 Interests evolve

The agent notices a repeated topic or a completed project and submits a proposal. The user sees the reason, source evidence, existing configuration, and proposed change, then accepts or rejects.

Creating a Watch, changing its interval/instructions, pausing it, or deprecating it must be reflected in the next brief. Deprecating an Interest makes its Watches ineligible without deleting their records; restoring the Interest restores only Watches that are themselves active.

Do not deprecate merely because a source has been quiet or inaccessible. Pending proposals for the same change are deduplicated. Rejected proposals are not resubmitted from the same evidence; genuinely new evidence can justify another proposal with an explanation.

One proposal targets one object in P0. A new Interest followed by a Watch may use two proposals across successive runs. Do not build a general configuration-plan engine.

### 5.8 Cross-session or cross-agent continuation

An agent writes context containing confirmed facts, failed attempts, unanswered questions, and a next step. It may create a child task with a parent item ID and record an external delegation/session ID in that context text.

Another agent reads the Item's context and sources by ID. The Control Plane preserves the handoff; the harness starts or coordinates that agent. “Delegated” must not be displayed as “running” without a reported run.

## 6. Portal structure

Use compact top navigation for the four top-level destinations; do not reserve a persistent sidebar for them:

| Page | Purpose |
| --- | --- |
| Attention | Metric-bearing outcomes first, then due reminders, open Todos, unacknowledged updates, and pending proposals; show each item once with relevant badges and direct actions. |
| Interests | Interests, nested Watches, simple editing, lifecycle actions, proposed changes, and source freshness. |
| Library | All items; filters for reports, tasks, outcomes, Todo/Done, Interest, and text; detail and history. |
| Activity | Recent runs, source coverage, failures, and user/agent changes. Preferences is a separate destination for display timezone and agent contexts. |

A stable item detail shell shows title, kind, current local state, reminder, sources, content timestamp, last successful inspection, generated body, user note, and history. Observed source status is explained in the generated body.

Default attention order: due reminders by scheduled time, explicit Todo items, other new findings, then pending proposals. An item can match multiple categories but must not multiply its count. Acknowledge hides ordinary update attention for that content version; it does not hide an open Todo or a due reminder.

Distinguish empty states: no Watches configured, never scanned, successfully scanned with no changes, a source failed, and the external runner is late. A successful empty scan advances coverage without manufacturing a report.

## 7. Generated UI contract

Agents generate data, not React or scripts. The v1 report envelope has `schema_version`, `body_md`, and optional typed `actions` (omission means none); the containing Item supplies its summary and sources. Markdown supports prose, lists, labeled values, and tables without separate block schemas. Do not require a block registry or a structured table model for P0.

Supported actions are:

| Action | Behavior |
| --- | --- |
| `open_link` | Open a referenced HTTP(S) source in a new tab. |
| `set_todo` | Set the containing item's Todo state to `none`, `todo`, or `done`. |
| `set_reminder` | Open the fixed reminder editor for the containing item. |
| `clear_reminder` | Clear its active reminder. |
| `acknowledge` | Acknowledge the displayed content version. |

Actions operate on the containing item only. Arbitrary endpoints, scripts, shell commands, cross-item mutations, custom forms, and Slack/Jira write actions are not part of this schema.

The action label is descriptive; the action type determines behavior. The renderer uses the same command handlers as ordinary UI controls. It must not maintain a second copy of Todo/reminder state inside report JSON.

If a stored report version is unsupported, show the fixed summary and sources with a clear fallback. New invalid envelopes are rejected on ingestion with actionable validation errors; do not silently drop invalid actions. Validation checks operational fields and types, not the organization or meaning of the prose.

## 8. Consistency rules

1. Agents publish content; local user interaction state is separately versioned and is not overwritten by ordinary ingestion after item creation.
2. All mutations use the server's shared command layer, including CLI and MCP. No adapter writes SQLite directly.
3. Stable keys deduplicate repeated findings. Content versions change only when the normalized content changes.
4. Source checkpoints advance only in the same successful commit as their published result. A failed scan does not advance coverage.
5. Human edits are durable changes delivered at least once to a subsequent run. Run completion cannot acknowledge edits made after its brief was captured.
6. A repeated request is idempotent. An outdated edit returns a conflict rather than silently overwriting newer state.
7. P0 permits only one active periodic run at a time; overlap is skipped or reported. A crashed run expires and does not block future heartbeats forever.
8. Source status is observed information, not independently verified truth. Retain the source reference and observation time.

## 9. Acceptance scenarios

| ID | Scenario and required result |
| --- | --- |
| P0-01 | Start an installed binary with an empty data directory; portal opens, migrations run, and no external runtime service is required. |
| P0-02 | Create an Interest and Slack Watch; next brief contains the exact source, instructions, interval, and baseline/cursor state. No agent is launched by the server. |
| P0-03 | Publish a Slack report; its source link opens the expected thread, without changing local status. |
| P0-04 | Set Todo, set a reminder, restart the server, and reopen the item; both state and timestamp survive. Republishing the report does not reset either. |
| P0-05 | A reminder becomes due with no agent heartbeat; it is visible on the next portal refresh. Test date-only 09:00, timezone conversion, clearing, and Done clearing it. |
| P0-06 | Repeat the same request and replay the same source finding in another run; one item remains, no duplicate reminder is created, and no-op content creates no version. |
| P0-07 | Edit a user note/Todo during a run; that edit is not overwritten or acknowledged prematurely and is visible in the following brief. |
| P0-08 | Source changes to resolved; the existing report and observed status update while local Todo/reminder state is preserved. |
| P0-09 | Accept/reject creation and deprecation proposals; changes persist, stale proposals conflict, and inactive Watches disappear from eligible work. |
| P0-10 | One Watch succeeds and another fails; only the successful checkpoint advances, coverage remains truthful, and the failed Watch is offered again. |
| P0-11 | Concurrent starts do not create two active runs; an expired run cannot publish late results, and a new run can proceed. |
| P0-12 | Obtain a context packet by item ID in a new session; it contains the parent and any external references, facts, source references, user changes, and next steps without old chat history. |
| P0-13 | The cost fixture produces a source-backed 18% analysis, while an incomparable-period fixture explicitly reports that comparison is unavailable. |
| P0-14 | Invalid report/action JSON receives a useful error. An unsupported stored report version falls back without losing sources or standard item controls. |
| P0-15 | The deterministic fixture runner completes two heartbeat cycles, including user actions between them, through public CLI/API interfaces rather than direct DB writes. |
| P0-16 | Create an Interest and Watch using free-form instructions and publish a Markdown report containing a table and source-status explanation. They remain readable in the next brief/detail without topic, threshold, external-status, or report-block schemas; ordinary Todo/reminder controls still work when generated actions are omitted. |
| P0-17 | Create one item of each kind with the same content shape; all render and support local actions, with only task defaulting to Todo. |
| P0-18 | Republish identical content with a later source observation time; freshness updates but content version, acknowledgement, local state, and content history do not change. |
| P0-19 | Edit parent Interest instructions during a run; the old run cannot publish or advance that Watch's checkpoint. The next run receives the revised instructions and due Watch. |
| P0-20 | Commit a partial Watch result; retrying the same request replays it, while replacing it within the same run conflicts. A later run can complete coverage without duplicating saved items. |

## 10. Delivery and iteration

Deliver working code, not just scaffolding. Include a fixture-backed demo, a reference heartbeat prompt, CLI examples, a Codex MCP setup example, tests covering the scenarios above, and build/release archives with checksums.

The offline demo is a mandatory end-to-end test, not a claim that Slack credentials exist. Attempt a real harness/source smoke test only when the environment provides it; otherwise document the precise untested integration boundary while completing the entire local product.

After dogfooding, consider SSE, a Kanban view, richer report presentation, large file artifacts, FTS5, recurring or OS reminders, preauthorized Watch adjustments, and OS service installation. None are prerequisites for P0.

The implementation agent should use the defaults in these two documents for reversible choices, record justified deviations in the repository, and finish build/test/package verification before declaring completion. Do not expand scope to solve speculative future problems.
