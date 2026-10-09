# User notes and inbox processing

- Date: 2026-10-08
- Revised: 2026-10-09 following independent review and owner decisions.
- Status: Implemented, locally verified, and independently code-reviewed on 2026-10-09.
- Authority: The owner requested a specification for unread Item notes, preserved
  prior notes, Run completion enforcement, and processing of Add to Inbox Items.
  The owner subsequently authorized implementation, software-system design,
  verification, independent code review, and obsolete-code cleanup on 2026-10-09.
- Related contracts: [Run context](20260930-run-context-and-brief-spec.md),
  [Watcher–Interest model](20260924-watcher-interest-model-spec.md),
  [Item delegation](20261002-agent-delegation-and-receipts-spec.md), and
  [review answers](20261004-item-review-formats-and-user-answers-spec.md).
- Vocabulary: [glossary](../glossary.md).

## Outcome and scope

When a person saves a note on an Item or submits Add to Inbox, the input must
remain discoverable to agents until explicitly handled. Successful processing
preserves the exact submitted text on the Item and records the response or
durable follow-up. A Run cannot report successful completion while captured
input remains unaccounted for.

This contract covers two entry points: the editable Item note and Add to Inbox.
It introduces one shared processing obligation on the existing Item, not a new
Task entity, global Todo execution queue, scheduler, or agent launcher.
Structured review answers retain their existing immutable storage and
applicability contract; automatic processing receipts for answers are outside
this slice. Ordinary acknowledgements, Todo changes, and reminders do not create
new input obligations.

Owner decisions on 2026-10-09: inbox input may be captured information,
instructions for Watchers/Interests, or a reference update; an explicit operation
may update or archive the processed inbox capture. Agents interpret related
input in a chronological batch, without a domain conflict-resolution engine.
Failures must appear as Item updates so the person can add guidance for follow-up.
The later request to implement this specification supplies implementation
authority for the complete slice, including its stated legacy backfill policy.
This does not authorize release or changing a running personal installation.

## Behavior before implementation and evidence limits

| Surface | Confirmed current behavior | Missing guarantee |
| --- | --- | --- |
| Item note | `SetUserNote` replaces `user_note`, fences on state version, and emits `item.note_updated` with a state version. | No immutable note revision history or processing receipt; a note change alone does not satisfy the Attention predicate. |
| Add to Inbox | Creates a user-origin `note` Item with the complete submission in the report body and no Todo by default. The UI promises review in the next Run. | No durable distinction between awaiting processing and previously processed. |
| Run context | Captures a user change range. Global Attention is optional; captured Attention entries omit note text and report bodies. | Change acknowledgement does not establish full input reading or handling. |
| Finish Run | Requires terminal coverage results for selected Watchers; derives status from those results. | No captured note/inbox processing requirement. |
| Item history | Stores versioned agent content; freeform notes live separately in user-owned state. | Agent content history is not a substitute for authoritative prior notes. |

Baseline: commit `21d05dbc791b96c9113d53cc34da5cd09aa5d040` before this slice.
Implementation owners: [note saving](../../internal/app/item_actions.go),
[Item model and Attention](../../internal/app/items.go),
[Run lifecycle](../../internal/app/runs.go),
[inspection context](../../internal/app/run_context.go), and
[inbox capture](../../web/src/workspace.tsx).
These are code-inspected behaviors, not a reproduction of the reported agent
session. The server can require an explicit handling record; it cannot prove
that a model understood the text.

This specification extends the Run-context contract for these explicit user
inputs. Unchanged Attention remains optional. Existing user changes outside
this scope continue to use their existing reading contract.

## Shared input identity and history

Each explicit submission has stable Item-local identity and an immutable text
revision. Record its kind (`note` or `inbox`), Item ID, full text, submission
time, and lifecycle state. Processing records identify the exact submission,
Run, runner, outcome, time, and response or saved follow-up. These are semantic
requirements, not a mandated database schema or transport format.

