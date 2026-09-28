# Agent-use efficiency reference baseline — 2026-09-28

These are local deterministic MCP replays from the
[agent-use evaluation](../../scripts/perf/README.md#agent-use-efficiency), not an
actual model run or a cross-machine latency guarantee. Token counts use pinned `js-tiktoken@1.0.21/o200k_base` as a
reference proxy. The MCP SDK returns both text `content` and
`structuredContent`; they are reported separately because an agent harness
may expose one or both. Neither column is provider-billed usage. Raw fixture
responses were counted in memory and were not saved in the report.

The fixture was the 10,000-Item Mature profile, with 25,000 content versions,
3,000 historical Runs, six Watchers, and six active Interests. The 1% and 20%
Attention variants had the same Item count and source setup. Each evaluation
started one Run, consumed every required continuation, submitted a result for
each selected Watcher, and finished. The synthetic source scan had no new
findings. Results below are single runs, so no percentile is implied.

| Case | MCP calls | Attention summaries | Changes read | Structured result tokens | Text result tokens |
| --- | ---: | ---: | ---: | ---: | ---: |
| Before, 1% Attention | 6 | 100 | 0 | 21,447 | 22,892 |
| After, 1% Attention | 6 | 100 | 0 | 20,238 | 21,520 |
| Before, 20% Attention | 44 | 2,000 | 0 | 307,400 | 327,399 |
| After removing repeated contexts, 20% Attention | 44 | 2,000 | 0 | 259,760 | 273,402 |
| Before, 20% Attention plus 250 synthetic user events | 49 | 2,000 | 251 | 322,479 | 343,204 |
| After, 20% Attention plus 250 synthetic user events | 49 | 2,000 | 251 | 274,957 | 289,325 |

The `start_run` response included 50 initial summaries. The 20% case needed 39
Attention continuation pages. Before the change, each repeated the stored
`AGENTS.md` and `USER.md`; their context objects represented 47,502 reference
tokens across those pages. Omitting them from continuations reduced structured
result tokens by 47,640 (15.5%) and text result tokens by 53,997 (16.5%) in
the same workload profile. The caller still read all 2,000 summaries and used the same
44 MCP calls. Tool definitions were another 2,523 reference tokens counted
once per agent context, outside the result-token columns.

The additional Fresh deterministic cases passed their state assertions:
`multi-interest` captured several active Interests for a broad Watcher;
`reconcile` read one existing Item, updated it from fixed source evidence, kept
a different Item at the same URL unchanged, and preserved the Todo and user
note; `partial-retry` returned the same receipt for the same request ID,
created one Watcher result, and did not advance its cursor. The fixed
one-matter source corpus itself was 225 reference tokens. These cases exercise
the protocol with scripted decisions; they do not establish that an agent
would choose those decisions unaided.

The second quick win is a write guardrail. New stored `AGENTS.md` edits are
limited to 8 KiB and `USER.md` edits to 16 KiB, measured as UTF-8 bytes on both
settings write paths. Preferences shows the server-provided limits and bytes
used. Existing longer text remains readable and is not truncated; an unrelated
timezone edit can retain it. This guardrail does not reduce the measured fixture
tokens because its contexts were already below the new limits.

The largest remaining cost is the required 2,000 compact Attention summaries:
after the context fix, their 39 continuation responses contributed 243,689
structured reference tokens. Reducing that cost would require a separate
decision about what information the agent must receive and how completeness is
preserved. Model decision quality and provider usage are outside this
deterministic CLI baseline.

## CLI stdout by response shape

The initial CLI measurement used separate matched Mature fixtures with 20%
Attention and 250 synthetic user events. Both formats read all 2,000 current
Attention Items and 250 events, submitted results for three due Watchers, and
completed the Run. Each format made 40 CLI calls. This was measured before the
CLI exposed captured packet continuations. Counts cover exact stdout, including
newlines, using the same pinned reference tokenizer. The default rendering
pretty-prints JSON and shortens display IDs; `--json` prints the API response
as received.

| CLI output category | Calls | `--json` tokens | Default tokens |
| --- | ---: | ---: | ---: |
| Attention Item list pages | 20 | 564,966 | 677,276 |
| Change pages | 3 | 14,025 | 21,045 |
| `brief` preview | 1 | 18,970 | 20,791 |
| `run start` first packet | 1 | 18,430 | 20,161 |
| Run list and detail | 2 | 18,503 | 20,771 |
| Status, settings, other lists/details, submissions, finish, version | 13 | 4,416 | 4,952 |
| **All measured stdout** | **40** | **639,310** | **764,996** |

Default output used 125,686 more reference tokens (19.7%) and 586,257 more
UTF-8 bytes (28.9%) across this command sample. Formatting cost was largest
for Attention list pages. The 20 `item list` pages contain current full Item
records rather than captured compact Run continuations. These are single local
runs, not provider usage or stable latency estimates.

### Quick win: captured continuations through the CLI

`aicp brief --cursor` now reaches the existing captured continuation API. The
rerun followed all 39 Attention cursors from `run start`, verified all 2,000
captured summaries and six active Interests against the Run snapshot, read the
remaining two change pages, and completed the Run. The two CLI renderings each
made 78 calls in the full measurement suite, which deliberately measured both
the compact packet and the alternative full Item list.

| Retrieval path | Calls | `--json` tokens | Default tokens |
| --- | ---: | ---: | ---: |
| Current full Attention Item list | 20 | 564,966 | 677,276 |
| `run start` plus captured Attention continuations | 40 | 262,267 | 264,102 |
| Remaining captured change pages | 2 | 11,217 | 16,829 |

Using the captured packet instead of the full Item list uses **53.6% fewer
reference tokens with `--json`** and **61.0% fewer with default output** in this
fixture. The packet path includes Watcher, Interest, and settings data, so this
is a conservative comparison for retrieving Attention summaries. It takes 40
calls versus 20 list calls; the observed local call times are not a stable
latency estimate. A CLI agent can now consume the complete captured packet
without substituting mutable full Item records for its compact summaries.

### Follow-up: larger pages and bounded Attention fields

Two successive runs used the same Mature 20% Attention profile. The first
raised continuation pages from 50 to at most 150 entries under the existing
64 KiB item-array target. The second removed `state_version` from Attention
entries, replaced the exact acknowledgement version with `unacknowledged`, and
added 256/512/256-byte title, summary, and Interest reason caps. The first
packet remains limited to 50 entries. Every run still consumed all 2,000
captured summaries and passed the deterministic state assertions.

| Stage, quiet MCP Run | Calls | Attention continuation pages | Structured result tokens | Text result tokens |
| --- | ---: | ---: | ---: | ---: |
| Prior 50-entry continuations | 44 | 39 | 259,599 | 273,241 |
| Up to 150 entries | 18 | 13 | 256,477 | 269,833 |
| Up to 150 entries and bounded fields | 18 | 13 | 243,578 | 256,834 |

Together, these changes removed 26 MCP calls and 16,021 structured reference
tokens (6.2%) in the matched workload profile. The corresponding CLI `--json`
`run start` plus Attention continuation output fell from 262,267 to 246,196
reference tokens. The fixture's prose was already shorter than the new caps,
so the measured token reduction comes from removed and simplified fields, plus
fewer page wrappers. A separate persisted-Item test verifies that long UTF-8
text is capped only in the packet, marked in `truncated_fields`, and remains
available in full Item detail. Single local timings are diagnostic; they do
not establish a stable latency distribution.

After adding the version 6 data migration, a fresh deterministic rerun passed
all six MCP and both CLI cases. Mature 20% quiet still used 18 MCP calls and
13 Attention continuation pages; it yielded 243,583 structured and 256,839
text reference tokens. The corresponding CLI `--json` Run start and Attention
continuations yielded 246,171 reference tokens. A populated version 5 upgrade
test verifies that captured Run, selected Watch, and retry receipt Attention
entries use the current projection, while the Run status and owner context stay
intact.

Before release, the version 6 migration and Run capture were corrected to
retain full text in snapshots and retry receipts. Caps now apply when responses
are rendered. The Run summary also omits `unacknowledged`; acknowledgement
actions remain available in the captured change range. The earlier figures
remain historical measurements of the previous projection.

The corrected Mature 20% quiet rerun consumed all 2,000 Attention summaries in
18 MCP calls and 13 continuation pages. It yielded 229,650 structured and
243,006 text reference tokens. The matched CLI backlog Run start and Attention
continuations used 232,238 `--json` or 221,642 default-output reference tokens.
All six MCP and two CLI deterministic cases passed. These single runs do not
establish a latency distribution or measure billed model usage.
