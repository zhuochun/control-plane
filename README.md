# aicp

**aicp** is a small local control plane for agent work. Agents inspect sources
with their existing tools and publish source-backed reports. You decide what
becomes a Todo, when to be reminded, and whether proposed monitoring changes are
accepted.

The application is one Go executable with an embedded React portal and a local
SQLite database. It binds only to `127.0.0.1:7331`. Source credentials and agent
scheduling stay outside aicp.

## Build and run

Development requires Node.js 24 and the Go version pinned in `go.mod`. The
installed executable has no Node.js dependency.

```powershell
npm --prefix web ci
npm --prefix web run build
go build -trimpath -o dist/aicp.exe ./cmd/aicp
.\dist\aicp.exe serve
```

Open <http://127.0.0.1:7331>. Data defaults to the operating system's user
configuration directory under `control-plane`. Set `AICP_DATA_DIR`, or pass
`serve --data-dir <directory>`, to use another location. Only `serve` opens the
database; every other CLI command calls the running server.

```powershell
.\dist\aicp.exe doctor
.\dist\aicp.exe config set Asia/Singapore
.\dist\aicp.exe interest create --file interest.json --json
.\dist\aicp.exe watch create --file watch.json --json
.\dist\aicp.exe brief --json
```

Request files are ordinary JSON. Use `aicp <command> --help` for the accepted
shape. Descriptive instructions and reports remain free-form Markdown; JSON
fields cover only identity, revisions, scheduling, source references, and user
actions needed for safe operation.

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
