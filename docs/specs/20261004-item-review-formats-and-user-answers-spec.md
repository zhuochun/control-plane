# Item review formats and user answers

- Date: 2026-10-04; refined using software system design.
- Status: Implemented, locally verified, and independently code-reviewed on
  2026-10-04. The owner authorized implementation; release remains separate.
- Vocabulary: [glossary](../glossary.md).
- Preserves: [Watcher–Interest model](20260924-watcher-interest-model-spec.md),
  [Run context and brief](20260930-run-context-and-brief-spec.md), and
  [Item-local delegation](20261002-agent-delegation-and-receipts-spec.md).

## Outcome and boundaries

Agents publish ordered reports containing prose, visual evidence, and review
controls. A person submits choices or text through the portal. Submissions appear
as additional user notes and are returned directly with Item detail for later
agent use. Item identity, kind, provenance, Todo, reminder, acknowledgement,
freeform note, and delegation ownership remain unchanged.

Reports and registered layouts are agent-authored. Answers are independently
human-owned. aicp persists the handoff; it does not launch an agent or execute an
external action when an answer is submitted. A saved answer is not evidence of
external completion, acknowledgement, or Done.

## Report document and interleaving

Version 1 remains Markdown `body_md` plus existing fixed actions. Version 2 has
required fallback `body_md`, ordered `blocks`, and the same `actions` definitions.
New readers render blocks in order. Older readers retain their existing summary
and sources fallback; they do not automatically understand version 2 controls.
Agents discover capabilities before publication. The shared application rejects
unsupported versions and primitives.

| Block | Fields beyond `id` and `type` | Scope |
| --- | --- | --- |
| `markdown` | `body_md` | Report, review, option evidence |
| `image` | Immutable `artifact_id`, required `description`, optional `caption` | Report, review, option evidence |
| `diagram` | `language` (`mermaid` or `plantuml`), `source`, required `description`, optional `caption` | Report, review, option evidence |
| `review` | `title`, ordered `blocks`, optional pinned `format`, optional `submit_label` | Report only |
| `choice` | `question`, `selection` (`single` or `multiple`), `required`, optional selection limits, `options` | Review only |
| `text_input` | `question`, `required` | Review only |
| `actions` | Ordered `action_ids` referencing existing report actions | Report only |

IDs are nonblank, up to 100 UTF-8 bytes, and unique in their enclosing collection.
Unknown types and fields that do not belong to a type are rejected. Reviews
cannot nest. Choice options have `id`, `label`, and optional display-only blocks.
Each review contains at least one input. One explicit submit button saves the
complete input set in that review; no generated handler or expression is allowed.

Existing actions remain `open_link`, `set_todo`, `set_reminder`, `clear_reminder`,
and `acknowledge`. Action blocks determine placement. Selecting a choice updates
draft input; it does not invoke an Item action. Action blocks are outside forms so
an unrelated action cannot accidentally submit a review.

Markdown uses ordinary safe rendering, with raw HTML skipped. Fenced `mermaid`
and `plantuml` code can invoke the same diagram component as explicit diagram
blocks. Markdown links remain links. Actions and inputs are explicit blocks;
there are no magic Markdown directives for mutations.

Example resolved report:

```json
{
  "schema_version": 2,
  "body_md": "Compare the rendering approaches and choose one.",
  "blocks": [
    { "id": "intro", "type": "markdown", "body_md": "## Rendering approaches" },
    {
      "id": "rendering-decision", "type": "review", "title": "Choose an approach",
      "format": { "id": "rendering-choice", "version": 1 },
      "blocks": [
        {
          "id": "flow", "type": "diagram", "language": "mermaid",
          "source": "flowchart LR\n  Agent --> Item\n  Item --> Human",
          "description": "An agent publishes an Item for human review."
        },
        {
          "id": "approach", "type": "choice", "selection": "single",
          "question": "Which rendering approach?", "required": true,
          "options": [
            { "id": "local", "label": "Local rendering" },
            { "id": "external", "label": "External service" }
          ]
        },
        { "id": "feedback", "type": "text_input", "question": "Additional instructions" }
      ],
      "submit_label": "Save answer"
    },
    { "id": "links", "type": "actions", "action_ids": ["open-source"] }
  ],
  "actions": [
    { "id": "open-source", "type": "open_link", "label": "Open source", "source_ref": "source-1" }
  ]
}
```

