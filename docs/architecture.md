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

Item-local delegations and existing-Item work updates are owned by
`internal/app/item_work.go` and the shared Item versioning path. The
[delegation contract](specs/20261002-agent-delegation-and-receipts-spec.md)
describes external-agent continuation and bounded recovery. aicp stores handoffs;
the primary agent launches and communicates with executors outside this runtime.

Structured report blocks, immutable review formats, image evidence, and saved
human answers are owned by `internal/app/review_*.go`. The
[review format contract](specs/20261004-item-review-formats-and-user-answers-spec.md)
defines interleaving and answer applicability. HTTP provides local PlantUML
rendering and portal-only submission; CLI/MCP expose discovery, registration,
uploads, and reads. The portal's `report-content.tsx` renders reviews and diagrams.
PlantUML needs a compatible Java runtime (the tested JAR requires Java 11 or
newer) on PATH and an absolute `AICP_PLANTUML_JAR` path. When unavailable, source
remains readable. No external rendering service is contacted.

At startup the server renders a small diagram through the same bounded sandbox
used for previews. `plantuml_available` is true only if that check succeeds;
`plantuml_setup` provides platform-specific instructions through HTTP, CLI, and
MCP capability discovery. After changing the installation or environment, restart
the server. The startup check has a ten-second timeout. Agents can use these
instructions to request dependency setup.

On macOS, [Homebrew](https://formulae.brew.sh/formula/plantuml) installs PlantUML,
OpenJDK, and Graphviz:

```sh
brew install plantuml
export PATH="$(brew --prefix openjdk)/bin:$PATH"
export AICP_PLANTUML_JAR="$(brew --prefix plantuml)/libexec/plantuml.jar"
```

On Windows, install Java 11 or newer (for example,
[Eclipse Temurin](https://adoptium.net/temurin/releases/)), download the
[official PlantUML JAR](https://plantuml.com/download), and configure the server's
PowerShell session with the actual installation paths:

```powershell
$env:PATH = 'C:\path\to\java\bin;' + $env:PATH
$env:AICP_PLANTUML_JAR = 'C:\path\to\plantuml.jar'
```

These are operator setup commands. The portal displays guidance; it does not run
package managers. The renderer uses Java and the JAR directly, including on
Homebrew installations, so a `plantuml` command alone is insufficient.
See [PlantUML installation prerequisites](https://plantuml.com/starting) for
Graphviz requirements on other platforms.

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
| Go source | `golangci-lint fmt`, then `scripts/check-go.ps1` | `go vet ./...`, `go test ./...`, and `go build ./cmd/aicp` |
| Application or migration | `go test ./internal/app` or `go test ./internal/store` | `scripts/check-go.ps1`, `go vet ./...`, `go test ./...`, and the affected public adapter test |
| HTTP, CLI, or MCP | `go test ./internal/httpapi`, `go test ./cmd/aicp`, or `go test ./internal/mcpserver` | `go test ./...`; use the demo for a run-flow change |
| Portal | `npm --prefix web run build` | Build the executable, then `npm --prefix web run test:e2e` |
| Cross-adapter run or Item state | Relevant Go package tests | `scripts/demo.ps1` and `scripts/verify.ps1` |

For normal iteration use [the verification runner](../scripts/testing/README.md):

```powershell
./scripts/test.ps1 -Level unit -Feature reviews
./scripts/test.ps1 -Level component -Feature reviews
./scripts/test.ps1 -Level system -Concern accessibility -Feature reviews
./scripts/test.ps1 -Level system -Scenario review-roundtrip
./scripts/test.ps1 -Level system -Concern contract
./scripts/test.ps1 -Suite change -Feature reviews -Plan
```

The runner prepares locked dependencies, frontend assets, the executable and
Chromium as needed, validating input and output digests before reuse. System
results always execute again. Fixtures own their server and database; port 7331
must be free. Each E2E journey starts with a fresh database. Focused UI cases share
one worker server. `scripts/demo.ps1` remains a standalone CLI demonstration,
including its optional executable parameter; the automated inspection journey
retains its two-cycle assertions and adds portal-owned notes and request replay.

`scripts/check-go.ps1` is the focused Go quality gate. It checks the pinned
golangci-lint version, gofmt/goimports output, and the configured standard,
Staticcheck, Go vet, `errcheck`, and targeted readability rules. Install the version in
`.golangci-lint-version` using the
[official local installation instructions](https://golangci-lint.run/docs/welcome/install/local/)
before running it. `scripts/verify.ps1` is the complete
Windows local gate through `test.ps1 -Suite main`: preparation, typecheck, Go
formatting/lint/vet, indexed Go/harness tests, focused browser checks, and all core
journeys. `.github/workflows/go-quality.yml` runs that complete suite on pushes
to `main` and pull requests. `.github/workflows/release.yml` adds required pinned
PlantUML rendering, Windows package smoke checks, and Linux verification for
release tags or a manual release run. Linux CI additionally runs Go tests with
`-race`. The demo and browser tests use
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

The implemented [testing levels, execution suites, and journey-record contract](specs/20261005-layered-verification-and-journey-goldens-spec.md)
defines unit, component/integration, and system testing levels; independent
concerns and execution suites; fast feature selection; and human-reviewable
expected/observed records. [Warm measurements](perf/20261005-verification-baseline.md)
record local iteration costs. `test-output/latest.json` locates the newest attempt;
its summary links numbered journey reports. `scripts/propose-goldens.ps1` copies
concrete evidence into ignored output for owner review without changing accepted
expectations. Visual golden adoption remains a human decision.
