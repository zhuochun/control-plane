# aicp — Watcher and Interest model specification

- Date: 2026-09-24
- Updated: 2026-09-27
- Status: Implemented; this document defines the intended behavior and acceptance claims.
- Vocabulary: [aicp glossary](../glossary.md)
- Prior baseline: [MVP product specification](20260913-agent-control-plane-product-spec.md), [technical design](20260913-agent-control-plane-technical-design.md), and [heartbeat contract](20260916-agent-experience-and-heartbeat-spec.md).

This document specifies the Watcher–Interest model and user-facing behavior. It supersedes
the older specifications where they make a Watch the child of exactly one
Interest, require an Interest for source inspection, or make a source run the
path for configuration work. Those documents describe the former implementation.
The existing safety properties still apply unless changed explicitly below. A versioned data
migration converts old durable records before the updated runtime serves;
normal runtime paths use one data model.

## 1. Purpose and scope

aicp helps an external agent turn source information into durable, useful
attention without losing the user's decisions. The user can change what they
care about independently of where information comes from. A continuing inbox
can serve several Interests; a short-lived project query can serve one.

The desired path is:

```text
User configures Watchers and Interests independently
  -> configures how each Watcher selects applicable Interests
External heartbeat selects due Watchers
  -> agent inspects source scope with its existing tools
  -> cites Item source references and assesses applicable Interests
  -> submits truthful coverage and source-backed Items
User sees why each Item matters and why it needs attention now
  -> acts on Todo, reminder, acknowledgement, or note state
Later runs update the same Item without erasing those user actions
```

aicp remains a local control plane. It does not hold source credentials, fetch
external sources, launch an agent, or infer that a due Watcher has run.

## 2. Former behavior and change

Previously a Watch had one required, immutable `interest_id`. A run selected only
Watches whose parent Interest was active and captured that parent's revision.
Publishing a Watch result assigned its parent Interest to each Item. A new Watch
became due immediately. CLI configuration mutations used JSON files; normal
configuration CRUD already worked without a run. Request IDs could be omitted,
and human CLI commands accepted unique UUID prefixes. The MCP surface exposed
the heartbeat and proposals but no direct configuration CRUD.

The new model makes the input stream and the relevance rule independent.
Changing an Interest association must not require replacing a Watcher or
losing its source checkpoint. Configuration is an ordinary operation with its
own history; it never claims due source work merely to obtain a work packet.

## 3. Domain behavior

### Watchers and Interests

A Watcher owns one bounded source scope, inspection instructions, cadence,
lifecycle, optional validity end, source cursor, and next-due state. Its scope
must identify the actual
mailbox/query, channel, folder, document set, or other source the agent will
inspect. The server validates its local shape; source access and semantic
validity are reported by the agent after inspection.

An Interest owns purpose and interpretation instructions. It can apply to many
Watchers, and several Interests can apply to the same Watcher. Descriptive
topics, thresholds, and background remain in instructions rather than being
split into mandatory metadata fields.

Each Watcher declares one matching policy:

- **Broad:** assess source input against the applicable active Interests. A
  newly active Interest joins future assessments without rewriting the Watcher.
- **Explicit:** assess source input only against its listed active Interests.
  Removing the final association leaves it with no relevance target and makes
  it ineligible for scheduled inspection until a target is added or its policy
  changes. The UI must say this plainly.

The source locator bounds what a broad Watcher can read. Broad matching does
not expand the source scope. A Watcher with an empty result still records
honest coverage; it does not manufacture an Item. Paused or deprecated
Interests do not contribute new relevance decisions, but existing Items and
their user state remain available. A broad Watcher with no active Interest is
ineligible for scheduled inspection until one becomes applicable. A temporary
Watcher also becomes ineligible at its validity end, without deleting its
history, Items, or user state.

For a focused source query, the Watcher and Interest association are separate
decisions. Editing an Interest does not silently rewrite the source query.
If two Watchers overlap, configuration shows the overlap and asks for the
reason; overlap is allowed when source range, cadence, evidence, or processing
needs actually differ. No automatic sharing of their cursors is implied.

