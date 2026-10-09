# aicp glossary

This is the shared vocabulary for aicp's Watcher–Interest model. The
[model specification](specs/20260924-watcher-interest-model-spec.md) describes
its behavior. Older 2026-09 MVP specifications describe the former **Watch**
child-of-one-Interest model.

## The main path

```text
Watcher reads its configured source -> cites an Item source reference
                                   -> creates or updates an Item
                                                  |
Interests explain relevance ---------------------+
                                                  |
User state and new evidence determine whether the Item appears in Attention
```

| Term | Meaning and boundary |
| --- | --- |
| **Watcher** | A durable definition of an input stream: the source and exact scope to inspect, how often, how to inspect it, its lifecycle, and its source-reading checkpoint. It may be broad (a mailbox) or focused (a query or project-specific stream). A Watcher can exist without one exclusive Interest. It is not a connector, credential store, or running agent. |
| **Watcher source** | The external system or bounded collection inspected through the agent's existing tools, such as a mailbox, Slack channel, folder, or repository query. `WatchSource {kind, locator}` describes this input scope; aicp does not fetch it. This is distinct from an Item source reference. |
| **Interest** | A durable statement of what matters to the user, with instructions for recognizing, interpreting, and retaining relevant information. It may absorb findings from many Watchers. Its descriptive meaning remains prose unless a field serves a specific application operation. |
| **Watcher–Interest association** | An explicit instruction to assess a Watcher's input for a particular Interest. A Watcher may have several associations. An association can change without changing the source stream's identity or checkpoint. |
| **Broad Watcher** | A Watcher configured to assess its input against the applicable active Interests, including Interests added later. Broad describes the matching policy, not unlimited source access or permission to scan beyond its locator. |
| **Focused Watcher** | A Watcher configured to assess input against explicitly associated Interests. Its source scope, cadence, or lifetime may also be narrower. An empty explicit association set does not silently mean “all Interests.” |
| **Item source reference** | The existing `Item.sources[]` entry for concrete evidence, such as a mail thread, Slack thread, issue, or document. Source-backed agent findings use `id`, `url`, `label`, and `observed_at`; its `id` is unique only within that Item and lets an `open_link` action choose a URL. A user-created Item may have no source or date-only source context without a link. This is the intended meaning of “anchor” in the discussion, not a new entity. |
| **Item** | A durable finding or piece of work about a continuing matter. It has a stable identity across later observations. Its content and evidence can change while user-owned Todo, reminder, acknowledgement, and note state remain separate. One Item can be relevant to several Interests; its originating Watcher and evidence remain visible. |
| **Relevance reason** | The explanation that connects an Item to an Interest. It describes why the evidence matters under that Interest; it does not claim that the Interest caused the source event. Several reasons can coexist on one Item. |
| **Provenance** | The answer to where an Item came from, assembled from its originating Watcher, Item source references, and Run/Watcher result. These existing facts do not require a separate Observation record. |
| **Attention** | The current projection of Items that call for the user's attention. Agent-generated source findings have Watcher provenance, an Item source reference, and an Interest reason. User-created Items can have no source and still appear through Todo, reminder, or acknowledgement state. Proposals have a separate review queue. Attention is not an independent source record. |
| **Attention trigger** | The present reason an Item needs a look: new substantive content, open Todo, due reminder, or another explicit review state. The trigger can change without changing the Item's original Watcher provenance. |
| **Todo / Done** | User-owned follow-up state on an Item. `todo` means locally open; `done` means locally completed. Neither state by itself proves that the external source changed. |
| **Reminder** | A user-owned time to resurface an Item in the portal. It does not schedule a source scan, launch an agent, or send an operating-system notification. |
| **Acknowledgement** | The user's acknowledgement of a particular Item content version, shown as Mark seen in the portal. Adding or completing a Todo and setting a reminder also mark the viewed content version as seen. New substantive content may need acknowledgement again; Mark seen does not clear an open Todo or reminder. Mark Done clears its reminder. |
| **User note** | Text owned by the user on an Item. Source publication must not overwrite it. Explicit input handling may clear the exact processed note while preserving its original text and result in Item-local history. New or revised notes remain pending for a later Run. |
| **User input** | A durable Item-local note revision or inbox submission awaiting agent handling, independent of Attention and source coverage. A Run captures pending inputs; explicit processing saves a visible result and receipt. Failure remains pending and visible for user guidance. Processing intake does not mark a Todo Done. |
| **Review answer** | A human-owned submission of choices or text for an Item review. It snapshots questions and resolved answers, appears as an additional user note, and remains available to later agents. Applicability follows the recorded review material; saving an answer does not imply acknowledgement, Done, or external execution. |
| **Item kind** | A presentation and initial Todo distinction: `note`, `report`, `task`, or `outcome`. All share the same content and user-action model. An Outcome is a lightweight Item about an intended result, not a separate goals engine. |
| **Report content** | Agent-authored Markdown, supported structured review blocks, and optional actions within an Item. Blocks can interleave prose, images, diagrams, and input controls. Its source claims should be grounded in cited evidence. It is versioned independently of user-owned state and saved review answers. |
| **Deduplication key** | A stable key for the continuing matter described by an Item. It must not include the current run or mutable display title. Source-native identity can be encoded here or explained in context. aicp does not need to group different Items by a shared source reference. |
| **Item context / handoff** | An Item's `context_md`: persisted facts, questions, and next steps for later agent work. It is distinct from global `AGENTS.md` and `USER.md` context stored in settings and included in the run packet. Neither is a substitute for source evidence. |
| **Item delegation** | An Item-local record of delegated work: stable local ID, executor, external continuation reference, instructions, follow-up marker, and context. The primary agent owns launch, retrieval, judgment, and repair; an executor may read/update the assigned Item. It is not an independent Task, execution attempt, receipt, or scheduler. |
| **Proposal** | An agent-suggested configuration change for a separate user review queue. Background discovery proposes; an explicit user configuration request can be applied directly. A proposal explains its evidence and overlap with existing configuration. |
| **Run** | One external agent inspection attempt over selected due Watchers. aicp records the attempt and its results; it does not start the agent. Configuration reading and editing do not start or claim a Run. |
| **Watcher result** | One terminal coverage report for a selected Watcher in a Run: success, partial, or failed, including limitations and any Items published. Only successful coverage can advance its checkpoint. |
| **Run status** | The outcome of the whole Run, derived from selected Watcher results and effective captured user-input dispositions, or explicit abandonment. It is distinct from an individual Watcher result's status; one successful Watcher does not make a partly failed Run successful. |
| **Checkpoint / cursor** | The source-specific opaque position last successfully committed for a Watcher. It says where reading reached; it does not prove that older content was assessed under every current Interest. Never copy it between different source scopes without an explicit compatibility decision. |
| **Continuation cursor** | A paging token for reading a bounded run packet or event range. It is not the Watcher's source cursor and does not represent source coverage. |
| **Assessment coverage** | The range of source input assessed using the Interest instructions captured by a Run. New or revised Interests apply only from a later Run; old input is not automatically reassessed. This meaning can be recorded with the existing Watcher result rather than a new coverage entity. |
| **Baseline** | The declared starting boundary and first-scan behavior for a Watcher. A new or revised Interest uses the next Run's source boundary; it does not create a retrospective baseline or claim historical inspection. |
| **Due** | Eligible for an external inspection attempt at the current time. Due does not mean an agent is running or a source was read. The portal should explain the scheduling or retry reason. |
| **Lifecycle state** | `active`, `paused`, or `deprecated` for an Interest or Watcher. Pausing stops current eligibility while retaining identity and history; deprecation records that it is no longer a current focus or input. Neither deletes Items or user state. |
| **Validity end** | An optional time after which a temporary Watcher is no longer eligible for new scheduled inspections. It retains its identity, results, and Items; expiration does not mark those Items done or erase their reminders. |
| **Slug** | A readable, unique name used to find and configure an Interest or Watcher, such as `gmail-inbox` or `delivery-risk`. A slug is stable until explicitly renamed; its display title may change freely. Internal immutable IDs can remain for referential integrity and history. |
| **Revision** | A version used to reject stale configuration edits and identify what a Run captured. A later configuration revision does not by itself invalidate that Run's result; the result remains attributed to its snapshot and obeys source-cursor rules. |
| **Request ID** | An optional caller-supplied identity for a retryable mutation. The server can assign one when omitted; safe replay after an uncertain response requires reusing the original request ID. |
| **Event** | A durable record that a configuration, finding, or user-owned state changed. It supports history and agent handoff; current records remain the authority for current state. |

