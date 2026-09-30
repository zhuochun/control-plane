# Agent-use efficiency evaluation

- Date: 2026-09-28
- Status: Deterministic MCP evaluation implemented.
- Domain language: [glossary](../glossary.md)
- Existing workload baseline: [long-term performance specification](20260928-long-term-performance-test-spec.md)
- Current agent journey: [Run context and brief](20260930-run-context-and-brief-spec.md) and [packaged prompt](../../examples/heartbeat-prompt.md)

## Outcome and boundary

Determine how much context, time, and protocol work a useful aicp Run requires as
Items, Attention, changes, and configuration accumulate. The evaluation must
identify repeated or unnecessary model-visible material without rewarding a
shorter trace that misses source findings, user changes, or coverage obligations.
It complements the existing browser and HTTP latency suite; it does not impose a
24-hour soak.

aicp supplies configuration, a bounded Run snapshot, Item history, and mutation
receipts. The deterministic runner follows scripted decisions over a fixed
synthetic source corpus. Report aicp packet and MCP costs separately from
source material. The reference tokenizer is a payload proxy, not a measure of
provider-billed tokens or independent agent decision quality.

The 2026-09-28 baseline measured the former requirement to read every Attention
summary. The 2026-09-30 contract makes that collection optional, keeps applicable
Interest instructions and captured changes required, and adds bounded Item
lookup. Comparisons must identify the reading contract used by each trace.

## Current behavior and evaluation obligation

`start_run` returns `{run, context}`. Required context comprises captured owner
and agent guidance, selected Watchers and prior coverage/limitations, applicable
Interest instructions, and the captured user/Proposal event range. Global
Attention is represented by counts and an optional snapshot cursor. Watcher
entries do not embed recent Item lists. `list_items` locates continuing matters;
`get_item` and `get_context` supply current detail for reconciliation. Each
selected Watcher needs one truthful terminal result. Acknowledgement covers
only changes fully consumed and durably reflected.

The packet includes stored `AGENTS.md` and `USER.md` once. The previous
`BriefPage` also included both on every continuation; the evaluator retains
that measured baseline for comparison and records whether a candidate repeats
them. `web/perf/heartbeat.mjs` continues to measure HTTP publication latency;
the separate MCP evaluator consumes the packet continuations and counts
reference tokens.

The first required Interest page retains up to 50 entries. Continuations may
carry up to 150 entries, targeting 64 KiB of items per page; one oversized
entry is returned alone to preserve progress. Agent-facing
Attention entries omit user-state and acknowledgement versions. An Item
acknowledgement since the previously handled Run appears in the captured
change range; open Todos and due reminders still follow Attention rules.
Title, summary, and Interest
reason text are capped at 256, 512, and 256 UTF-8 bytes in this projection;
`truncated_fields` identifies omissions. The full Item remains available from
`get_item`. Explicit optional-Attention reads still verify their captured range
and read full detail when omitted text matters. Run snapshots retain full
Attention text; required packet receipts retain captured instructions and
events. Version 7 converts older Run-start receipts to the new response while
preserving the optional full context in the Run snapshot.

Progressive disclosure means finding the Items needed for actual source evidence
or user changes, without first enumerating the global Attention backlog.
Do not skip required Interest or event pages. Quiet and source-reconciliation
cases read zero optional Attention summaries; this is not evidence that the
user's entire Attention queue was reviewed. The evaluator
distinguishes bytes transferred from reference tokens in the two MCP result
representations; neither measure establishes billed model usage.

## Representative scenarios

Use isolated deterministic data directories and the existing Fresh, Mature,
Large, and Skewed profiles where their shape fits. Add explicit fixture variants
for the conditions below; record their counts and content sizes in a manifest.
These are workload assumptions, not claims about a typical user's data.

| Scenario | Fixed situation and expected agent decision | Cost or risk exposed |
| --- | --- | --- |
| Quiet heartbeat | Few due Watchers; no new relevant source evidence; an aged library and a small Attention set. Read the complete packet, submit honest empty successful results, finish. | Fixed cost per useful no-change Run. |
| Large Attention set | Same due Watchers/source evidence with 1% versus 20% of Items in Attention. Verify captured counts without enumerating optional summaries. | Required packet cost should remain independent of unchanged backlog size. |
| One changed matter | One older Item supported by new evidence and a distinct Item sharing its source URL. Locate the correct Item through bounded MCP lookup before reading/updating it; retain identity and user state. | Reconciliation without an initial global Item list, false merge, or duplicate. |
| Many applicable Interests | One broad Watcher assessed against several active Interests and one focused Watcher assessed against its explicit links. Use the captured Interest revisions and relevance reasons. | Instruction duplication, association handling, and assessment coverage. |
| Change backlog | A narrow recent change range versus many unacknowledged events, including a user action and a configuration revision. Follow every event page and acknowledge only the fully handled captured range. | Backlog scaling and premature acknowledgement. |
| Retry and incomplete source read | An uncertain submission response followed by replay with the same request ID; another Watcher has partial or failed source access. Reuse the request ID, report limitations, and preserve the unsuccessful Watcher's cursor. | Extra work, duplicate publication, and false coverage. |

