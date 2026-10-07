# Development and verification

Build instructions are in the [README](../README.md#try-the-demo-or-build-from-source).
The [architecture map](architecture.md) identifies implementation owners and
focused verification commands. All commands below run from the repository root.

## Verify the application

The offline demo starts an isolated server, completes two fixture-backed Run
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
with `npx --prefix web playwright install chromium` when needed. The
[verification runner](../scripts/testing/README.md) supports focused suites.

The deterministic demo verifies the local application path; it does not
establish live Slack, Drive, or other source access. The
[product-team showcase](showcase.md) is a separate presentation dataset.

## Simulate outcome updates

With `aicp serve` running, seed five outcome metrics, then publish their
next-period values through the normal Run/publication path:

```powershell
.\scripts\okr-simulation.ps1 -Phase baseline
.\scripts\okr-simulation.ps1 -Phase update
```

The simulator uses stable deduplication keys, so the second command updates the
same Items and preserves local Todo or reminder state.

## Package releases

GoReleaser builds macOS amd64/arm64, Linux amd64/arm64, and Windows amd64 archives
with `checksums.txt`:

```powershell
goreleaser release --snapshot --clean
```

Snapshot artifacts appear under `dist/` and are not published. The checked-in
workflow creates a draft release for version tags and supports a manual snapshot
run. Local verification and package smoke tests run on Windows; CI verifies
Windows and Linux. macOS archives are cross-compiled until a native macOS runner
is added. Generated assets, databases, credentials, and archives are ignored by Git.