The original text remains user-owned. Agents may append processing results but
must not rewrite, summarize away, delete, or fabricate the submission. Prior
notes are authoritative Item-local records, available through Item detail and
paged history. Copying a paraphrase into `context_md` does not satisfy retention.

For Add to Inbox, preserve the entered information and supplied optional title
and source date. Process the same Item, retaining its identity and user origin.
An explicit input-processing operation may update its report or relevance;
the original submission remains separately retrievable even if the report body
changes. Creating a replacement Item and leaving the original unhandled is not
successful processing.

### Inbox processing operation

Provide an equivalent explicit operation through HTTP, CLI, and MCP to record
handling of captured input. Its contract includes the exact input revision,
active Run, request ID, expected Item versions, visible result, and whether to
retain/update the capture or archive it from the active inbox. It may change
same-Item report, context, and Interest relevance with supported evidence/reasons,
while preserving user origin, Watcher provenance, identity, and user-owned state.
This is a new bounded permission; do not achieve it by impersonating the user
through whole-Item replacement or broadening ordinary `update_item_work`.

Inbox interpretation determines the result:

- Captured information: incorporate it into the Item or save its reference.
- Explicit Watcher/Interest instructions: use the owning configuration commands
  within the user's authority, then record the affected IDs/revisions and result.
- Reference updates: record what was updated and a durable reference to it.

These outcomes may retain an updated Item or archive its inbox capture once
handled. Archiving removes the capture from the active inbox, not the durable
Item, its original submission, history, or ability to add a note. It must not
suppress an open Todo, due reminder, or other existing user Attention obligation.
A new note on an archived capture makes its pending input discoverable again;
the person can reach it from history and the agent from the required input list.
Unresolved failure is not eligible for successful-processing archive.

Configuration/reference changes owned elsewhere need not share the handling
transaction. Persist their receipts or verifiable result references before
recording successful handling. On retry, inspect those results and reuse their
existing idempotency contracts rather than applying the changes twice. An
uncertain or partly applied change must be reported honestly on the Item; do
not roll it back implicitly or claim success. Configuration changed during a
Run affects later source Runs, preserving the current Run's captured basis.

### Saving, revising, and withdrawing notes

- A nonempty saved note creates pending input. Saving unchanged text is a no-op
  for processing identity; replaying the same request cannot create another input.
- The current note remains editable until processed. An explicit save of changed
  text creates a new revision and supersedes the previous unprocessed revision.
  Keep the old exact text and its superseded disposition in prior-note history.
  The latest revision replaces the earlier instruction; agents must not execute
  a superseded request merely because it was previously captured.
- Explicitly clearing an unprocessed note withdraws that revision. Preserve the
  text and withdrawal in history; no empty processing obligation is created.
- After processing, the current note field is cleared by the successful
  processing operation. A later nonempty save creates new pending input. Prior
  processed notes are immutable; corrections are new input.
- Whitespace-only text is empty for submission eligibility. Do not apply lossy
  trimming or normalization to the retained text beyond the entry point's
  declared input normalization. Existing size limits continue to apply.

## Processing lifecycle

| State or outcome | Meaning | Intake remains pending? |
| --- | --- | --- |
| Pending | No durable handling result exists for this exact input. | Yes |
| Processed: responded | A supported response is saved on the Item. | No |
| Processed: incorporated | Information is saved in Item context, with a reason that no further action is needed. | No |
| Processed: follow-up recorded | A durable next step or Item-local delegation and continuation context are saved. | No; underlying work remains outstanding |
| Blocked or failed attempt | The agent cannot safely interpret or handle the input; a visible Item update records the limitation and next step. | Yes |
| Superseded or withdrawn by user | The person replaced or removed the unprocessed note. | No for that revision; any replacement remains pending |

Reading and processing are separate. `get_item`, listing, opening the reader,
and reading history have no processing side effects. A generic “read” or
“changes acknowledged” flag cannot clear input.

To record processed input, the agent must read its full captured text and the
current Item, reconcile later user changes, and persist a meaningful outcome.
The handling record must explain what was done and where the response or
follow-up is stored. A follow-up reference must identify durable existing work;
a promise to do something later without saved continuation is insufficient.

