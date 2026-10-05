# Testing levels, execution suites, and reviewable journey records

- Date: 2026-10-05.
- Status: Implemented verification contract. The owner authorized the harness and
  existing-test refactor. Local measurements and verification limits are recorded
  in [the implementation baseline](../perf/20261005-verification-baseline.md).
- Source snapshot: `6c185a1` (structured Item reviews and UX repairs).
- Uses software-verification strategy and execution modes; implementation evidence
  remains separate from the normative contract. Golden acceptance is still human-owned.
- Preserves the [architecture boundaries](../architecture.md),
  [Watcher–Interest model](20260924-watcher-interest-model-spec.md),
  [Run context](20260930-run-context-and-brief-spec.md), and
  [review answer contract](20261004-item-review-formats-and-user-answers-spec.md).

## Intended outcome

A developer or code agent can run the smallest relevant test set while changing
one feature, then increase confidence at later stages through evidence at component
and system boundaries, including focused UI checks and complete product journeys.
Human reviewers can inspect expected and observed
journeys, identify an incorrect expectation or confusing experience, and give
feedback using stable case and step identifiers.

Testing level describes the exercised boundary, testing concern describes the
property being checked, and execution suite describes when checks are selected.
Passing a wider-scope test does not replace a
failed narrower invariant or establish complete product, accessibility,
external-source, or model correctness. Product behavior and release authority
remain unchanged.

## Evidence and gaps before implementation

| Current surface | What it exercises | Gap this change addresses |
| --- | --- | --- |
| Go package tests | Application/store rules, HTTP adapters, CLI commands, and MCP contracts, including SQLite and in-memory MCP transport | No common feature selection or level inventory; these are a mixture of unit and component/integration checks |
| `web/tests/{attention,reader,reviews}.spec.ts` | Real browser and complete server, mostly API-arranged Items followed by portal interaction | System-level UI tests; many exercise one feature rather than a complete cross-consumer journey, and selection needs to express that scope |
| `scripts/demo.ps1` | Shipped executable, two CLI inspection cycles, human-state preservation | Separate orchestration and output; no combined MCP/portal/restart journey |
| Performance/agent-evaluation scripts | Synthetic workloads, protocol traces, restart and partial-retry cases | Evidence can inform fixtures; latency/load and real-model claims remain separate |
| `scripts/verify.ps1` | Dependency install, build, Go checks, browser tests, CLI demo | Expensive as an inner-loop entry point; repeats installation and builds |
| Pull-request Go quality workflow | Web build, Go formatting/lint/vet/race tests | Does not currently execute browser/journey tests; release workflows do execute browser tests |

The preceding system-level UI run passed 22 browser cases in about 26 seconds. This is
one Windows execution, not a portable timing guarantee. Existing runtime evidence
is reusable only for unchanged exercised behavior and environment.

## Testing levels and acceptance

| Level | Primary question | Method and boundary | Typical run point |
| --- | --- | --- | --- |
| Unit (`unit`) | Does one isolated function or module satisfy its rules? | Pure validation/hash/selection rules or an isolated module with controlled dependencies | Every relevant edit |
| Component / integration (`component`) | Does a component work with its dependencies, and do cooperating components agree? | Application + SQLite component tests; HTTP handlers + application; MCP adapter + HTTP API integration | During a feature change and before system checks |
| System (`system`) | Does the complete built application satisfy the exercised behavior through supported interfaces? | Feature-focused portal interaction and complete E2E journeys using the executable, CLI/MCP stdio, persistence and fresh-process reads | During UI iteration when relevant, and coherent-change/CI/release gates |
| Acceptance testing and owner review | Does the product meet agreed user needs? | Agreed acceptance criteria plus human review of expected/observed journeys, screenshots and golden changes; may reuse system evidence | Initial golden adoption and changed user expectations |

`component` is the runner's combined selection for component/integration evidence;
each case records whether it exercises one component with dependencies or an
integration between components. Unit cases remain separate. Process boundaries
and test language do not decide the level: an in-process application + SQLite test
is not an isolated unit test, and a short browser case against the complete server
is still a system test. Scope and fidelity are recorded explicitly.