### Item sources, Items, and Attention

Every newly agent-published, source-derived Item identifies at least one
concrete `sources[]` reference and the Watcher that supplied it. The existing
Item source reference has an Item-local `id`, HTTP(S) `url`, `label`, and
`observed_at`; its ID is not a global external-object identity. Each
source-derived Item records
the Interest or Interests that made it relevant and a concise explanation of
why. Its deduplication identity represents the continuing matter, not the
current Interest, run, title, or summary. A repeated observation by the
originating Watcher may add evidence or an Interest reason to an existing Item
without creating a duplicate. Distinct matters citing the same source remain
distinct Items;
aicp does not group them by URL or source-reference ID.

An Item may cite several source references. The current schema requires a
usable HTTP(S) URL for each reference, and `open_link` actions use its Item-local
ID. Source-native stable identity can remain in the Item's deduplication key or
context when the URL changes. This specification does not introduce an Anchor
record, Observation record, source-to-Items index, or a copy of all raw source
content. An Item retains its originating Watcher. Overlapping Watchers are not
required to merge their Items merely because they cite the same source.

Agent-generated Attention entries are a view of source-derived Items. Each can
answer three questions in the portal and agent context:
where the evidence came from, why an Interest made it relevant, and which
current trigger places it in Attention. A new substantive update, an open Todo,
or a due reminder may be that trigger. A reminder set by the user can bring an
existing Item back without claiming a new Watcher observation. The same Item
appears once even if several Interests or triggers apply.

User-created Items may have no source reference or a user-supplied source date
without a URL. The date is descriptive source context as entered by the user;
it is not a reminder, a Watcher observation, or proof of source coverage. A
date-only reference has no `open_link` action. User-created Items can enter
Attention through their ordinary Todo, reminder, and acknowledgement state.
Existing local or Interest-level Items retain that visibility and user state
without being assigned invented Watcher provenance. Agent-generated Items
without Watcher, Item source, and Interest evidence do not enter Attention or
advance Watcher coverage. Proposals use a separate review queue and do not
acquire invented source provenance.

### Two kinds of progress

The Watcher's source checkpoint means where successful source reading reached.
Assessment coverage means which input range was considered under which
Interest instructions. New or revised Interests apply only to future source
inspections. No prior source window is automatically reassessed, and the
system must not claim that earlier input was assessed under new instructions.
For a Watcher already selected by a Run, “future” begins with the next Run's
captured configuration and its starting source cursor, after the current Run
has finished or been abandoned. This can exclude input read by the older Run
after the configuration edit; the portal must make that boundary understandable.
An opaque source cursor cannot be fabricated by aicp from the current clock.

## 4. Configuration interaction

Listing, showing, creating, editing, pausing, linking, and unlinking Watchers
and Interests work without starting a Run. They do not select due Watchers,
submit partial coverage, or acknowledge the heartbeat's captured change range.
Configuration changes still emit events for the next agent context and Activity.

Human-facing CLI and portal controls use stable, unique slugs for Watchers and
Interests. Their titles are editable labels. An explicit slug rename preserves
the internal identity, history, links, and Items. Slugs must be unique within
their entity type; a duplicate, stale revision, or ambiguous reference fails
without mutation. Machine payloads may retain immutable IDs. Legacy UUIDs and
their unique prefixes remain resolvable during transition.

Simple CLI mutations accept ordinary flags and an optional instructions file.
Structured files remain available for complex or grouped changes. The MCP
surface provides direct read and mutation operations for an agent carrying an
explicit user configuration request, subject to the same validation and
revision checks as CLI and portal. Background discovery continues to produce
reviewable proposals rather than applying configuration implicitly.

Example intent, not a fixed CLI syntax:

```text
show watcher gmail-inbox
link watcher gmail-inbox to interest delivery-risk
show effective scope, Interests, next due reason, and source progress
```

