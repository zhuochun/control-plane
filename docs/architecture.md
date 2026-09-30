# Current architecture and verification map

This is a guide to the implemented repository. For domain meaning, read the
[glossary](glossary.md) and the current
[Watcher–Interest specification](specs/20260924-watcher-interest-model-spec.md).
The older MVP specifications describe earlier behavior where they conflict with
that specification. Check the code and migration when changing a contract.
The [agent-led onboarding journey](specs/20260927-agent-led-onboarding-journey-spec.md)
describes first and returning use, including the derived setup indication.

## Runtime path

```text
External scheduler -> agent with its own source tools
                         | CLI or MCP stdio
                         v
                   local HTTP API <-> portal
                         |
                         v
                    application rules
                         |
                         v
                  SQLite store and migrations
```

`cmd/aicp` starts the loopback server and provides local CLI commands. Ordinary
CLI commands use `internal/client` to call the server. `internal/mcpserver` is
also an HTTP client of that server; it does not open the database. The portal
in `web/src` calls the same API. `internal/httpapi` owns HTTP routing, decoding,
and response handling; `internal/app` owns shared operations and transaction
boundaries. `internal/store` owns the SQLite connection, directory lock, and
versioned migrations in `internal/store/migrations`. The server embeds the
portal built into `web/dist` through `web/assets.go`.

The external scheduler and source inspection live outside this repository's
runtime. aicp stores configuration, run snapshots, results, Item state,
checkpoints, and history. A running inspection uses its captured configuration;
later configuration edits apply to later runs. Source results must preserve
user-owned Item state. See the current specification for the exact rules.

## Find the owner of a change

| Change or question | Start here | Follow through |
| --- | --- | --- |
| Watcher, Interest, Item, or Run meaning | `docs/glossary.md`, current Watcher–Interest specification | `internal/app`, migration, affected adapters |
| Database shape or startup migration | [Migration practices](migrations.md), `internal/store/migrations` | `internal/store/store.go`, `internal/store/store_test.go`, `internal/app` |
| Command, publication, or user-state rule | `internal/app` | `internal/app/*_test.go`, HTTP/CLI/MCP tests |
| HTTP behavior | `internal/httpapi` | `internal/httpapi/server_test.go`, client and portal callers |
| CLI or MCP behavior | `cmd/aicp`, `internal/mcpserver`, `internal/client` | Corresponding tests and HTTP contract |
| Portal behavior | `web/src` | `web/tests`, `web/assets_test.go` |
| End-to-end run behavior | `examples/heartbeat-prompt.md`, `scripts/demo.ps1` | CLI, API, application, and portal paths |

The specification is intended behavior; tests and code show the implemented
behavior. A green test alone does not resolve a conflict between them. Inspect
all affected public adapters before changing shared behavior.

## Verification path

Run commands from the repository root. Development uses Node.js 24 and the Go
version in `go.mod`. Check that `node`, `npm`, and `go` are available before
running a path that needs them. `web/dist` is ignored but embedded by Go, so
build the portal before Go tests or builds on a clean checkout:

```powershell
npm --prefix web ci
npm --prefix web run build
```

Choose the smallest check that can expose the affected behavior, then run a
broader check before handing off a change. These commands select existing
tests; they do not replace public-interface verification for a behavior change.

| Affected path | Focused feedback | Broader evidence |
| --- | --- | --- |
| Application or migration | `go test ./internal/app` or `go test ./internal/store` | `go test ./...` and the affected public adapter test |
| HTTP, CLI, or MCP | `go test ./internal/httpapi`, `go test ./cmd/aicp`, or `go test ./internal/mcpserver` | `go test ./...`; use the demo for a run-flow change |
| Portal | `npm --prefix web run build` | Build the executable, then `npm --prefix web run test:e2e` |
| Cross-adapter run or Item state | Relevant Go package tests | `scripts/demo.ps1` and `scripts/verify.ps1` |

For browser tests, build `dist/aicp.exe` first and install Chromium once with
`npx --prefix web playwright install chromium`. Playwright starts an isolated
test server and data directory; it does not reuse a server already bound to
`127.0.0.1:7331`. `scripts/demo.ps1` also starts its own isolated server and
exercises two run cycles through the public CLI. Stop another local aicp server
before either check if it owns that port.

`scripts/verify.ps1` is the complete Windows local gate: locked web install,
typecheck, portal build, Go vet and tests, executable build, browser tests, and
the fixture demo. `.github/workflows/release.yml` verifies Windows and Linux
for release tags or a manual release run; it is not a pull-request gate. Linux
CI additionally runs Go tests with `-race`. The demo and browser tests use
local fixtures and an isolated database, so they do not establish live access
to external sources.

For accumulated-data latency, use the
[stress-test specification](specs/20260928-long-term-performance-test-spec.md)
and isolated [performance suite](../scripts/perf/README.md); compare with the
[2026-09-28 local baseline](perf/20260928-perf-baseline.md). It is an on-demand
measurement, not part of `scripts/verify.ps1`.
For agent packet costs and progressive detail reads, see the
[agent evaluation spec](specs/20260928-agent-efficiency-evaluation-spec.md) and
[first reference baseline](perf/20260928-agent-efficiency-baseline.md).
The required Run context, live brief, and compact history contract is recorded in the
[Run context and brief spec](specs/20260930-run-context-and-brief-spec.md).
It compares former and current returns, including optional captured Attention
and bounded lookup before reconciliation.
The [2026-09-30 evaluation](perf/20260930-run-context-baseline.md) records
verification and reference payload costs under the current reading contract.

When reporting verification, name the exact command, result, and skipped
surfaces. A package test does not establish portal behavior; a portal build does
not establish API behavior; the offline demo does not establish live source
inspection.