E2E denotes a complete journey within system testing, not a separate mandatory
level. A focused system UI test and a broad product journey can have the same
level while answering different questions. Acceptance review is not inferred
from an automated pass or from the use of a golden file. Owner acceptance can reuse
recorded evidence without rerunning unchanged checks, but remains an explicit
decision about user expectations rather than release approval.

Keep Go tests beside their owners. Add a frontend unit runner only when a concrete
nonvisual rule needs it; this change does not mandate another test framework.
System UI checks use the existing browser tooling. API-assisted setup is acceptable
for a feature-focused interaction claim. A journey must use the supported consumer
path for each transition it claims: app-method calls or database writes cannot
stand in for CLI/MCP publication, human submission, or persistence recovery.

## Concerns and execution suites

Testing concerns are tags independent of level: `functional`, `contract`, `ui`,
`visual`, `accessibility`, `persistence`, `recovery`, and `performance`.
Contract checks assert interface compatibility/semantics; UI interaction, visual
regression, accessibility, performance and recovery identify properties or methods,
not levels. A case can have several concerns. UX evaluation is a broader human
activity supported by these checks, not a `ux` level or an automated guarantee.

Execution suites select evidence for iteration, a coherent change, PR, main and
release. They do not change a case's level. For example:

| Case | Level | Concerns | Useful selection |
| --- | --- | --- | --- |
| Review material hashing | Unit | Functional | Reviews unit iteration |
| Saved answers survive SQLite reopen | Component | Persistence, functional | Reviews component iteration |
| MCP answer retrieval through HTTP | Integration within component selection | Contract, functional | Reviews component/change checks |
| Radio limits and first-error focus against the real server | System | UI, accessibility, functional | Reviews system/UI iteration |
| MCP publish → human answer → restart → fresh MCP read | System / E2E journey | Contract, UI, persistence, recovery | `review-roundtrip`, PR/main/release |

The portfolio builds confidence from distinct boundaries and failure modes. It
does not require every edit to run every level in order, or assume that a system
test provides stronger evidence for every property than a focused unit check.

## Refactor existing coverage without losing obligations

- Classify existing browser cases as system-level UI tests while they exercise
  the complete server. Extend the inbox-to-next-Run and agent-to-human-answer paths
  into named E2E journeys when their missing consumer/process boundaries are added.
  They remain system tests; classification alone adds no evidence.
- Split the large review layout case: keep invalid-field focus, selection limits,
  pending controls, and responsive layout in focused system/UI cases; retain one compact
  publication/answer/restart/retrieval journey for the full path.
- Keep exhaustive validation, answer snapshots, immutability, idempotency,
  applicability, migration, and user-state preservation rules with their Go
  owners, classified as unit or component/integration by their actual dependencies.
  System cases check representative boundary composition.
- Extract shared fixture builders and process lifecycle helpers. Tests retain
  visible intent and assertions; helpers must not hide a journey's critical
  transitions or calculate expected outcomes using production logic.
- Preserve lost-response and stale-draft regressions as named cases. Put broad
  invalid-input permutations at their rule-owning unit/component level, preserving
  representative system UI feedback and full-path retry coverage.
- Reuse the CLI demo's two-cycle fixture as `inspection-two-cycles`. Its existing
  entry point may become a thin invocation of that scenario once equivalent
  executable/CLI coverage exists.
- Before deleting or merging a test, map each assertion to a retained case and
  its level and concerns. A moved claim needs matching executed evidence. Reduce redundant
  setup and repeated permutations, not obligations at distinct boundaries.

## Targeted iteration interface

Introduce one repository entry point, `scripts/test.ps1`, with these proposed
interfaces. These commands are not available in the source snapshot yet:

```powershell
./scripts/test.ps1 -Level unit -Feature reviews
./scripts/test.ps1 -Level component -Feature reviews
./scripts/test.ps1 -Level system -Concern ui -Feature reviews
./scripts/test.ps1 -Level system -Scenario review-roundtrip
./scripts/test.ps1 -Suite change -Feature reviews
./scripts/test.ps1 -Suite main
./scripts/test.ps1 -Suite release
./scripts/test.ps1 -Suite change -Feature reviews -Plan
```

