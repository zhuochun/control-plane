# Long-term aicp performance test specification

## Purpose and boundaries

Measure how the local aicp server, SQLite store, CLI/MCP-facing API, and portal
behave with data accumulated over months or years: Items, revisions, Runs,
results, events, and command receipts. The suite uses aged datasets and short,
repeatable browser and API journeys. It should make a performance regression
reproducible and identify whether time is spent in database work, application
processing, response transfer, or browser rendering. It is a test design, not a
claim about current performance or an instruction to change product behavior.

Use the implemented Watcher–Interest model in [the glossary](../glossary.md). A
realistic installation has **few Watchers and many Items**. In this specification,
"retired Watchers" means `paused` or `deprecated` Watchers retained with their
history. Item archiving is not implemented; acknowledgement, Done, and future
reminders must not be used as substitutes for an archive state. Add an Item
archive scenario only after its behavior is specified and implemented.

The external agent and source systems are outside aicp. Generate deterministic
fixture evidence and measure aicp's own request path. Report source inspection
time separately if a later end-to-end test includes a real connector.

## Dataset profiles

Use a fixed seed, an explicit logical clock for fixture timestamps, varied
content sizes, and stable dedupe keys. Preserve normal foreign keys, version
history, result snapshots, and request receipts. Publish a manifest with row
counts, byte sizes, distribution, seed, schema version, and fixture generator
version. The figures below are **workload assumptions to test**, not observed
user statistics or capacity promises.

| Profile | Interests / Watchers | Items and history | Runs and other history | Purpose |
| --- | --- | --- | --- | --- |
| Fresh | 3 active / 2 active | 100 Items; 120 content versions | 30 Runs | Control for fixed request overhead. |
| Mature | 8 Interests (6 active, 2 deprecated) / 6 Watchers (3 active, 1 paused, 2 deprecated) | 10,000 Items; 25,000 content versions | 3,000 Runs, 5,000 Watcher results, proportional events and receipts | Main long-term single-user profile. |
| Large | Same small configuration set | 100,000 Items; 300,000 content versions | 30,000 Runs, 50,000 results, proportional events and receipts | Find growth curves and failure points. |
| Skewed | 5 active Interests / 4 Watchers (2 active, 2 deprecated) | 20,000 Items; 100,000 versions, with 20% of Items receiving 80% of updates | 10,000 Runs; one Watcher owns 80% of Items | Repeated updates and uneven source activity. |

For every aged profile, distribute Items among `note`, `report`, `task`, and
`outcome`; include agent and user-created Items, several Interests per Item,
one to three source references on source-derived Items, parent links where
valid, and both small and moderately large reports. Keep a few near the
512 KiB Item-content limit in a separate payload stress variant so normal
latency is not dominated by an extreme size. Retain the full content-version
distribution (many unchanged Items, some with 2–5 revisions, a few with
20–100), plus old and recent Watcher results. Use at least these current-state
mixes: 1% and 20% Attention; open Todo, Done, acknowledged, due and future
reminders. This isolates total-history growth from active-set growth.

Build aged fixtures offline through a versioned fixture builder using the
current schema and invariants; validate a sample of its records through public
APIs. Exercise all timed mutations through HTTP so the real validation,
transaction, event, and receipt paths are measured. Never run against the
user's data directory. Keep the service on loopback and give each run an
isolated data directory and port. Do not include fixture construction time in
request latency, but report it separately.

## Workloads

Run each row against Fresh, Mature, and Large unless the row names a narrower
profile. Use the same ordered request trace for comparisons. Record HTTP
status, response bytes, server-side duration, and caller-visible duration per
operation. Consume every continuation page when a workflow requires it.

| Scenario | Realistic request sequence | Main question |
| --- | --- | --- |
| Morning open | `GET /api/v1/status`, `GET /api/v1/items?view=attention&limit=50`, first Item detail and context, pending proposals; open Attention in a real browser | How long until Attention is visible and responsive, and does that scale with all historical Items or only the visible set? |
| Library browsing | First, middle, and final pages of `GET /api/v1/items?view=all`; repeat for `todo`, Interest, Watcher, kind, and `q` filters; in a browser open Library, scroll, search, filter, and open recent and old Item inspectors | Do page depth, search, list rendering, and inspector responsiveness degrade with history? |
| Monitoring and Activity | In a browser open Watcher configuration, monitoring status, Run history, and old Run detail; switch between these and Attention | Do retained Watcher and Run histories delay navigation or rendering? |
| Heartbeat with little news | `GET /api/v1/brief`, `POST /api/v1/runs`, consume Interest, Attention, and change pages, submit one terminal result for each selected Watcher, finish; 0–2 Items changed | How much does a routine Run cost after years of history? |
| Heartbeat with findings | Same flow, with 5–20 new Items and 5–20 existing Item updates in batches that fit the 4 MiB request limit; read existing Items first | Where do publication latency, write amplification, and WAL growth appear? |
| No-change repetition | Re-submit existing dedupe keys with unchanged substantive content during later successful Runs | Does the common no-change path avoid unnecessary versions and preserve user state? |
| Hot Item | Repeatedly revise a small set of Items over many Runs; fetch detail, last 20 versions, and Item context after 100 revisions | Do version count and source merging slow updates or history reads? |
| User actions during history growth | Acknowledge, set/complete Todo, set/clear reminder, and edit note on recently and long-ago updated Items while heartbeat reads run | Do reads and writes stay responsive, and is user-owned state preserved? |
| Retired Watchers | Pause/deprecate Watchers after they have substantial history; continue Runs on the few active Watchers and browse old Watcher/Run detail | Does retained history affect due selection or status despite few active Watchers? |
| Run and event history | Read first/deep pages of Runs and configuration/proposal history; consume `changes` from a narrow recent range and a large unacknowledged range | Can the agent and portal navigate accumulated history without reading it all? |
| Restart and recovery | Stop cleanly, restart the aged database, read status and first page, then run one heartbeat; repeat after a completed Run and an intentionally unfinished Run | How long until usable, and are receipts, checkpoints, and active-run state intact? |
| Mixed local use | One writer doing heartbeat publication, 2–4 readers doing status, Attention, Item detail, and portal navigation at realistic think times | Does single-connection SQLite serialization cause reader tail latency or starvation? |

