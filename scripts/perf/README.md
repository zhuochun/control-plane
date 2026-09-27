# Local performance suite

The fixture builder creates a new, isolated aicp data directory with deterministic
historical Items, content versions, Runs, Watcher results, events, and receipts.
It refuses an existing directory. Both measurement scripts require the generated
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