`-Level` and `-Suite` are mutually exclusive. `-Feature` selects a registered
feature; `-Concern` filters concern tags and requires `-Level`. `-Scenario` selects
one E2E journey and requires `-Level system`; it cannot combine with `-Feature`
or `-Concern`. A level/feature selection includes all matching feature-focused
and journey cases; add a concern filter or select one scenario to narrow it.
`-Suite change -Feature reviews` includes the feature's required levels and
associated journeys. Main/release suites reject feature/concern/scenario filters
rather than silently narrowing their required inventory. PR selection is CI-owned
and initially complete; only verified impact selection can narrow it later.
`-Plan` prints selection, prerequisites, build decisions, estimated runtime,
and omitted claims without executing or claiming a pass.

Maintain a small versioned feature/case index containing stable IDs, level,
concern tags, exercised component/integration boundary where applicable,
owning Go packages or browser tags, associated journeys, dependency profile,
and source globs for conservative impact selection. Initial feature names are
`setup`, `items`, `runs`, `reviews`, `delegations`, and `diagrams`.
Shared paths such as Item versioning, HTTP routing, store/migrations, and common
portal components expand selection to all affected registered features.
Unknown paths expand to the complete appropriate suite; unknown level, concern,
suite, feature, or case names are errors. Direct explicit selection remains available for an agent that
already knows its scope.

The plan and result enumerate selected cases and omitted features. Check
discovery before execution. An empty selection or unexpectedly missing case is
a nonzero harness error, never “all passed.” A narrow green result states its
scope and does not claim the full product passed. Initially agents choose the
feature explicitly; changed-file automation is an independent follow-on.

Discovery is bidirectional: declared required cases must exist, and every runnable
Go/browser/journey case must be covered by an indexed feature/level/concern mapping or explicit
dependency-profile exclusion. Go cases may inherit their registered package's
mapping when its tests share a level and concerns; mixed packages use explicit
case/family overrides so isolated units and integration tests are not conflated.
They do not need a duplicate per-function list. Browser tags and journey
IDs carry their mappings. Main/release discovery enumerates the complete runner
inventory before filtering, so a new unmapped test cannot silently disappear.
Unmapped cases are a nonzero inventory error until classified; no full-suite pass
is reported from the indexed subset alone. Treat table-driven subcases as their
declared test family and preserve their executed outcomes in evidence.

Existing `go test -run` and Playwright file/grep commands remain usable. Test
names/tags and the feature index make those selections stable and documented.

## Execution suites and gates

| Suite / run point | Required evidence | Scope and scheduling |
| --- | --- | --- |
| Iteration | Selected unit or component/integration cases; selected system/UI case when changing interaction | Explicit `-Level` commands; avoid the full install/gate |
| Change / PR | Static/build checks, affected unit and component/integration tests, affected system/UI cases, all associated E2E journeys | Explicit feature selection locally; CI initially runs full sets until conservative affected selection is implemented and verified |
| Main | Complete unit and component/integration sets, system feature checks, and core E2E journeys | Full discovery enforced, without duplicating builds/install between levels |
| Release | Main evidence plus Windows/Linux executable/package checks and provisioned renderer profile | Existing release authority unchanged; macOS execution is not claimed until a runner exists |

Wire browser/journey evidence into PR/main CI rather than leaving it only in the
release workflow. Preserve existing Go lint/vet and Linux race obligations.
Changed-file selection may narrow CI only after its discovery/expansion controls
pass; shared/unknown changes then broaden it. Full main/release sets remain fixed.
`scripts/verify.ps1` remains the compatibility entry point for a complete local
gate, delegating to shared execution helpers after installation/static checks.
Do not silently narrow it during the refactor.

## Runtime and environment contract

Use deterministic fixtures and protocol clients for the required journey gate.
The MCP client launches the actual `aicp mcp` child process over stdio and talks
to the actual server. It represents the tool consumer without an LLM, network
account, or external source dependency. Real-agent semantic evaluations are
separate, on-demand evidence with pinned model/prompt/task versions.

The current server is fixed to loopback port 7331. Run system UI/E2E server-owning sets
sequentially on one machine initially. Refuse an occupied port; do not stop an
unrelated process or reuse a user's server. Parallelism within isolated in-process
tests is allowed. A production port/configuration change is outside this spec.

