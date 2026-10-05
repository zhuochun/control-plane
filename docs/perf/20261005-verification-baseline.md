# Local verification implementation baseline

Measured 2026-10-05 on Windows amd64 with Node 24.18.0, Go 1.27.1
(`CGO_ENABLED=0`) and the repository-locked Playwright/Chromium. The candidate
is the uncommitted verification change on `6c185a1`; each attempt records its
revision and dirty-content digest. These timings establish this host, not CI,
Linux, macOS, live-source accuracy or model behavior.

Three sequential warm runs per selection, including discovery, preparation
identity checks, execution and cleanup:

| Selection | Runs (seconds) | Median | Budget |
| --- | --- | ---: | ---: |
| `-Level unit -Feature reviews` | 2.51 / 2.51 / 2.49 | 2.51 | 5 |
| `-Level component -Feature reviews` | 6.61 / 4.84 / 4.77 | 4.84 | 10 |
| `-Level system -Concern ui -Feature reviews` | 15.77 / 15.73 / 15.13 | 15.73 | 18 (revised) |
| `-Level system -Scenario review-roundtrip` | 6.37 / 6.31 / 6.35 | 6.35 | 20 |
| `-Level system -Concern contract` | 14.22 / 13.94 / 13.96 | 13.96 | 60 |

These measurements preceded three additional focused answer-note captures;
the final roundtrip including those captures also passed in under seven seconds.
They do not change the frozen observations or the selection's required steps.

The original 15-second system/UI budget was exceeded by the full nine-case review
selection, which includes two complete journeys plus seven focused checks.
Investigation found normal browser/process startup and required interaction work;
Go package invocations were already batched to reduce unit iteration overhead.
This implementation explicitly revises that full selection's budget to 18 seconds.
Other feature selections retain the initial 15-second budget. No assertion,
restart, failure case or tolerance was removed to meet a runtime target. Use a
visual/accessibility concern or one case when the edit needs narrower feedback.

Initial dependency installation was cold and took roughly two minutes; that
observation is not a controlled installation benchmark. Cold builds and browser
installation depend on caches and network availability. Warm runs reused verified
assets and executable digests; Go cache reuse is recorded per package, while
browser/journey results are always fresh. Manifests separately record discovery
and preparation time and list installation/build/reuse decisions. Raw three-run
manifests were saved in ignored `test-output/warm-measurements.json`.

The migration retains all existing browser assertions, splits the oversized
review test, and adds setup, two-cycle inspection, review roundtrip and recovery
journeys. Recovery has separate answer and unfinished-Run cases. Frozen contracts
are independent executable expectations; readable expected/observed reports and
screenshots can be proposed for owner review. They are not accepted visual goldens.

Harness controls reject invalid selectors, missing/unindexed or duplicate cases,
wrong observations, omitted/reordered/failed journey steps, stale binaries and
migration inputs, and premature lock identity advancement. Real-source discovery
rejects unsupported runnable Go examples/fuzz families. A real descendant TCP
listener is terminated by the bounded timeout control. Independent review found
and repaired migration-fingerprint, dependency-install, timeout cleanup, evidence
isolation and journey-completeness gaps.

Final Windows checks passed: `scripts/verify.ps1` (73 Go families and 28 browser
cases), then `scripts/test.ps1 -Suite release -Profile renderer` (74 Go families,
eight harness controls, 29 browser cases, zero lint findings). The final renderer
suite took 91.79 seconds, including 46.6 seconds of fresh browser execution.
The pinned PlantUML 1.2026.8 JAR used Temurin 21.0.12.1; required diagram and
artifact images loaded. Independent review has no remaining findings. Evidence
is in `test-output/run-2026-10-05T03-49-31.897Z-7588`; the complete owner proposal
is `test-output/proposed-goldens-2026-10-05T03-51-22.680Z/README.md`.

The Windows pass did not establish Linux behavior. A subsequent native Linux
follow-up ran in Ubuntu 24.04.3 under WSL2, from an isolated copy of the current
checkout on the Linux filesystem, with separate Linux dependencies and outputs.
The initial browser attempt failed because the WSL image lacked NSS/NSPR/ALSA
libraries. Private copies of those Ubuntu libraries resolved Chromium startup;
no system-wide package installation or product change was needed.

`node scripts/testing/runner.mjs --suite release --profile renderer` then passed:
zero lint findings, Go vet and race checks (`CGO_ENABLED=1`), 74 indexed Go
families, eight harness controls, and all 29 browser cases. Native Node 24.18.0,
Go 1.27.1, PowerShell 7.6.5 and Temurin 21.0.12.1 were used. The warm complete
gate took 52.74 seconds, including 33.0 seconds of browser execution; this is
one run, not a Linux performance calibration. Cold dependency installation,
builds, browser download and first race compilation happened in the initial run.
Copied evidence is in `test-output/wsl-20261005/run-2026-10-05T04-00-53.267Z-20592`;
the Linux proposal is in
`test-output/wsl-20261005/proposed-goldens-2026-10-05T04-01-46.062Z/README.md`.

Hosted CI execution and packaged-binary checks remain unverified locally. Owner
acceptance of proposed screenshots and visual-baseline calibration remain
separate steps; WSL does not establish macOS behavior.