For the measured heartbeat scenarios, preserve the real order of `start -> read
-> submit one result per selected Watcher -> finish`. Use supported Watcher
intervals or explicit forced selection; label forced cycles separately because
they do not measure ordinary due scheduling. Include success, partial, and
failed results, and confirm only successful coverage advances its cursor. Keep
one active Run at a time; concurrent clients may read or perform permitted user
actions. A fixed set of repeated cycles is sufficient to exercise revisions
and publication without a day-long run.

## Measurement and comparison

1. Pin executable commit, build mode, schema, OS, CPU, RAM, disk type, Go/Node
   versions, and power mode. Build `web/dist` before the Go executable. Keep
   fixture seed and request trace identical between revisions.
2. Measure cold startup, first navigation, and warm navigation separately.
   Repeat each scripted browser journey across fresh browser contexts and at
   least three server processes per profile. Report p50 and p95 with sample
   counts; report p99 only when the sample is large enough to support it.
3. Treat real-browser journey latency as the primary result: time to useful
   content, list and inspector rendering, long tasks, search/filter
   input-to-render delay, and browser memory. Use the same viewport and
   scripted interactions on Fresh, Mature, and Large. Capture network timing,
   response bytes, and failed requests to locate a delay. Record Go heap/RSS,
   CPU, and SQLite DB/WAL size for diagnosis. For agent-only heartbeat flows,
   report caller-visible API timing and cumulative pages and bytes.
4. Keep measurement probes lightweight and disabled or identically configured
   in both compared builds. For slow cases, capture CPU/heap profiles and
   SQLite `EXPLAIN QUERY PLAN` on the specific query. Do not put profiling
   overhead into the reference latency series.
5. Run A/B on the same fixture snapshot and machine, alternating build order.
   Compare Fresh-to-Mature-to-Large slopes as well as absolute latency. A change
   that speeds up one request but increases total heartbeat bytes, write time,
   or portal memory still needs an explicit tradeoff decision.

Treat the first clean run as the baseline. Before adopting hard gates, agree on
device and workload targets. Initial review targets for a typical local
development machine are: p95 under 250 ms for status and first page of
Attention with a 1% active set; under 1 s for a 10-Item findings submission;
and under 2 s for first useful portal render on Mature. These are proposed
targets, not measured guarantees. Also flag any >2x Mature-to-Large latency
increase in a bounded first-page operation for investigation; the 10x data
growth means a simple absolute limit alone can hide poor scaling.

## Correctness gates and interpretation

At each profile and after each measured workflow, compare API-visible counts and a
deterministic sample with the fixture manifest. Verify stable dedupe identity,
expected content versions, preservation of Todo/reminder/acknowledgement/note
state, Item source provenance and Interest reasons, immutable historical Run
snapshots, one terminal result per selected Watcher, correct cursor advancement,
and idempotent replay with the same request ID. Check SQLite integrity and
foreign keys on the stopped test database. A fast response with missing pages,
changed ordering, or incorrect state fails the suite.

The result report should show browser p50/p95 and memory against Item count for
Attention, Library, Item inspector, Monitoring, and Activity. Give the slowest
request or rendering step, dominant code/query path when known, response bytes,
and evidence from the profiles before proposing an optimization. The
[first baseline](../perf/20260928-perf-baseline.md) measured the earlier
whole-collection pagination and portal loading paths and the resulting
improvement. Continue separating server latency from browser rendering and
external-agent time.

## Current coverage and remaining work

The [performance suite](../../scripts/perf/README.md) seeds Fresh, Mature,
Large, and Skewed profiles. Its browser driver covers Attention, Library first
and next pages, search and filters, Item inspector, Todos, Monitoring, and
Activity. Its HTTP driver exercises three publication cycles, unchanged
content, two concurrent readers, restart with an unfinished Run, and owner
abandonment. The [first local report](../perf/20260928-perf-baseline.md) records
single-host results and their sample limits.

Deep-page and hot-Item history reads, partial and failed Watcher results under
aged data, and broader mixed-use traces remain specified scenarios without
dedicated measurements. The current fixture manifest records profile counts and
generator version; byte sizes, the seed, and schema metadata are still to be
added. Repeated measurements across separate server processes are also needed
before adopting latency gates. Keep this suite on demand;
`scripts/verify.ps1` exercises functional behavior but does not establish
performance distributions.