For reconciliation, use a small fixed source corpus with known relevant and
distinct evidence. The expected outcome must identify
which Items should be created or updated, which should remain unchanged, and
what source range was actually inspected. Keep the corpus and its answer key
separate in the fixture. Scripted protocol replay measures payload cost and
checks final state; it does not establish that an agent would choose correctly.

## Trace and measurement contract

For each case, record the ordered MCP call names, categories, and result sizes
from `start_run` through `finish_run`, including all continuations,
`get_changes`, selected `get_item` reads, and mutation results. Capture
latency, response bytes, page count, and repeated calls. Do not record private
user data or credentials in evaluation artifacts; use synthetic fixture data.

Count reference tokens over the serialized MCP result `content` and
`structuredContent` separately with a named, pinned tokenizer and version.
Break down tool descriptions, initial packet, each continuation collection,
changes, Item detail, mutation receipts, and external source material. Record
repeated `AGENTS.md`/`USER.md` tokens separately as a diagnostic subset; do not
add them to the response total again.

The CLI output slice uses the same tokenizer on exact stdout, including its
trailing newline. Run matched fixtures separately for default pretty JSON and
`--json`; break down status, settings, list pages, record detail, preview,
change pages, Run start, submission, finish, and history. Follow every captured
required Interest cursor through `brief --cursor` and every remaining
change cursor through `changes --cursor`; validate counts against the Run
selected Interest set and final Run state. Explicit whole-Attention CLI listing
is an alternative user-review path, reported separately from required Run reads;
it must not be charged as mandatory source context or used to locate the MCP
reconciliation target.

Report total Run cost and useful finding count, including zero-finding cases
without dividing by zero. Show tool-call count, full-detail reads, cumulative
bytes, and observed tool latencies. For matched comparisons, pin code revision,
fixture, source corpus, tool set, settings, tokenizer, and execution environment.
Single local runs do not support percentile or cross-machine latency claims.

## Correctness and comparison

A case fails if the scripted trace omits a required page, misses an answer-key finding,
conflates distinct matters, invents evidence, changes user-owned state,
acknowledges unread changes, publishes the same substantive update twice after
a retry, or advances a cursor after partial or failed coverage. A truthful empty
successful scan passes when the fixed source corpus has no relevant new input.
Compare the final API-visible Items, versions, Watcher results, checkpoints,
acknowledged change sequence, and user state with the answer key, not just the
runner's summary text.

Compare candidate changes with the same fixture and ordered trace. Show token,
byte, call-count, and latency deltas by category, alongside every correctness
result. A reduction is a useful improvement only if the required information
and outcome remain intact. Do not assign a universal token target or price gate
until a representative workload and baseline distribution are chosen; use the
first clean run to establish those values.

## Acceptance claims and delivery slices

- **AE-01 — Reproducible accounting:** A matched replay attributes every
  model-visible tool response to a category, reports total and duplicate tokens
  with a pinned tokenizer, and retains the raw byte and latency measures.
- **AE-02 — Complete required context:** Applicable Interest and change pages
  are consumed without silent omission; optional Attention remains retained and
  queryable without adding mandatory summaries to quiet Runs.
- **AE-03 — Selective detail:** The one-matter case reaches the correct Item and
  reconciles it without reading unrelated full histories or merging distinct
  Items that share a source URL.
- **AE-04 — Correct coverage:** Every selected Watcher has a truthful terminal
  result; a partial or failed read does not advance its checkpoint.
- **AE-05 — Safe comparison:** An optimization report includes both efficiency
  deltas and answer-key outcomes, with no claimed win when correctness regresses.
- **AE-06 — CLI rendering cost:** Exact stdout tokens are measured separately
  for default and `--json` output across representative command shapes, with
  all captured packet continuations and alternative full Item list cost visible.

The first useful slice is deterministic MCP trace capture and token accounting
for the quiet and large-Attention cases, including the repeated-context cost.
The second adds the fixed source corpus and scripted reconciliation case.
Retry, backlog, and multi-Interest cases extend that same harness. Matched CLI
output cases measure both renderings of the long-term backlog. The existing
browser suite remains the reference for portal experience.

## Decisions for future changes

The deterministic suite uses a pinned reference tokenizer and fixed synthetic
corpus. Any proposal to send less than the complete current snapshot, change
continuation semantics, or alter what a Run acknowledges requires a separate
behavior and compatibility decision. An agent decision-quality evaluation would
need its own model, source corpus, and usage accounting; it is outside this
CLI suite.
