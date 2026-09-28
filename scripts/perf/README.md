# Local performance suite

The fixture builder creates a new, isolated aicp data directory with deterministic
historical Items, content versions, Runs, Watcher results, events, and receipts.
It refuses an existing directory. The browser and heartbeat scripts require the generated
`perf-manifest.json`, use the loopback server, and refuse an occupied port 7331.
They do not inspect external sources. The seeded historical rows are synthetic;
the heartbeat script performs its measured writes through the public HTTP API.

From the repository root in PowerShell (with Go, Node, web dependencies, and
Playwright Chromium installed):

```powershell
npm --prefix web run build
go build -o dist/aicp.exe ./cmd/aicp
$dataDir = Join-Path ([IO.Path]::GetTempPath()) ('aicp-perf-' + [guid]::NewGuid().ToString('N'))
go run ./scripts/perf/seed.go -data-dir $dataDir -profile mature
node web/perf/measure.mjs --binary dist/aicp.exe --data-dir $dataDir --output (Join-Path ([IO.Path]::GetTempPath()) 'aicp-browser.json') --iterations 5
```

Profiles: `fresh` (100 Items), `mature` (10,000), `large` (100,000), and
`skewed` (20,000 with concentrated revisions and 20% Attention). Run the
browser measurement before the heartbeat measurement on the same fixture,
because heartbeat writes new Items and Runs. For an A/B comparison, seed two
directories with the same profile and run one binary against each. Re-run the
browser command with fresh browser contexts or a new server process to estimate
variation. The JSON contains each journey's elapsed time, browser heap, and
network resource timings.

The bounded heartbeat check creates and updates five Items per selected active
Watcher, repeats unchanged content, applies a user Todo and note, and verifies
that publication preserves that state. It also measures two concurrent readers
while findings are submitted, restarts with an unfinished Run, verifies recovery,
and abandons that Run through the owner API:

```powershell
node web/perf/heartbeat.mjs --binary dist/aicp.exe --data-dir $dataDir --output (Join-Path ([IO.Path]::GetTempPath()) 'aicp-heartbeat.json')
```

Do not treat timings from a few repetitions as stable p95 targets. Compare
matched fixtures and builds on the same machine, inspect the JSON for failed
requests, and repeat across separate server processes before setting a gate.

## Agent-use efficiency

The [agent evaluation spec](../../docs/specs/20260928-agent-efficiency-evaluation-spec.md)
adds short MCP journeys and token accounting. Run the six MCP cases and two CLI
output cases with one command; it creates separate synthetic fixtures and JSON
outputs in a new temporary directory:

```powershell
./scripts/perf/run-agent-eval.ps1
```

To run one case separately:

```powershell
$dataDir = Join-Path ([IO.Path]::GetTempPath()) ('aicp-agent-eval-' + [guid]::NewGuid().ToString('N'))
go run ./scripts/perf/seed.go -data-dir $dataDir -profile mature -attention-percent 20
go run ./scripts/perf/agent-eval -data-dir $dataDir -mode quiet -output (Join-Path ([IO.Path]::GetTempPath()) 'aicp-agent-eval.json')
```

Modes are `quiet`, `backlog` (250 synthetic user events), `reconcile` (one
changed matter and a separate Item sharing its source URL), `multi-interest`
(a broad Watcher), and `partial-retry` (one partial result with idempotent
replay). Use Fresh for the last three quick checks, and matched Mature fixtures
with 1% and 20% Attention to compare summary cost. The evaluator follows every
Interest, Attention, and change cursor, submits a terminal result per selected
Watcher, and checks the resulting state. It refuses data directories without a
performance manifest. The one-matter source corpus and separate answer key are
under `scripts/perf/corpus`.

`web/perf/count-tokens.mjs` uses the pinned `js-tiktoken@1.0.21` package and
`o200k_base` encoding as a **reference proxy**, not a claim about any Codex
model's billed tokens. The evaluator counts MCP `content` and
`structuredContent` separately because the SDK returns both representations;
the actual agent harness determines which reaches the model. It keeps raw
synthetic tool payloads in memory and writes only aggregate JSON. Tool
definitions are counted once. The `duplicate_context_probe` rows are diagnostic
subsets of continuation responses and must not be added to response totals.
External source text is counted separately in `reconcile` mode.

These deterministic traces verify protocol costs and expected state. They do
not establish that a model selected the right Item or report provider usage.
The [first local results](../../docs/perf/20260928-agent-efficiency-baseline.md)
record the current reference baseline and its limits.

The CLI cases build `aicp`, then run matched Mature fixtures with 20% Attention
and 250 synthetic changes. They count exact stdout from both `--json` and the
default pretty JSON rendering, including its shortened display IDs. The sample
covers status, settings, Interest and Watcher lists/details, Run and proposal
lists, one Item detail, all 20 full Attention Item list pages, `brief` preview,
`run start`, every captured Interest and Attention continuation via `brief
--cursor`, the remaining change pages, three `run submit` calls, `run finish`,
and `run get`. Each output is counted by command category; only aggregate
counts are saved. The evaluator asserts that the CLI consumed the full captured
snapshot. CLI `item list` pages return current full Items, so their cost is
reported separately from the compact Run packet.

The first packet keeps a 50-entry limit. Captured Interest and Attention
continuations allow up to 150 entries with a 64 KiB item-array target; a single
oversized entry is still returned so paging can progress. Attention entries
omit user-state and acknowledgement versions, and cap title, summary, and Interest reason text
at 256, 512, and 256 UTF-8 bytes. `truncated_fields` marks entries that need a
full `get_item` read when their omitted text matters. The source Item remains
unchanged, and the captured Run snapshot and retry receipt retain full text.
