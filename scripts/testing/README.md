# Verification selections and evidence

Run from the repository root with Go (go.mod), Node 24/npm, and PowerShell.
The runner installs locked dependencies and Chromium only when their identity is
missing or changed. Fresh Go embeds require frontend assets; preparation builds
them when necessary even for a selected Go test. Initial setup/build is recorded
separately from warm iteration. No system/UI/E2E result is cached.

```powershell
./scripts/test.ps1 -Level unit -Feature reviews
./scripts/test.ps1 -Level component -Feature reviews
./scripts/test.ps1 -Level system -Concern accessibility -Feature reviews
./scripts/test.ps1 -Level system -Scenario review-roundtrip
./scripts/test.ps1 -Level system -Concern contract # All four core journey families
./scripts/test.ps1 -Suite change -Feature reviews -Plan
./scripts/test.ps1 -Suite change -Feature reviews
./scripts/test.ps1 -Suite main
./scripts/verify.ps1
```

Levels describe boundaries; concerns describe checked properties. E2E journeys
remain system tests. `component` combines component/integration selection while
case metadata preserves each boundary. Suites decide scheduling. A concern can
span a focused UI check and an E2E journey; `-Concern ui -Feature reviews` therefore
includes both. Use the accessibility or visual concern for narrower feedback,
or native Playwright `--grep @case:review-pending` after preparation. A narrow
green result proves only the printed selection, not the complete product.

`index.mjs` classifies Go file families, isolated overrides and required families.
Browser tests carry native `@case`, `@feature`, `@concern`, `@scenario`, and
`@profile` tags. New unclassified files/cases, missing required families, unknown
selectors and empty selections fail. Runnable Go examples/fuzz families are
currently explicitly rejected pending an indexed runner; they cannot disappear
silently. Benchmarks and real-agent/performance scripts remain on-demand evidence.
There is no automatic changed-file selection yet: local selection is explicit,
and PR/main CI discovers and runs complete sets.

Every journey has a fresh database; focused UI cases share their owned worker
server. Port 7331 must be free, or set `AICP_TEST_PORT` to a free loopback port
to preserve a running installation. `aicp serve --port` retains loopback-only
binding and defaults to 7331. The fixtures refuse an occupied port, prove the
owned child emitted readiness, capture its binary digest/PID, and stop only owned
children. Restart steps use a fresh PID and consumer. Optional renderer variables
are cleared for core tests, and the runner clears any inherited executable override.
The source identity is fenced before discovery and after execution. Failed journey
data/logs remain available for diagnosis. Every frozen step must execute exactly
once, in order, and pass; an incomplete record fails even if the test returns early.

`test-output/latest.json` points to the newest isolated attempt. Open its
`summary.md`, then the linked numbered journey `report.md` files. Reports show
expected/observed observations, labels and IDs, and screenshots. Raw protocol,
traces, and process metadata supplement those reports. Keep one successful local
bundle; failed bundles persist until diagnosed. CI uploads bundles for 14 days
without changing the test exit status. Missing dependencies or lost isolation
are not run/inconclusive and nonzero, never passing exclusions.

The JSON files under `tests/e2e/contracts` are frozen executable expectations.
They are not accepted visual baselines. To produce an owner-review bundle:

```powershell
./scripts/propose-goldens.ps1
# Or retain a concrete prior attempt explicitly:
./scripts/propose-goldens.ps1 -Bundle test-output/run-<timestamp>-<pid>
```

Proposals stay in ignored `test-output/proposed-goldens-*`. Normal runs never
rewrite expectations. The proposal command refuses contracts changed since the
recorded execution. Human acceptance precedes committed golden adoption; no
automatic accept/update command exists. Screenshots are pinned to Chromium,
viewport, locale and timezone, but remain human review evidence until platform
fonts, masks and tolerances are calibrated.

Release uses the existing Linux/Windows/package jobs plus a required renderer:

```powershell
./scripts/testing/provision-renderer.ps1
# Supply compatible Java (11+; release CI uses Temurin 21) on PATH.
./scripts/test.ps1 -Suite release -Profile renderer
```

`renderer.json` pins the PlantUML JAR and checksum. The provisioner validates even
an existing download. Release never substitutes an unavailable provider or a
skipped case for renderer success. The local suite establishes the current host;
other operating systems and packaged binaries are verified by their release jobs.

See [the local warm baseline](../../docs/perf/20261005-verification-baseline.md)
for three-run measurements and the full review UI set's documented 18-second
budget. Narrow visual/accessibility selections remain available for faster edits.