Every journey gets a fresh temporary database and its own owned process set.
Reuse one server within that journey except for its explicit restart step; no
state may flow from another journey. Record PID, binary digest/revision, data
directory, process start and health handshake. After restart, verify a new owned
PID and read persisted state through a fresh consumer session. A health response
from an unrelated or stale process is insufficient.

Default tests clear inherited optional renderer configuration for their child
processes. Real PlantUML belongs to an explicit `renderer` profile with pinned,
checksum-verified JAR and compatible Java. Core review journeys use choices/text
and Mermaid; dedicated diagrams coverage checks actual PlantUML success/failure
and unavailable-provider feedback. A required renderer case with missing
dependencies is **not run**, makes that required suite nonzero, and is shown in
the report. Optional omitted profiles are listed separately from passing cases.

## Keep feedback fast

Warm-run design targets, excluding initial dependency downloads/browser install:

| Selection | Initial target | Evidence needed |
| --- | --- | --- |
| Selected unit feature | 5 seconds | Discovery plus execution time |
| Selected component/integration feature | 10 seconds | Discovery plus execution time |
| Selected system/UI feature | 15 seconds | Owned process startup, readiness, selected cases and cleanup |
| Full review system/UI selection (nine cases) | 18 seconds | Explicit implementation revision; includes two E2E cases and seven focused UI checks |
| One core E2E journey | 20 seconds | Process startup, steps, required restart and cleanup |
| All core journeys | 60 seconds | All four required journey outcomes |

These are engineering budgets, not portable runtime guarantees. Initial
implementation records three successful warm runs on the same named environment,
reports individual and median durations, and identifies cold setup/build time
separately. Budget misses require investigation or an explicit documented contract
revision; never remove assertions, omit a restart, or increase tolerances merely
to obtain a green result. Correctness and runtime-budget results remain separate.

- Install locked dependencies/browser only when missing or their lock/config
  changed. Release/clean CI starts retain locked installation.
- Use Go's incremental build/test cache for valid unit/component evidence,
  identifying reused results separately from fresh execution. System UI cases
  and E2E journeys must actually execute on each requested run and must not inherit
  cached passes.
- Build frontend assets once if their source/config/lock inputs changed; rebuild
  the executable when Go or embedded-asset inputs changed. A small build manifest
  records relevant input digests, tool versions, build configuration, asset digest,
  and binary digest. Missing/mismatched identity forces rebuild; mtime alone cannot
  establish a current binary. Print reuse decisions in the run report.
- Keep fixtures small. Use condition polling, controlled response barriers and
  readiness signals rather than fixed sleeps. No automatic assertion retries
  that turn a flaky failure into a pass. Browser auto-waiting remains allowed.
- Bound every process/step wait, including restart and fault barriers. Initial
  per-journey safety timeout is 60 seconds; total core-run timeout is five minutes.
  Record timeout as failure/inconclusive according to the cause, not a success.

## Required journey contracts

Each case freezes its inputs, sequence, expected values, omission boundaries,
dependency profile, and oracle version before observing a candidate. Maintainers
own execution; the product owner owns disputed behavior/golden changes.

| Claim key and label | Scenario and method | Frozen pass/fail oracle | Boundary / renewal |
| --- | --- | --- | --- |
| **VER-setup — Configuration does not start inspection** | `setup-no-run`: configure settings, Interest and Watcher through CLI; inspect portal and MCP brief | Configured values visible; zero Runs created until explicit start; due/configured state is correct | No real scheduler or source. Renew for setup, due-policy or configuration adapter changes |
| **VER-inspection — Repeated inspection preserves human state** | `inspection-two-cycles`: CLI/MCP fixture publication, portal note/Todo/reminder, second publication of the same matter | Same Item ID; substantive content change advances content version; unchanged replay does not; exact human state preserved; successful result advances expected checkpoint | Deterministic source fixtures, not source-reading quality. Renew for Run/Item/versioning/state contracts |
| **VER-review — Human answers reach the next agent** | `review-roundtrip`: MCP stdio registers/publishes a review; portal submits; server restarts; fresh MCP reads brief/change range/Item; portal corrects and MCP reads current/history | Exact choices/text and snapshots persist; answer event discoverable; applicable answer available directly; no implicit Todo/Done/acknowledgement; one current corrected answer and two immutable history records | No authenticated proof of a human or agent judgment. Renew for report/answer/event/adapter/persistence changes |
| **VER-recovery — Failures remain recoverable** | `recovery`: explicit checkpointed phases for lost committed response/replay and unfinished Run restart/owner recovery | Lost response retry produces one answer and event; conflicting change preserves draft and refuses stale writes; unfinished Run is inspectable after restart and owner can recover/abandon it without corruption | Only named faults. Renew for retry/fencing/Run/restart behavior or harness changes |