Long-running work does not need to finish in this Run. A recorded delegation
can complete intake only when its instructions and available continuation
context are durable and the Item visibly retains outstanding work. Executor
delivery remains subject to primary-agent judgment under the delegation contract.

Processing does not grant new authority. An ambiguous or unauthorized request
requires a saved clarification or authority request and a visible follow-up.
That can count as handled intake; it does not authorize the requested external
action. A tool failure with no usable response or follow-up counts as failed
processing and remains pending. Publish a visible Item update describing what
was attempted, what failed, any changes already applied, and the next useful
step or guidance needed. A Run-only error or hidden attempt receipt is not
sufficient. Preserve the original input and let the person add a new note to
guide the next attempt; read that guidance alongside the earlier failure.

If publication itself fails, do not clear input or claim the visible update
exists. Return the publication error and retain pending work for recovery.

### Atomicity, conflicts, and retries

Saving the handling result, preserving original input, and transitioning its
intake state must be one atomic application operation. Failure leaves the input
pending and must not silently clear the note. Existing content-version checks
apply when agent content changes; processing also checks the exact input
identity/revision. Unrelated user state must be preserved.

Every new handling or blocked/failed-attempt mutation must name a running Run
that captured that exact input revision. Reject mutations for closed Runs and
uncaptured inputs; processing outside a Run is not part of this slice. Run
membership, lifecycle, revision checks, and writes share the same transaction
so Finish or abandonment cannot race with a late processing write. An identical
request-ID replay returns its original committed receipt even after Run closure,
without performing another write or changing settled accounting.

Clearing the current note must advance the Item's existing user state version
and state-update time. User note saves retain their existing expected-state-
version check. A browser editor loaded before processing cannot silently restore
the processed text with its old version: it receives a recoverable conflict and
retains the draft for an explicit decision after rereading. The durable processing
transition emits an event and updates Item, history, and pending-count read
results; it is agent handling, not a new user submission event. Failed attempts
likewise expose their recorded limitation without claiming the note was cleared.

If a person saves note B while an agent handles captured note A, A cannot clear
B. A stale processing mutation fails with a recoverable conflict. The agent
rereads and reconciles; A may now be accounted for as superseded, while B remains
pending for the next Run. Archiving must not increment the Item content version
solely to mimic a source finding; keep processing and content changes distinct.

Identical uncertain retries use the same request ID and produce one receipt,
one archive transition, and one logical response. Changed payloads after a
conflict use a new request ID. A later Run must not duplicate a response for an
already processed input after a crash between processing and Run completion.

## Required Run input

At Run start, atomically capture the pending supported inputs alongside the
existing context and event boundary. Capture immutable input IDs/revisions and
full submission text, not only Item summaries or live note pointers. Selection
does not depend on Attention, Todo, acknowledgement, Watcher association,
Interest association, or the acknowledged event cursor.

Required input is paged with a Run-bound continuation. The default packet must
identify its count, first page, and required continuation independently of
optional Attention. Full text may be retrieved through a captured-input detail
read when necessary for payload bounds; every selected input must have a
discoverable complete read path. No silent truncation, sampling, or implicit
overflow into a later Run is allowed in this first slice.

Return captured inputs in submission-time order, with stable input identity as
the tie-breaker. Before acting, the agent consumes the complete captured batch
and reads related notes and prior handling results on affected Items, including
relevant paged history. Corrections/cancellations can affect an earlier inbox
request even after the correcting note has been cleared into Prior notes.
The agent judges meaning and precedence from the batch and current context;
timestamps aid interpretation but do not mechanically resolve contradictions.
If necessary, publish a question or limitation instead of guessing. No semantic
conflict engine or mandatory extra human question is introduced.

The agent must read each full input and record an outcome for each. A Run with
no selected Watchers still captures and
processes pending notes/inbox input. Live brief and status expose pending input
counts so an external agent can discover work even when no Watcher is due;
neither starts an agent or claims processing occurred.

