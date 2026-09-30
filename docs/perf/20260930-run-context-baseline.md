# Run context evaluation: 2026-09-30

The [Run context contract](../specs/20260930-run-context-and-brief-spec.md)
removes global Attention from mandatory source context. This baseline measures
the implemented contract, not the former obligation to enumerate every summary.

## Executed evidence

- `scripts/verify.ps1`: passed locked install, web typecheck/build, Go vet/tests,
  executable build, all 16 Playwright tests, and the isolated two-cycle demo.
- `scripts/perf/run-agent-eval.ps1 -OutputRoot ./dist/run-context-eval-20260930`:
  all six MCP and two CLI cases passed their deterministic state/protocol checks.
- Independent code review and focused re-review: no unresolved actionable findings.
- Application regressions cover 10,000 unchanged Todos, fixed optional snapshots,
  scoped Interest continuations, source-generation failures including paused
  Watchers, live brief paging, full captured events, and v6 receipt migration/replay.

The first Windows gate exposed two browser tests consuming superseded brief
fields. Their consumers were updated and the complete gate rerun successfully.

## MCP payload accounting

The tokenizer is the existing pinned `js-tiktoken@1.0.21`, `o200k_base` reference
proxy. SDK structured and text responses are counted separately; which reaches
the model depends on the harness. These are not billed model tokens. Tool
definitions cost 2,800 reference tokens once and are excluded from both response
columns. The reconciliation corpus costs another 234, counted separately.

| Case | MCP calls | Required Interests | Mandatory Attention summaries | Captured events read | Structured response tokens | Text response tokens |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Mature, 1% Attention, quiet | 5 | 3 | 0 | 0 | 2,811 | 3,081 |
| Mature, 20% Attention, quiet | 5 | 3 | 0 | 0 | 2,849 | 3,119 |
| Mature, 20% Attention, backlog | 10 | 3 | 0 | 251 | 18,182 | 19,179 |
| Fresh, reconciliation | 6 | 2 | 0 | 0 | 3,008 | 3,304 |
| Fresh, broad Interest matching | 4 | 3 | 0 | 0 | 2,530 | 2,779 |
| Fresh, partial coverage/retry | 5 | 2 | 0 | 0 | 2,465 | 2,720 |

The Mature fixtures contain 10,000 Items and 3,000 historical Runs. At 20%
Attention, the former corrected baseline required 2,000 summaries, 18 calls,
and 229,650 structured response reference tokens. The new quiet case requires
5 calls and 2,849: approximately 98.8% fewer structured response tokens.
This comparison changes the required-reading policy intentionally; it does not
prove the agent reviewed those 2,000 Items more efficiently. They remain in the
Run snapshot and user Attention, and can be requested explicitly.

The 1% versus 20% quiet cases use the same number of required reads. Small token
differences include counts and randomly generated record IDs. Growing a genuine
user-change backlog still costs more; those events must not be skipped merely
to improve a token result.

Reconciliation discovers the existing Item through MCP exact-key lookup before
reading current detail. The answer key confirms identity, user Todo/note state,
and the distinct matter sharing its source URL remain correct. Partial coverage
preserves its source cursor, and retry creates one terminal result.

## CLI accounting

Both CLI backlog cases pass. Each trace contains 39 calls, including an explicit
whole-Attention review path with 20 current full-Item pages. Those pages are
optional user review, not source Run requirements.

| Rendering | Required source flow stdout tokens | Entire mixed trace stdout tokens |
| --- | ---: | ---: |
| `--json` | 16,832 | 593,388 |
| Default pretty JSON | 24,067 | 714,951 |

Required source flow includes Run start, both captured change continuations,
three submissions, and finish. The mixed trace also includes the explicit full
Attention listing, settings, status, configuration lists/details, history,
brief, and Run detail. Its total must not be presented as heartbeat context cost.

## Limits

This is one local deterministic evaluation, not a timing benchmark, provider
usage report, or real-agent decision-quality evaluation. Source material is
synthetic and source access remains external. Full Attention capture at Run
start is deliberately retained; no storage or query-speed improvement is claimed.
The existing frontend chunk-size warning remains non-failing and unrelated to
the context contract.