Changing only a Watcher's Interest association retains its Watcher identity,
source cursor, source-result history, and due schedule. It records the old and
new association plus the time of the edit. The new association first affects
the next Run; no historical assessment is implied. Changing the source kind or
locator starts a new source checkpoint for the current Watcher configuration.
The prior source result and checkpoint remain historical facts and are not
applied to the new source scope.

A grouped configuration change may preview and commit related edits as one
local transaction. Preview reports creates, association changes, pauses,
deprecations, affected Watchers and Items, source checkpoint effects, new
assessment gaps, and changes to due work. Apply is bound to the previewed
content and expected revisions; if the state changes, it reports a conflict
and requires a fresh preview. A successful retry with the same request ID
returns the original result. This is a bounded configuration change set, not
automatic reconciliation or deletion of every record omitted from a file.

## 5. Source runs and concurrency

The external scheduler still starts the agent. `start_run` selects a bounded
set of active, due Watchers with at least one applicable active Interest. Its
packet captures each selected Watcher's source scope, revision, cursor, and
the applicable Interest IDs, revisions, instructions, and matching policy.
The agent receives Interests once in the packet rather than duplicating their
instructions on each Watcher.

Every selected Watcher receives one terminal success, partial, or failed
result before the Run finishes. Successful source coverage commits Items and
the source checkpoint together. Partial or failed coverage does not advance
the source checkpoint. The result records which Interest set and instruction
revisions were used, the assessed range, and limitations. A successful scan
with no relevant Items remains a meaningful empty result.

A Watcher, its source scope, or an applicable Interest may change while a Run
is active. These configuration edits commit independently and affect only
subsequent Runs. The current Run can still submit its result using the Watcher
and Interest snapshot it captured at start. Its Items and result history are
attributed to that snapshot; no later Interest is retroactively attached.
For the same source scope, a successful old result may advance the source
cursor if its `cursor_before` still matches the current cursor. It must not
overwrite scheduling changes made after Run start. If the source scope changed,
the old result remains a truthful historical result and may publish its Items,
but cannot advance the new source scope's cursor or next-due state. A failed
or partial old result advances neither cursor. An explicit user-state edit made
during the Run remains protected from Item publication as before.

The run packet remains complete and bounded: continuation pages must be
consumed before claiming full context; a compact view cannot silently hide
unread user changes or unresolved partial coverage. Runs and configuration
changes have separate completion and history records.

## 6. Proposal and portal behavior

A proposal can suggest a new Interest, Watcher, association, source-scope
change, pause, or deprecation. It names evidence, possible overlap with
current configuration, and the effect on source and assessment progress.
Pending proposals are reviewable in their own queue without changing
configuration. Accepting a multi-record suggestion applies one reviewed
configuration change; rejecting or postponing it records a decision so
unchanged evidence does not repeatedly create the same suggestion.

The portal provides stable deep links for Watchers, Interests, and Items. A
Watcher detail shows source scope, matching policy, associated Interests,
cursor/coverage summary, last attempt, next due reason, and source-derived
Items. An Interest detail shows associated Watchers, the findings it explains,
and any gaps in assessment coverage. An Item shows its source references,
originating Watcher, Interest reasons, user state, and content history.
Configuration previews and Activity show association and scope changes over
time.

## 7. Preserved controls and limits

- The agent reads sources using its own tools; aicp does not receive their
  credentials or make external reads as part of configuration apply.
- User-owned Todo, reminder, acknowledgement, and notes survive source content
  updates, association changes, deprecation, and migration.
- Stable deduplication keys, content/state versions, revision fencing, request
  receipts, and terminal Watcher results remain safety controls.
- A source cursor advances only with a successful committed result. A due or
  configured Watcher is not evidence that inspection happened.
- Events and historical results preserve what was known and configured at the
  time; the current records remain the authority for current configuration.
- The supported runtime handles one canonical data shape. A versioned data
  migration transforms prior records before startup; migration failure stops
  serving rather than enabling mixed-version reads or writes.