## Registered formats and binding

Formats are immutable ordered review layouts, composed from platform primitives.
They define structure and constraints; Item instances supply question wording,
option labels, prose, and evidence. They cannot add executable rendering code.
The registry is scoped to the local aicp installation. No deletion or mutable
alias is introduced in this slice.

```json
{
  "format": {
    "id": "rendering-choice", "version": 1, "title": "Rendering choice",
    "fields": [
      { "id": "flow", "type": "diagram" },
      { "id": "approach", "type": "choice", "selection": "single", "required": true },
      { "id": "feedback", "type": "text_input" }
    ]
  }
}
```

`fields` includes display blocks as well as inputs, in exact order. Publication
must match IDs, types, required flags, selection mode, and minimum/maximum
selections. The complete resolved instance is saved in Item content. It does not
need registry lookup to render or interpret saved answers. Inline reviews use the
same validation and require no registry entry.

Identical registration at an existing ID/version is a safe replay; changing its
definition conflicts and requires a new positive version. IDs use an ordinary
short descriptive name; uniqueness is enforced on `(id, version)`. Registration
does not update existing Items, start a Run, or change human state.

## Visual evidence and rendering

PNG/JPEG artifacts are uploaded as base64, validated, stored in SQLite, and
addressed by SHA-256 of their bytes. aicp serves them from its own origin. Remote
image URLs are not server-fetched. All artifacts are retained in this slice;
there is no garbage collection that could break answer history. Upload is
idempotent and identical bytes reuse the same ID.

Both diagram languages support preview, enlargement, readable source, and
description. Mermaid is lazy-loaded in the portal with platform-controlled
strict configuration, then sanitized and displayed as an SVG image rather than
inserted as executable report HTML. PlantUML uses a configured local Java/JAR
renderer captured at server startup. Set `AICP_PLANTUML_JAR` to an absolute JAR
path and make Java available on PATH. No external renderer is used.

Capability discovery reports `plantuml_available` only after a bounded startup
render succeeds, and includes `plantuml_setup` with platform-specific guidance.
Agents can request installation of missing dependencies using that guidance;
package installation remains an operator action. On macOS use Homebrew's
PlantUML package and configure its OpenJDK binary directory and JAR path; on
Windows use a compatible Java runtime and the official JAR. The renderer does
not execute package-manager commands or shell wrappers. Restart the server after
changing dependencies or environment. See the
[architecture setup instructions](../architecture.md) for commands and sources.

PlantUML runs with its SANDBOX security profile, fixed flags, bounded output,
bounded concurrency, a timeout, and a heap limit. The initial subset excludes
external include/import directives, themes, image embedding, and percent
functions. Configuration is operator-owned; source cannot supply process paths,
flags, or environment. Output is PNG, avoiding executable SVG from the process.
Unavailable dependencies or invalid syntax show an error and readable source.

Missing images retain labels/descriptions. Visual failures are visible before
submission; the person can still explicitly answer based on available material.
Saved visual choices retain immutable artifact IDs or the exact diagram source.
Source is authoritative; renderer appearance is a derived preview.