## Relationships and examples

A broad `gmail-inbox` Watcher can inspect the mailbox once and assess new mail
against `delivery-risk` and `team-leadership`. A focused `migration-mail` Watcher
can inspect a narrower Gmail query for `delivery-risk` on a different cadence.
The second Watcher is justified by a distinct source scope or inspection need,
not simply by wanting another Interest label on the same mail.

A mail thread can appear as an Item source reference. The same thread may
support one Item about a delivery decision and another about a hiring
commitment. One decision Item may also cite that thread and a linked document.
The shared URL creates no required relationship between the two Items.

If the user sets a reminder on an Item, its later appearance in Attention is
triggered by the reminder. The Item still retains the Watcher and source
reference that originally supported it. Historical local Items without a
Watcher remain readable during migration; the new model must not invent source
provenance for them.

## Representation

| Meaning | Representation | Boundary |
| --- | --- | --- |
| Input scope | `Watch.source {kind, locator}` | The Watcher has no compulsory parent Interest. |
| Concrete evidence and link | `Item.sources[] {id, url, label, observed_at}` | Reuse it. “Anchor” is a discussion synonym, not a second object or a cross-Item index. |
| Source-result coverage | `WatchResult.result.coverage` with cursors, observation time, and limitations | Reuse for reading progress; record applicable Interest context in the result when assessment history is required. No separate coverage entity is implied. |
| Source provenance | `Item.watch_id`, Item source references, and Watch results | Keep these facts; no new provenance layer is needed for the described journeys. |
| Interest relevance | `Item.interests[]` backed by `item_interests` | Each Interest has an Item-local relevance reason. |
| Attention trigger | Todo, reminder, and acknowledged content-version fields | Derive it from existing state rather than store another status. |
| Human handle | Unique Interest and Watcher slugs | Retain immutable IDs for references and accept unique UUID prefixes. |

## Names inherited from the former model

| Former contract | Current vocabulary |
| --- | --- |
| `Watch` with one immutable `interest_id` parent | `Watcher` with a configurable broad or explicit Interest-matching policy |
| `Interest -> Watch -> Item` tree | Watcher input, Item evidence, and many-to-many Interest relevance |
| `watch move --interest` | Change a Watcher's associations or matching policy; keep source identity and checkpoint when source scope is unchanged |
| `interest_id` on an Item as sole explanation | One or more explicit relevance reasons, with historical context retained |
| UUID prefix as the usual human handle | Slug as the usual human handle; immutable ID retained internally |

The implementation retains `watch` in database tables and many API paths while
using Watcher for the domain concept. The versioned migration converts prior
records before the runtime serves requests.
