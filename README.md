# aicp

**aicp** is a small local control plane for agent work. Agents inspect sources
with their existing tools and save source-backed findings. You decide what
becomes a Todo, when to be reminded, and whether proposed monitoring changes are
accepted.

The application is one Go executable with an embedded React portal and a local
SQLite database. It binds only to `127.0.0.1:7331`. Source credentials and agent
scheduling stay outside aicp.

## Domain model

The durable configuration and work relationships are:

```text
Interest = why it matters
    └── Watch = where and how to inspect it
Agent run = one inspection attempt across up to 20 due Watches
Item = a durable finding tied to an Interest, optionally to a Watch;
       later runs can update the same Item
```

An **Interest** is the durable purpose: a title, free-form instructions, and
an `active`, `paused`, or `deprecated` lifecycle state. A **Watch** attaches a
primary source to an Interest and describes its locator, inspection
instructions, interval, lookback window, and lifecycle state. Several Watches
can share one Interest.

An **Item** is retained work or knowledge produced by a run. Its kind is
`note`, `report`, `task`, or `outcome`; all kinds can contain Markdown content,
source references, context, Todo state, reminders, acknowledgement state, and
user notes. A **Proposal** is an agent-suggested creation, update, or
deprecation of one Interest or Watch. Proposals are reviewed by a person and
do not change configuration automatically.

## Build and run

Development requires Node.js 24 and the Go version pinned in `go.mod`. The
installed executable has no Node.js dependency.

```powershell
npm --prefix web ci
npm --prefix web run build
go build -trimpath -o dist/aicp.exe ./cmd/aicp
.\dist\aicp.exe init
.\dist\aicp.exe serve
```

Open <http://127.0.0.1:7331>. Data defaults to the operating system's user
configuration directory under `control-plane`. Set `AICP_DATA_DIR`, or pass
`serve --data-dir <directory>`, to use another location. `init` prepares the
database and defaults; ordinary commands call the running server.

```powershell
.\dist\aicp.exe doctor
.\dist\aicp.exe config set Asia/Singapore
.\dist\aicp.exe interest create --file interest.json --json
.\dist\aicp.exe watch create --file watch.json --json
.\dist\aicp.exe brief --json
.\dist\aicp.exe run start
.\dist\aicp.exe run submit <watch-id> --file findings.json
.\dist\aicp.exe run finish --summary "Inspected selected Watches"
```

Configuration and complex finding/item payload files are ordinary JSON. The
normal run start and finish commands need no files. Use `aicp <command> --help`
for the accepted shape. Descriptive instructions and reports remain free-form
Markdown; JSON fields cover only identity, revisions, scheduling, source
references, and user actions needed for safe operation.

## Agent connection

Start the HTTP server first. Register the same executable as a local MCP stdio
adapter, using an absolute path in a real installation:

```powershell
codex mcp add aicp -- C:\absolute\path\aicp.exe mcp --server http://127.0.0.1:7331
```

`examples/heartbeat-prompt.md` is the reference run contract.
`examples/heartbeat.ps1` and `examples/heartbeat.sh` show how an external
scheduler can invoke a configured Codex harness with overlap protection. See
`examples/scheduling.md`. aicp records when Watches are due; it does not launch
an agent itself.

### How an agent run works

aicp is the local control plane, not the source connector or scheduler. An
external heartbeat starts the agent, and the agent uses its existing tools to
inspect sources:

1. Call `start_run`. aicp returns each active Interest once, bounded pages of
   unarchived Attention summaries, due Watch snapshots, contexts, and the
   captured change range.
2. Consume all continuation cursors, then inspect only the selected Watches
   with the agent's Slack, browser, GitHub, Drive, or other tools. aicp never
   receives source credentials or fetches those systems.
3. Call `submit_watch_findings` once per selected Watch. The result records
   coverage, limitations, the next cursor, and zero or more Item upserts. If a
   finding is relevant to an Interest but not a selected Watch, use
   `upsert_item` without a Watch. Stable dedupe keys and expected content
   versions let the agent update findings instead of creating duplicates.
4. Call `finish_run` only after every selected Watch has a terminal result.
   Successful coverage advances the Watch checkpoint and next due time; failed
   or partial coverage remains eligible for a later run.
   If the external agent cannot finish, review the active run in Activity and
   explicitly abandon it with a reason. Submitted findings and successful
   checkpoints remain; unreported Watches stay due and captured changes replay.
5. Review the resulting Attention items. You can open sources, acknowledge
   findings, set Todo or Done, add reminders, and accept or reject proposals.
   The next run receives those local changes as context.

The server does not promise that an inspection is running merely because a
Watch exists. Until an external runner connects, the portal reports that no
agent run has been received.
Monitoring shows each active Watch's latest inspection and next due time;
Activity shows the active run and the number of due Watches. Due work beyond
the 20-Watch run limit remains visible for a later heartbeat.

## Demo and verification

The offline demo starts an isolated server, completes two fixture-backed run
cycles through the public CLI, applies a Todo and reminder between cycles, and
checks that later agent content preserves them:

```powershell
npm --prefix web run build
go build -o dist/aicp.exe ./cmd/aicp
.\scripts\demo.ps1
```

Run the complete local check with `scripts/verify.ps1`. It installs locked web
dependencies, type-checks and builds the portal, runs Go tests and vet, builds
the executable, runs Playwright, and then runs the demo. Install Chromium once
with `npx --prefix web playwright install chromium` when needed.

### Simulate OKR updates

With `aicp serve` running, seed five different outcome metrics and then publish
their next-period values through the normal run/publication path:

```powershell
.\scripts\okr-simulation.ps1 -Phase baseline
.\scripts\okr-simulation.ps1 -Phase update
```

The simulator uses its own stable deduplication keys, so the second command
updates the same metric items and preserves any local Todo or reminder state.

## Backup and restore

Stop `aicp serve` before copying its data directory. The database uses SQLite
WAL mode, so copying only the live `.db` file is not a consistent backup. To
restore, keep the server stopped, replace the whole data directory with the
saved copy, and restart. aicp refuses a database created by a newer unsupported
schema instead of resetting it.

## Release archives

GoReleaser builds macOS amd64/arm64, Linux amd64/arm64, and Windows amd64
archives with `checksums.txt`:

```powershell
goreleaser release --snapshot --clean
```

Snapshot artifacts appear under `dist/` and are not published. The checked-in
workflow creates a draft release for version tags and supports a manual snapshot
run. Local verification and package smoke tests run on Windows; CI verifies
Windows and Linux. macOS archives are cross-compiled until a native macOS runner
is added. Generated assets, databases, credentials, and archives are ignored by
Git.

The specifications are in [`docs/specs`](docs/specs). The deterministic fixture
demo verifies the local application path; it does not claim live Slack, Drive,
or other connector access.