Inputs submitted after Run start belong to the next Run. A replacement note
submitted after the boundary must not become mandatory work for the current
Run or be cleared by it. Run history retains what was captured and the actual
handling or user-withdrawal disposition, even when current Item state changes.

Outstanding input is durable across event-range acknowledgement. A failed
attempt is selected again in the next Run, even if its original creation event
was already acknowledged. No source cursor or synthetic Watcher is used to
represent this local work.

## Run completion and recovery

Finish requires a terminal accounting for every selected Watcher and every
captured input. For input, valid accounting is a durable processed receipt, a
recorded blocked/failed attempt for this Run, or a verified owner supersession
or withdrawal. An omitted input rejects Finish with a recoverable conflict
identifying the missing inputs. Finish never clears input automatically.

Keep existing Watcher status rules and add a separate input outcome:

Attempt history and effective disposition are distinct. Repeated attempts are
permitted while the Run remains active. A failed attempt followed by successful
handling has an effective processed disposition; keep the failure in history
without downgrading the recovered input. Supersession/withdrawal is accounted
from current owner state. Otherwise the latest committed unresolved attempt
determines blocked/failed disposition. An already processed revision cannot
receive a new failure that reopens it; a new user submission is new input.
Use effective dispositions for status/counts and separate historical attempt
counts in the summary.

1. Watcher outcome is `completed` when none were selected or all succeeded;
   `partial` when at least one succeeded or was partial but not all succeeded;
   otherwise `failed`.
2. Input outcome is `completed` when none were captured or all are processed,
   superseded, or withdrawn; `partial` when some are accounted for successfully
   and some have effective unresolved blocked/failed dispositions; otherwise
   `failed`.
3. Overall status is `completed` only when both outcomes are completed. It is
   `failed` when every nonempty work category failed. Otherwise it is `partial`.
   Empty categories do not turn entirely failed real work into partial success.

Thus a Run may close truthfully after a failed attempt, but it cannot report
completion with pending captured input. Blocked/failed inputs remain pending
and visibly count toward later work. Successful source checkpoints remain
committed even if local input processing fails, and input processing does not
advance source coverage.

Existing `ack_through_seq` equality checks and the prohibition on acknowledging
changes for an overall failed Run remain. For a partial Run, event-range
acknowledgement is still permitted, but it cannot retire pending input or stand
in for a handling receipt. Run summaries separately expose source results,
processed inputs, user withdrawals/supersessions, failed attempts, and remaining
pending inputs. Future inputs are excluded from captured completion accounting.

Owner abandonment remains available without fake processing results. It marks
the Run failed and leaves unhandled inputs pending. Already committed handling
receipts and original-text history survive; the next Run selects only the
remaining pending revisions. Closing a Run does not close delegations or user
Todos.

## Proposed user experience

The Item reader keeps a current-note editor and adds **Prior notes** containing
exact original text, submission time, disposition, and handling outcome. Show
superseded and withdrawn notes distinctly from agent-processed notes. History
is ordered and paged; processing results stay connected to their original input.

For both entry points, present **Awaiting agent** until handled. A failed attempt
retains that state and publishes an Item update with the limitation and next
step, adjacent to the available note editor. A person may add guidance without
starting a new Item or finding the Run log. After a successful handling
record, show **Processed**, with the response or outstanding follow-up accessible
on the same Item. Processed is not labelled Done when work remains. A cleared
note editor must leave an immediately visible processing result and prior-note
entry so clearing cannot appear to be data loss.

Saving and processing conflicts preserve entered editor text and explain that
the Item changed, allowing reread and retry. Saving failures do not close the
capture dialog or show success. Status updates must be available to assistive
technology; current forms, keyboard interaction, focus, and narrow-layout
reading order must remain usable. This is a code-grounded experience proposal,
not a rendered or user-tested design.

## Affected surfaces and compatibility