- A configuration transaction is local to aicp. It cannot guarantee external
  source access, validate remote queries, or promise complete historical
  reassessment.

## 8. Representative acceptance claims

| Key | Observable result |
| --- | --- |
| `WI-01` — Independent configuration | A broad Gmail Watcher exists without one parent Interest. Adding an Interest changes future assessment without replacing the Watcher or losing its source cursor. |
| `WI-02` — Focused input | A narrow Gmail query can be linked only to a specified Interest, with its own cadence and optional validity end. Empty explicit associations do not become broad matching; expiry stops new inspections without deleting history. |
| `WI-03` — One matter, several reasons | One mail thread source reference may support an Item relevant to two Interests; one Item and its user state remain when the second reason is added. Another distinct matter at that thread can be a separate Item. |
| `WI-04` — Traceable Attention | For a source-derived Item, the user can see its Watcher, Item source reference, Interest reason, and present Attention trigger. A due reminder makes no claim of a new source scan. |
| `WI-05` — Forward-only assessment | Adding or revising an Interest preserves the source cursor and applies from the next Run snapshot. Past mail is not marked as assessed under the new Interest. |
| `WI-06` — Configuration without heartbeat | An agent can inspect and change permitted configuration through a direct interface while a Gmail Watcher is due; no run is started and no Gmail result is fabricated. |
| `WI-07` — Stable identity | A slug rename or association edit keeps internal identity, history, checkpoint, Items, and user state. A duplicate slug or stale edit fails clearly. |
| `WI-08` — Transaction and retry | A reviewed grouped configuration change either commits all its local effects and receipt or none; replay with the same request ID returns the original result. |
| `WI-09` — Independent in-flight edits | A configuration edit during a Run does not prevent its snapshot-based result from publishing; the result is attributed to the captured configuration. An old source result cannot advance a newly configured source scope or undo a newer due schedule. |
| `WI-10` — Existing data | A versioned data migration preserves existing Watches, Items, results, and user actions in the new model before the runtime serves; a legacy Item without source provenance is labeled honestly. |
| `WI-11` — User-created Items | The user can create an Item with no source or with a date-only source and use Todo, reminders, and acknowledgement. A date-only source is not an external link or Watcher coverage. |

## 9. Smallest useful delivery slice

The first coherent slice is one broad Watcher and one focused Watcher, multiple
Interest associations, slug-based direct configuration, an updated run packet
and result contract, user-created Items with optional source context,
source-derived Item provenance, and an Attention detail that explains origin,
relevance, and trigger. It includes a versioned migration of current
Watch/Interest records and historical runtime-readable JSON, preserving Items
and user state.
Grouped change previews, richer proposal review, and graph navigation can
follow, provided direct changes already preserve identity and history.

The implementation includes the grouped preview/apply and proposal review
behavior described above. Acceptance remains a matter of executable tests and
public-interface verification, not this document's status label.

## 10. Accepted decisions and implementation boundary

- New and revised Interests apply only from the next Run's captured
  configuration. There is no automatic historical reassessment.
- In-flight configuration changes commit independently. The old Run completes
  against its own snapshot; old source results do not alter a new source scope.
- User-created Items may have no source or a date-only source and can receive
  normal user actions. Agent-generated source findings remain source-backed.
- A versioned data migration converts prior durable state before startup.
  Normal runtime paths read and write one data model, with no legacy-shape
  branches. Each old Watch becomes a focused Watcher associated with its
  former parent Interest; migration must not infer broad matching. Preserve
  its identity, source checkpoint, results, Items, and user state. This also
  includes historical JSON that the runtime reads; preserve what the original
  Run actually saw when converting its snapshot. A failed
  migration prevents serving until it is repaired or the stopped-server data
  backup is restored. CLI, MCP, portal, and agent instructions must switch to
  the new contract together; incompatible clients must fail clearly rather
  than silently writing an obsolete shape.

The exact database and API shapes, migration script, and verification cases
belong to implementation design. They must preserve these behaviors rather
than introducing a second source or provenance entity to fill a schema gap.
