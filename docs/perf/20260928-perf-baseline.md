# Aged-data performance baseline — 2026-09-28

This is a local Windows snapshot from the [performance suite](../../scripts/perf/README.md),
not a cross-machine latency guarantee. Browser timings measure navigation or
interaction until the target content is visible in headless Chromium. Each
Mature comparison used the same deterministic fixture version 3 with 10,000
Items, 25,000 versions, and 3,000 Runs, with 3 repetitions before and 5 after
on one server process.
These small samples support finding large bottlenecks, not a reliable p95.
The base revision was `615d1f0246b92f24d0ed5a00d993eaa4d7eac9ac`; the
"after" binary included the working-tree changes described below. The host was
Windows 11 Education (build 26200), Intel Core i7-8700 with 12 logical CPUs
and about 32 GB RAM, Go 1.27.1, and Node 24.18.0. Disk type and power mode
were not recorded.

| Browser journey | Before median | After median | Change |
| --- | ---: | ---: | ---: |
| Attention | 258 ms | 244 ms | Similar |
| Library first usable rows | 18,396 ms | 129 ms | 143× faster |
| Library search for an old Item | 4,475 ms | 101 ms | 44× faster |
| Item inspector | 262 ms | 39 ms | Faster in this sample |
| Monitoring | 160 ms | 90 ms | Faster in this sample |
| Activity with 3,000 historical Runs | 892 ms | 68 ms | 13× faster |

The main cause was visible in the network trace and code: the portal fetched
every Item in 100-page batches, while each HTTP page loaded and decoded the
whole Item collection. Library also rendered all 10,000 rows. The change pages
Items and Runs in SQLite before decoding, makes Library/Attention/Todos load
100 rows at a time with an explicit **Load more items** control, and loads only
the 20 Runs displayed in Activity. Search and filters query the full Item set.
The browser suite also found that a direct `/todos` load returned 404; the
portal fallback now serves that route, and reload is covered by an end-to-end
test.

After the change, the 100,000-Item / 300,000-version / 30,000-Run profile had
browser medians of 253 ms for Library, 822 ms for a whole-library search, and
151 ms for Activity (3 repetitions). The revision-heavy 20,000-Item profile
had 166 ms Library and 219 ms search medians (3 repetitions). Large-profile
search still scales with all Items and is the next measured candidate if it
becomes a real user delay.

The HTTP heartbeat exercise on a separately seeded Mature fixture used three
active Watchers, five new findings each, an updated second cycle, and an
unchanged third cycle. The final build completed `start_run` in 49–59 ms and
each five-Item submission in 15–31 ms in that one run. The previous build's
matched sample had 55–59 ms starts and 14–35 ms submissions; there is no
supported claim of a heartbeat speedup. The 100,000-Item profile's three
starts took 367–374 ms, and five-Item submissions took 110–121 ms. The script
verified content versions, no-change publication, and preservation of a user
Todo and note.

A later bounded recovery and mixed-use run on the final build kept two readers
active while the second heartbeat published findings. On Mature, the slowest of
20 concurrent reads was 114 ms; on Large it was 910 ms. These are single-run
tails, not stable percentiles. The server restarted with an unfinished Run in
115–117 ms to health readiness on both profiles, retained the active Run, and
accepted an explicit owner abandonment. The Large mixed-read tail and 822 ms
whole-library search remain candidates for future profiling if they affect
actual use.

All measurements used isolated fixture data directories on this host. Source
inspection and a running external agent were outside scope. The browser runner
measured fresh browser contexts against one server process per report; repeat
on more processes and hardware before adopting hard latency budgets. Item
archive behavior is absent from the current product, so retired Watchers are
represented by paused and deprecated states.