| Owner/surface | Required behavior delta |
| --- | --- |
| `internal/app` | Submission lifecycle, original-text history, atomic handling, Run capture and completion accounting, pending discovery. |
| `internal/store` | Durable input identities/revisions, history and results; migration that cannot invent processing receipts. |
| HTTP, CLI, MCP | Equivalent bounded reads and explicit handling/update/archive operations, including bounded same-Item relevance changes, with shared conflict/idempotency semantics. Note-writing authority stays with existing user entry points. |
| Portal | Pending/processed visibility, Prior notes, preserved inbox original, archived capture access, visible failure updates with follow-up notes, and recoverable conflicts. |
| Stored guidance and heartbeat examples | Required input consumption, full reads, handling receipts, and failed-attempt reporting before Finish. |
| Run/Item history and status | Separate intake processing from source coverage, user state, and outstanding follow-up. |

The stronger Finish rule is a behavioral compatibility change for old clients:
they may discover that Finish is rejected when input lacks an outcome. Errors
must identify the obligation and the supported read/handling path. Existing
Runs started before the upgrade keep their former captured completion rules;
the migration must not retrofit an obligation they never captured. Newly
started Runs use this contract, including retries of their original Start
response. Update current default guidance using the existing exact-prior-default
migration rule; retain owner-customized `agents_md` and expose the new default
for deliberate merge/reset. Adoption instructions must cover MCP reconnect and
copied heartbeat prompts.

Implemented migration policy: nonempty existing freeform notes become pending;
existing user-origin `note` Items without Watcher provenance receive a legacy
inbox intake obligation preserving their current full text. Human
acknowledgement or Done does not prove prior agent processing. Label legacy
input as requiring review and never infer external execution authority from
backfill. This deliberately favors one bounded re-review over silently losing
unreceipted input. Older text overwritten before this feature cannot be
reconstructed and must not be fabricated. The requested implementation adopts
this legacy selection rule; other user-created Item kinds
are not backfilled.

## Acceptance claims

| ID | Observable requirement and representative boundary |
| --- | --- |
| CHG-01 — Durable intake | A saved note or inbox submission is selectable until processed or explicitly superseded/withdrawn, even after event acknowledgement and with no due Watchers. |
| CHG-02 — Full input | A Run captures exact input revisions with complete text available through bounded reads; optional Attention does not control intake selection. |
| CHG-03 — Explicit handling | Reading alone leaves intake pending. Successful handling saves a response, incorporation reason, or durable follow-up together with its receipt. |
| CHG-04 — Original retained | Processing clears the current note and exposes its exact text in Prior notes; rewriting an inbox report retains the original submission. |
| CHG-05 — Newer input protected | Handling A cannot clear concurrent B. User revision/withdrawal preserves history and prevents stale instructions from being newly processed. |
| CHG-06 — Completion enforced | An omitted captured input rejects Finish. Effective unresolved blocked/failed dispositions permit truthful partial/failed closure, preserve pending input, and prevent completed status; recovered historical failures do not downgrade successful handling. |
| CHG-07 — Finite boundary | Input arriving after Start remains pending for a later Run and cannot prevent current completion. |
| CHG-08 — Recovery once | An uncertain handling retry produces one receipt/history transition. A crash before Finish does not make a later Run duplicate a saved response. |
| CHG-09 — Ownership preserved | Processing preserves Todo, reminders, acknowledgements, provenance, unrelated content, and other delegations; it grants no external action authority. |
| CHG-10 — Independent outcomes | Successful source checkpoints survive local input failure; input-only Runs have truthful status; event acknowledgement cannot retire pending intake. |
| CHG-11 — Visible lifecycle | Users can distinguish awaiting processing, a failed attempt, processed intake, supersession/withdrawal, and outstanding follow-up on the same Item. |
| CHG-12 — Compatible adoption | Pre-upgrade Runs retain captured rules; custom guidance survives; legacy backfill retains evidence limits and does not invent prior handling. |
| CHG-13 — Stale editor fenced | Processing advances user state when clearing a note. A save from an older editor conflicts, retains its draft, and cannot silently recreate already processed input. |
| CHG-14 — Run write fenced | New handling/attempt writes require a running Run that captured the exact revision; closed or uncaptured writes fail. Identical committed retries after closure return their original receipt without new effects. |
| CHG-15 — Inbox operation | Equivalent agent operations can retain/update or archive handled inbox captures and change bounded Item relevance without changing ownership; configuration/reference outcomes link to actual durable results. |
| CHG-16 — Batch interpretation | Agents read the complete chronological captured batch and related prior handling before acting; a cleared correcting note remains available when interpreting an earlier inbox request. |
| CHG-17 — Visible failure and recovery | Failures appear as Item updates with partial effects and next steps; users can add guidance. A later successful attempt controls effective status while retaining failure history. |