Official rendering references:
[Mermaid security](https://mermaid.js.org/config/schema-docs/config-properties-securitylevel.html),
[PlantUML security](https://plantuml.com/security), and
[PlantUML command line](https://plantuml.com/command-line).

## Additional user notes and answer storage

Each explicit submission appends an Item-local answer record. The User answers
area presents its readable note alongside the independently editable freeform
`user_note`. Do not concatenate generated answers into that field. Structured
records are authoritative; readable notes are derived at submission from server
snapshots, not independently editable copies.

An answer stores its ID, review ID/title, submission timestamp, reviewed content
version, pinned format reference, review-material hash, full field set, and
optional `supersedes` answer ID. Each field stores its ID, type, question wording,
disposition, typed value, and readable answer. Choices snapshot selected IDs,
labels, and visual option content. Text is preserved exactly and displayed safely.
The next agent does not need to decode an option ID to understand the answer.

Example additional note:

```text
Your answer · Choose an approach · 2026-10-04T10:00:00Z
Which rendering approach?: Local rendering
Additional instructions: Avoid external services.
Answered against Item content version 7.
```

Every field supplies `disposition: answered` or `skipped`. Required fields cannot
be skipped. Optional multiple choice can explicitly select none, while empty
optional text is an answered empty value. These differ from skipped input.
Required text must be nonblank. Optional single choice with no value is skipped.

Corrections append a complete replacement with `supersedes` naming the current
submission. Retain history and allow at most one current answer per review.
Concurrent corrections conflict. Agents cannot alter answers through report/work
publication. A newly submitted answer must not change Todo, reminders,
acknowledgement, freeform notes, content version, or delegation state.

## Submission, applicability, and next use

Submission supplies Item/review identity, expected content and state versions,
review-material hash, complete typed values, request ID, and predecessor if
correcting. In one application transaction: validate against stored material,
append the server-snapshotted answer, advance state version, record a user-origin
`item.answer_submitted` event, and save the retry receipt. A failed submission
writes nothing. Identical uncertain retries return the original saved result.
The portal keeps draft input on errors and reloads the latest Item for review.
Uncertain failures retain the entire original payload and request ID through
refetch. Confirmed success, definitive conflict/validation failure, or explicit
input editing clears that pending request. Refreshed fields preserve same-type
text drafts, filter removed option IDs, initialize new fields as skipped, and
reset changed types/selection modes.

Compute a server-owned SHA-256 over title, summary, report, and source references.
This conservative material hash includes all displayed decision context and
immutable artifact identities. Exclude handoff `context_md`, delegations, and
human state. Their updates preserve answer applicability. Changed visible
material marks earlier answers as needing review; removing a review retains its
answers in history. The hash proves unchanged recorded material, not freshness
of external facts.

`get_item` / Item detail returns the freeform note and current answer submissions
with readable question/answer text and applicability for reviews still present.
Removed review answers remain in history; `answer_history_available` exposes that
history even when no active review has answers. Historical submissions use
a paged read. Reading never consumes answers. Current answer pointers are local
to the Item; no separate scheduler or workflow state is introduced.

The existing brief/change path carries the compact user event with Item/review/
answer IDs. Later agents fetch full Item detail to continue. Preserve captured
event boundaries: a later submission appears in a later change range, not in a
previous Run snapshot. Do not embed full answers into every Run packet or infer
agent wakeup/external completion from saved input.

## Adapters, limits, and authority

HTTP, CLI, and MCP share format registration and content validation. Agent tools
expose capability discovery, format list/exact lookup/registration, image upload,
and answer-history reads. Existing publication/work tools carry version 2 reports.
Answers are submitted through the portal HTTP endpoint; no answer-writing CLI or
MCP tool is added. Submission requires the local portal Origin header. The existing
loopback/origin controls and this adapter restriction reduce accidental writes;
they are not authenticated proof that a human made the request. Strong human
identity enforcement is outside this local trusted-device slice.

| Operation | HTTP / CLI / MCP |
| --- | --- |
| Capabilities | `GET /review-capabilities` / `review capabilities` / `get_review_capabilities` |
| Formats | `GET /review-formats` / `review formats` / `list_review_formats` |
| Exact format | `GET /review-formats/{id}/{version}` / `review format` / `get_review_format` |
| Register | `POST /review-formats` / `review register --file` / `register_review_format` |
| Upload | `POST /review-artifacts` / `review upload --file` / `upload_review_artifact` |
| Image bytes | `GET /review-artifacts/{id}` |
| PlantUML preview | `POST /review-diagrams` |
| Submit | `POST /items/{id}/answers`, portal only |
| History | `GET /items/{id}/answers?after=...` / MCP `get_answer_history` |

Paths above are under `/api/v1`. Registry listing returns up to 100 versions;
larger registries require exact lookup and return an explicit list-limit error.
Answer history pages contain 20 entries with `next_after`. Existing content stays
bounded at 512 KiB. Individual collections contain at most 100 blocks/options;
the whole report permits at most 300 blocks and 100 input fields. Each saved
submission is at most 256 KiB.
Diagram source is at most 32 KiB; text input at most 16 KiB. Images are at most
2 MiB, 16 million pixels, and 8192 pixels per dimension. PlantUML allows two active
renders, ten seconds per render, 128 MiB Java heap, and 2 MiB output. These are
initial enforced limits, not measured production capacity claims.

`internal/app` owns validation, registry, answer authority, and transactions.
`internal/store` owns migration/persistence. HTTP owns local rendering transport;
the portal owns safe display, draft input, and user feedback. External agents own
continuation. A versioned additive migration must preserve populated prior Items,
history, user state, and receipts; older runtimes reject the newer schema.

## Acceptance evidence and closure

Verify registered and inline layouts, version immutability and mismatch rejection,
interleaved Markdown/controls/actions, Mermaid and PlantUML previews and errors,
image selection, exact answer snapshots, corrections/history, restart durability,
retry deduplication, stale/invalid submissions without partial writes, and
applicability across handoff versus report changes. Exercise agent retrieval and
brief markers through the public adapters. Confirm old reports/actions remain
usable, foreign/missing portal Origin submissions fail, and agent publication
cannot overwrite freeform notes or answer records.

Run focused application, migration, adapter, and portal tests, then the full
`scripts/verify.ps1` Windows gate. Obtain independent code review, repair concrete
findings, and obtain focused re-review for material fixes before completion.
Report evidence and limitations; a green gate does not prove external agent
execution or deployment fitness.

### Recorded verification on 2026-10-04

- `scripts/verify.ps1`: passed locked install, TypeScript checks/build, Go
  formatting/lint (zero issues), vet, all Go tests, executable build, all 21
  browser tests, and the two-cycle fixture demo.
- Real PlantUML success, syntax rejection, and restricted-source rejection ran
  with a temporary checksum-verified Java 21 runtime and PlantUML 1.2026.8 JAR.
  The system Java 8 runtime was not changed; local deployment needs compatible
  Java and its own configured JAR.
- Independent code review identified two portal defects in uncertain retry and
  refreshed draft reconciliation. Both were repaired, covered by browser
  regressions, and independently re-reviewed with no remaining actionable
  findings. The reviewer independently passed scoped Go tests.
- Browser coverage includes committed-but-lost responses, live field changes,
  image choices, actual Mermaid/PlantUML previews, source/enlargement controls,
  readable answer notes, correction history, action preservation, stale review
  rejection, and draft retention. The captured portal screenshot was inspected.
- MCP verification covers registered publication, current answer retrieval, and
  discovery of the compact answer event through the brief change range.
- Populated schema-upgrade tests preserve prior Item content, note, Todo,
  reminder, acknowledgement, versions, history, and existing Run receipts.
- `git diff --check`: passed. Build size warnings from diagram dependencies
  remain; no performance or authenticated-human identity guarantee is claimed.

### Follow-up verification on 2026-10-05

- Added platform dependency setup guidance to capability discovery and preview
  failures. Startup now verifies Java/JAR usability with a bounded sandbox render.
- Independent follow-up review found no actionable findings and independently
  passed all four PlantUML tests using Java 21 and PlantUML 1.2026.8. macOS/Linux
  setup instructions were checked against official sources, not executed.
- Re-ran `scripts/verify.ps1`: locked install/audit, TypeScript build/checks,
  Go format/lint/vet/tests/build, all 21 browser tests, and the two-cycle fixture
  demo passed. The diagram dependency chunk-size warning remains.

### Portal UX follow-up on 2026-10-05

- Structured content uses the available reader width; Markdown prose retains a
  readable line length at every display depth. Choice labels have larger touch
  targets, and forms use tighter mobile spacing.
- Required answers and selection limits receive field-level feedback before a
  new submission, with focus on the first invalid field. Uncertain retries still
  resend the frozen original request. Extra choices are disabled at the maximum;
  optional controls are disabled while saving. Initial unanswered fields are
  distinguished from skipped values.
- Saved answer cards show local formatted times and question/answer snapshots;
  history labels identify latest/earlier submissions. Stored plaintext notes and
  applicability rules remain unchanged. Long titles and input wrap on phones.
- Fresh desktop/mobile screenshots were inspected. The final portal build,
  executable rebuild, all 22 browser tests, embedded-asset tests, and diff check
  passed. Independent review findings were repaired and re-reviewed with no
  remaining actionable findings. This is scoped UX evidence, not full
  accessibility certification.