For `recovery`, Run restart/recovery and answer replay use independent fixture
subcases under one case family; their results are reported individually. A pass
requires every required subcase, so one long mixed script cannot hide a failure.
Capture expected partial/failed checkpoint behavior as component/integration coverage and
one representative public-consumer negative case as this family expands.

## Human-reviewable golden records

Proposed location per journey:

```text
tests/e2e/goldens/review-roundtrip/
  README.md          # purpose, case/step IDs, rationale and intentional boundaries
  expected.json      # small canonical expected product observations
  desktop.png        # selected reviewed visual checkpoint, if applicable
  mobile.png         # selected reviewed visual checkpoint, if applicable
```

`expected.json` contains business observations, not an entire HTTP response or
database dump. The test case consumes those values through independent assertions.
The README explains the flow without duplicating expected field values as another
authority. Keep orchestration in ordinary test code; do not build a workflow DSL,
generic replay engine, or self-generated oracle from the production implementation.

Example canonical observations for `review-roundtrip`:
the fixture offers `local` (“Local”) and `remote` (“Remote”) approaches. Steps
R1/R2 register/publish; R3 submits Local plus instructions; R4 restarts; R5 reads
through a fresh MCP session; R6 corrects the approach to Remote and reads history.
These illustrate the proposed contract, not execution evidence.

```json
{
  "contract_version": 1,
  "case_id": "review-roundtrip",
  "claim": "VER-review",
  "answers": {
    "approach": { "disposition": "answered", "selected_ids": ["local"] },
    "instructions": { "disposition": "answered", "text": "Keep the rollout small." }
  },
  "after_restart": {
    "same_item": true,
    "answer_count": 1,
    "answer_event_count": 1,
    "applicable": true,
    "todo_state": "none",
    "acknowledged_content_version": 0
  },
  "after_correction": {
    "selected_ids": ["remote"],
    "current_answer_count": 1,
    "history_count": 2,
    "answer_event_count": 2,
    "supersedes_original": true,
    "original_snapshot_unchanged": true
  }
}
```

Every execution writes an isolated result bundle under ignored test output:

```text
summary.md
review-roundtrip/report.md
review-roundtrip/actual.json
review-roundtrip/screenshots/03-answer-saved.png
review-roundtrip/screenshots/05-after-restart.png
review-roundtrip/raw/                 # fixture-only protocol/server diagnostics
manifest.json                        # snapshot, tools, environment, timings, outcomes
```

The human report leads with outcome and changed expectations, then lists numbered
steps with plain-language actor/action, expected/observed result, and pass/fail/
inconclusive/not-run status. Show question labels and selected option labels along
with stable IDs. Embed screenshots at meaningful checkpoints; retain a text diff
for exact state, chronology, and negative assertions. Normal passing runs retain
small observations and selected screenshots; failures retain trace and bounded
diagnostics. Raw JSON/stdio alone is not the human review surface.

Example report row:

| Step | Action | Expected | Observed | Outcome |
| --- | --- | --- | --- | --- |
| R3 | Person chooses “Local” and saves instructions | One saved answer; Todo remains None | Local; “Keep the rollout small.”; one answer; Todo None | Pass |
| R5 | Restart and read through a fresh MCP session | Same Item and answer; change discoverable | Compare canonical observations and record exact differences | Pass / Fail |

Resolve random IDs to stable aliases by first occurrence, preserving equality and
cross-step relationships. Raw evidence retains original IDs/timestamps. Normalize
paths, ephemeral process IDs and presentation-only absolute times in the readable
diff. Do not normalize away event counts, ordering, state/content versions,
disposition, applicability, corrections, source identity or user state. Assert
timestamp validity/ordering separately where the claim requires them. Lists are
sorted only when their public contract makes order immaterial.