## Delivery boundary and readiness

The smallest coherent slice includes both supported entry points, original-text
retention, mandatory bounded Run input, atomic processing receipts, Finish
enforcement, equivalent public adapters, lifecycle visibility, and guidance
adoption. Implementing only a prompt instruction or automatically clearing notes
at Finish does not satisfy this contract.

Implementation authority covers the lifecycle, withdrawal, recovery and backfill
rules above. Evidence must cover CHG-01 through CHG-17 across the shared
application and public adapters; local passing evidence is not production
validation or a release decision.
The user is the decision owner; repository maintainers own adapter compatibility
and implementation. Structured-answer intake can be specified as a follow-on
without weakening this slice's two explicit obligations.

Independent specification review identified two gaps in the initial draft:
automatic clearing needed explicit user-state-version fencing, and handling
mutations needed active captured-Run fencing. Both are addressed above and in
CHG-13/CHG-14. Review establishes specification integrity within its scope,
not runtime behavior, owner acceptance, or implementation authority.

The fresh 2026-10-09 review raised missing equivalent relevance operations,
related-input reconciliation, and effective outcome precedence after retries.
The owner decisions above address their direction; the operation, chronological
batch reads, visible failures, and effective-disposition rules specify that
direction. Independent follow-up confirmed the three findings were addressed
and identified a minor CHG-06 wording conflict with recovered-success semantics;
CHG-06 now distinguishes unresolved failures from historical failed attempts.
The review supports owner specification use, not implementation approval.

## Implemented technical design

The existing `internal/app` transaction is the completion authority. Migration
11 adds Item-local immutable `item_inputs` and append-only `input_attempts`.
Frozen Run input summaries are persisted in an atomic `run.finished` event
using the existing event table/index; no Run column is added. There is no new service, worker, source Watcher,
or independent Task owner. Notes retain their existing editable `user_note`
projection; saving revises/withdraws immutable intake records in the same
transaction. New user-origin note Items receive an inbox input at creation.

```text
Portal capture / note save -> Item + immutable input
Run Start -> captured chronological pending inputs + existing source snapshot
Agent reads captured batch + current Item/history
Owning commands -> configuration/reference effects and durable references
Process input -> app.mutate transaction:
  validate active Run membership + input ID + content/state versions
  save visible report/context/relevance/delegation result + attempt
  on success, process input; clear exact note or archive inbox capture
Finish -> source results + effective input dispositions -> persisted summary
```

Reusing only events was rejected: acknowledged changes cannot represent durable
unhandled input, preserve every submitted revision, or enforce exact-input
completion. A separate queue/runtime was unnecessary because the current SQLite
transaction already owns state, history, idempotency receipts, and Run capture.
Input IDs identify immutable revisions; current user state supplies withdrawal
and supersession. Source cursor advancement remains with Watcher publication.

The public handling operation is `POST /api/v1/items/{id}/inputs/process`,
`aicp item process-input <id> --file <json>`, or MCP `process_item_input` with
`{item_id,input}`. The command supplies `run_id`, `input_id`, current
`expected_content_version` and `expected_state_version`, `outcome`, and
`result_md`. Optional `references`, `archive`, `report`, `context_md`,
`interests`, and `delegations` implement the bounded result update. A follow-up
needs an explicit durable reference or resulting pending/blocked delegation;
an old closed delegation does not satisfy it. Failure updates are published on
the same Item and keep input pending. Current defaults and heartbeat guidance
explain the new obligation without replacing owner-customized instructions.

