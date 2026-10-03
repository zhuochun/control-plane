# Luna delegation smoke test — 2026-10-03

Result: passed with a real Codex `gpt-6-luna` thread, including same-thread
rework and recovery from an optimistic concurrency conflict.

## Scope and evidence

An isolated aicp server used disposable test data. A synthetic, user-created
Item described a Copy button: copy the template verbatim, confirm success,
and show a retryable error on failure. Luna assessed feasibility and later
added two observable acceptance checks. No clipboard implementation, source
scan, browsing, or external publishing was performed.

- Item: `a2ba6ca0-489a-46b2-99b0-54a02a17c241`
- Delegation: `feasibility`; executor: `codex:gpt-6-luna`
- Thread: `01a0fd9d-ceed-75e3-9f7c-02a6a6078011`
- Thread title: `aicp Luna delegation smoke test`
- Raw snapshots: `%LOCALAPPDATA%\Temp\aicp-luna-smoke-f104b1f2e5bb4fd3ae1fcd9379af5286`

The primary agent launched the thread outside aicp and stored its ID in the
Item delegation's `external_ref`. Luna received only the assigned Item ID,
delegation ID, task, and Item read/update protocol. It read and updated that
Item directly through HTTP.

## Observed path

| Step | Evidence |
| --- | --- |
| Create Item and delegation | Item v1; pending delegation v2 |
| Persist launched thread identity | Primary writes `external_ref`, producing v3 |
| Return feasibility assessment | Luna's write based on v2 receives HTTP 409; it re-reads v3, preserves the thread reference, and writes v4 |
| Recover and accept | Primary finds the Item through CLI delegation filters and `matching_delegation_ids`, checks the result, and closes it at v5 |
| Request rework | Primary reopens the same delegation at v6 and messages the thread identified by the stored `external_ref` |
| Return acceptance checks | Same Luna thread reads v6, appends the checks, and writes v7 |
| Final review | Primary checks the retained report and closes the delegation at v8; pending/blocked query returns zero Items |

The first Luna turn took about 52 seconds and five HTTP shell calls, including
the conflict recovery. Rework took about 29 seconds and three calls. These are
single-run observed durations, not a performance baseline.

The original report remained intact after rework. Todo stayed `todo`, the
human note was unchanged, and `state_version` stayed 2. Closing delegated
work therefore did not complete the user's Todo. The same delegation and
thread identity were retained throughout.

## Experience and recommended improvements

1. **Make autonomous judgment explicit in task wording.** The launch prompt
   requested a human decision detail and Luna escalated toast versus button
   text and duration. These reversible presentation details could normally
   receive a proposed default with a rationale. Guidance should request human
   decisions only where material scope, tradeoffs, or authority require them;
   avoid implicitly requiring every task to manufacture a question.
2. **Reduce repeated handoff instructions.** This tiny task required a fairly
   long prompt describing HTTP bodies, preservation rules, and conflict
   recovery. A reusable handoff example and the existing `get_item` /
   `update_item_work` tools can reduce this boilerplate where those tools are
   available. Agent launch remains the primary agent's responsibility.
3. **Teach the identity-write race before changing concurrency.** The worker
   read the Item before the primary saved its thread ID. Existing revision
   checks correctly prevented overwriting it, but added a failed write and
   another read. Handoff guidance should demonstrate re-reading, merging, and
   retrying on 409. Separate field revisions are not justified by one smoke
   run; measure recurring contention before adding state or API complexity.
4. **Keep continuation context usable and reports stable.** A thread ID works
   when the primary also knows the system and continuation tools. Record those
   concise instructions in delegation context. Ask workers to preserve prior
   conclusions and append bounded rework results: this smoke did so for the
   report, while replacing delegation context. Whole Markdown replacements
   still require careful merge behavior from the worker.

These improvements fit existing Item fields, agent guidance, and tooling;
the smoke did not establish a need for additional workflow entities.

## Follow-up optimization

The handoff recommendations are now reflected in the default application
AGENTS guidance (migration 9), the heartbeat prompt, the `update_item_work` tool
description, and a reusable [executor assignment](../../examples/delegation-handoff.md).
The migration upgrades untouched guidance and retains owner edits. Tests cover
upgrades from versions 7 and 8 and repeated migration without duplicate guidance.
The full `scripts/verify.ps1` gate passed after these changes: Go lint/vet/tests,
web typecheck/build, 16 browser tests, and the two-cycle demo. The example JSON
and its document links were checked separately.

## Live rerun after optimization

A new `gpt-6-luna` projectless thread used the revised handoff wording with an
equivalent synthetic Copy spec. HTTP access was supplied in the assignment;
this did not exercise an installed MCP connection or automatic consumption of
the application AGENTS defaults.

- Item: `90cf1013-b10e-4ab7-9904-60b7d8a50291`
- Thread: `01a0ff9a-7ae1-7532-8135-a48e6a0a27d7`
- Thread title: `aicp Luna optimized handoff smoke`
- Raw snapshots: `%LOCALAPPDATA%\Temp\aicp-luna-v2-cf5924307a1e4274940bef566328217e`
- Assignment size: 1,124 characters, including HTTP protocol instructions.

| Turn | Duration | HTTP calls | Result |
| --- | --- | --- | --- |
| Assessment | 48.479 s | 4, including one rejected write | Chooses nearby, non-blocking success feedback with a reason; no human presentation decision requested |
| Acceptance-check rework | 26.399 s | 2 | Same thread adds two checks and retains prior report and continuation context |
| Focused correction | 22.710 s | 2 | Primary notices missing visible retryable-error assertion; same thread corrects check 2 |

The assessment again encountered the real launch-reference race: it read v2,
received a conflict after the primary saved the reference at v3, re-read, merged,
and wrote v4 with a new request ID. Both the reference and continuation
instructions survived. Filtered Item lookup recovered the original thread for
rework. The primary accepted, closed, and reopened the same delegation at
v5/v6; subsequent work produced v7/v8, followed by final closure at v9.
Pending/blocked lookup returned zero Items. The original report, user note,
Todo, origin, and user state version remained intact. The isolated server was
stopped after final verification.

This run supports the intended autonomous default choice and continuation-context
preservation. It also demonstrates why primary-agent review remains necessary:
the first pair of acceptance checks omitted the observable failure message.
The primary detected that omission and requested repair without asking the
person. Quality correction increased total worker turn time to about 98 seconds,
versus about 81 seconds in the earlier two-turn smoke. Individual turns were
slightly shorter, but these single runs do not establish a speed improvement.
HTTP handoff still requires protocol boilerplate; MCP convenience remains
unmeasured.

## Evidence limits

This demonstrates a synthetic Item handoff, direct worker update, filtered
recovery, primary judgment, and same-thread repair with a reachable local
server. It does not demonstrate a full Watcher scan pipeline, recovery after
a server restart or unavailable thread, restricted worker credentials, or
real product feasibility research. The prompt restricted worker scope;
this was not a test of server-enforced access isolation.