Visual baselines use pinned viewport, browser version, locale, timezone, fonts,
fixture state and platform. Define masking and comparison tolerance per checkpoint
before capture; masks cover only declared nondeterministic presentation and cannot
hide review content or controls. Uncalibrated screenshot comparisons are evidence
for human review, not claimed visual gates. Use geometric/accessibility assertions
for explicit width/focus requirements. Cross-platform screenshots are comparable
only with a supported baseline for that rendering environment.

## Owner feedback and golden updates

The owner can refer to `review-roundtrip / R3` and state a desired behavior or
annotation. Classify feedback before changing a baseline:

1. Product defect: fix the owning implementation; retain the expectation.
2. Harness/environment defect: fix capture or isolation; retain the expectation.
3. Incorrect or intentionally changed expectation: revise the owning product
   contract and golden version, then rerun affected claims against that revision.
4. Uncertain outcome: record inconclusive and obtain the missing observation.

Normal test runs never rewrite committed goldens. An explicit update command may
write a proposed bundle to ignored output, with expected/actual differences and
screenshots. Human acceptance of the concrete proposed expectation precedes
replacing the committed golden. Agents may repair product/harness code within
authorized scope; failing observations alone are not golden-update authority.
Independent invariants, such as one receipt/answer/event and preserved user state,
remain directly asserted even if a presentation golden changes.

Before adopting the harness, demonstrate a representative intentionally wrong
expected value is rejected, a missing test is rejected, and a stale binary forces
rebuild. Also demonstrate that a new runnable test without a feature/level/concern mapping
fails full-suite discovery. These controls are isolated harness tests, not mutations of production
or the committed accepted golden.

## Evidence ownership, failure and cleanup

Each attempt records case/claim/oracle versions, source revision and dirty digest,
binary/asset digests, selected/omitted cases, tools/renderer profile, environment,
invocation, start/end times, exit status and artifact paths. Core output is synthetic
and may be published as CI artifacts; never capture user account data or secrets.
Retain one current successful bundle locally and failed bundles until diagnosed;
CI artifacts use a documented bounded retention period.

A product mismatch is fail. Lost isolation, an unrelated occupied port, dependency
absence or an invalid observer is inconclusive/not run and nonzero for a required
case. Retry attempts stay visible. PR/main/release jobs propagate required-case
failure or omission; a successful report upload cannot overwrite the test exit code.

Finally blocks stop only recorded owned processes, close MCP/browser sessions,
and validate temporary paths before deleting their own data. Preserve failed
evidence. If cleanup fails, record the remaining PID/path, make the required run
nonzero, and never claim cleanup succeeded. Tests must not leave the portal port
occupied or make an ordinary developer session depend on their fixtures.

## Delivery and acceptance

1. Inventory/classify current assertions, add case/feature selection and a plan
   command, preserve current gates, and record warm baselines.
2. Implement `review-roundtrip` first with real MCP stdio, portal submission,
   restart/readback, and a proposed human report/golden bundle for owner review.
3. Add setup, inspection, and recovery journeys; split focused system/UI cases and reuse
   demo/process fixtures. Wire PR/main/release suites and artifact publishing.
4. Independently review selector completeness, failure propagation, binary
   freshness and golden controls; execute both positive and negative harness cases.

Acceptance requires: targeted commands execute only the selected/expanded claims;
required sets discover every declared case; migrated assertions have retained
executed owners; all four journey families pass their fixed oracles; timing
measurements meet budgets or expose a documented owned revision; readable reports
and proposed golden updates support case/step feedback; wrong expectations/missing
cases/stale binaries cannot produce false acceptance; required CI omissions fail;
cleanup is demonstrated. Passing checks remain bounded to their recorded target.

Implementation: `scripts/testing` owns selection, discovery, preparation and evidence;
existing Go tests remain beside their owners with explicit boundary classification.
Browser cases carry native tags, and the combined review case was split into layout,
validation and pending-submission checks without losing its assertions. Five physical
cases cover the four E2E families through real CLI, MCP and portal paths. Full main
and renderer release suites are wired into the existing workflows. See the linked
baseline for measured runtimes and host verification evidence. Visual tolerances
remain uncalibrated; golden acceptance remains an explicit owner step.