Input history is paged through `GET /items/{id}/inputs?offset=...`, CLI `item
inputs`, and MCP `get_item_inputs`. Detail/history contain latest attempt
summaries and total counts. Complete attempt history uses `GET
/items/{id}/inputs/{input_id}/attempts?offset=...`, CLI `item inputs --input`,
and MCP `get_input_attempts`. Input pages contain at most 50 records and attempt
pages at most 20. Run-bound `user_inputs` pages reuse the existing count/byte
bounded context paging. Reading either history never processes input.

The reader keeps the note editor separate from history, shows failed attempts
with a guidance path, and preserves drafts while distinguishing stale note
identity from unrelated user-state changes. Original submissions are disclosed
without forcing short reports to grow; failure details open for follow-up.
Withdrawal/supersession is labelled separately from successful processing.

Item detail returns current pending inputs and their latest attempt summaries;
prior inputs have one canonical paged read path. Only Item detail polls. A change
to Item versions, pending input IDs, attempt counts, or archive state invalidates
the history query. Existing monotonic versions detect complete note transitions
between polls; idle detail polling does not fetch history again. The
editable current-note projection and separate archive field remain intact.
This simplification was authorized after the implementation debrief on
2026-10-09. The external Run-detail `input_summary` remains unchanged and frozen
after later input recovery; older Runs without a completion event return null.

For verification, `AICP_TEST_PORT` selects an alternate loopback port while
`serve --port` retains the default 7331. Owned CLI/MCP subprocesses receive
explicit server URLs and owned data-directory defaults; the readiness handshake
must still originate from their own server process. This preserves a running
installation while exercising the same public application path.

## Implementation verification and review

Focused application, migration, adapter, and isolated browser checks passed.
Independent code review found closed-delegation follow-up acceptance, unbounded
attempt reads, and a misleading withdrawn-note summary; all were repaired and
independently rechecked without remaining actionable findings in that scope.
Final evidence on 2026-10-09:

- `scripts/verify.ps1` with `AICP_TEST_PORT=17331`: passed typecheck, Go
  formatting/lint/vet, 82 Go cases in seven packages, and all 35 core browser
  checks/journeys. Two optional renderer-profile cases were outside this gate.
- `go test ./...`: passed all packages. Focused regression tests additionally
  cover stale edits, supersession, input-only failure/recovery, migration from
  schema 10, open-delegation validation/rollback, and complete 25-attempt paging.
- `node --test scripts/testing/harness.test.mjs`: seven tests passed, including
  subprocess endpoint/data-directory isolation.
- `scripts/demo.ps1 -Port 17332`: passed two CLI inspection cycles with user
  Todo/reminder preservation. No source fetches or real external agents ran.
- `git diff --check`: passed. Independent code-review follow-ups found no
  remaining actionable issues within the inspected implementation and later
  verification-routing changes.

Evidence bundle: `test-output/run-2026-10-08T18-12-52.688Z-28012/summary.md`
(local ignored output; its UTC directory date precedes the Singapore local date).
An early alternate-port attempt exposed a subprocess default-endpoint escape
into an existing disposable UX demo. Its original owner context was restored,
the accidental fixture work was retired, and test history retained. CLI/MCP
endpoint binding and demo readiness were repaired before the passing rerun.
No hosted release or live personal installation upgrade is claimed.

After the authorized debrief simplification, the complete Windows gate was
rerun with `AICP_TEST_PORT=17331`: 83 Go cases in seven packages and all 37 core
browser checks/journeys passed, with no formatting/lint/vet/typecheck issues.
The two optional renderer-profile cases remain outside the core gate. Final
bundle: `test-output/run-2026-10-09T00-35-39.774Z-6388/summary.md`.
New regression evidence covers frozen completion counts after later recovery,
single Finish-event publication on retry, no idle history polling, and a
save/withdraw transition entirely between detail polls. Independent follow-up
review found no remaining actionable findings after adding monotonic Item
versions to invalidation and disabling inherited polling on both history views.
