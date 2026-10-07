# Getting started with aicp

Give your agent this prompt:

> Install and set up aicp from https://github.com/zhuochun/control-plane.
> Follow its GETTING_STARTED.md, connect yourself through CLI or MCP, and open
> the review portal when ready.

Then tell it your interests and sources, for example:

> Track launch readiness and customer onboarding. Look after Slack
> #launch-readiness and pilot-customer email through your existing tools.
> Investigate and delegate useful follow-ups within my authority. Bring me
> material decisions with evidence and a recommendation.

The remaining instructions are the **agent's setup and operating contract**.
The agent configures the workspace and runs inspections; the person reviews
its output and supplies decisions, answers, or new direction.

Ask your agent to download the archive for your operating system from the
[aicp releases](https://github.com/zhuochun/control-plane/releases), verify it
against `checksums.txt`, and extract it. Read `aicp --help` and this file
together. Keep the executable and this guide from the release archive. aicp is
a local control plane: it stores what you
care about, where to look, what an inspection found, and what you chose to do.
Your agent uses its own tools to access sources. aicp does not hold source
credentials, launch the agent, or schedule it.

An **Interest** explains why a finding matters. A **Watcher** names one bounded
source and when it is due for inspection. A **Run** records an actual inspection
and its coverage. An **Item** retains a matter to review or act on. A Watcher
can assess all active Interests or only those explicitly linked to it.

## Agent setup

1. Run `aicp init`, then `aicp serve` in a persistent terminal. In another
   terminal, run `aicp doctor`; `aicp open` opens the local portal. Initialization
   creates useful stored `AGENTS.md` and a starting `USER.md`. They are editable
   settings, not files you need to place beside the executable.
2. Capture the person's current priorities, relevant background, and preferred
   follow-through as owner context. Save their stated context with
   `aicp config user-context set --file USER.md`, the MCP
   `set_user_context` tool, or Preferences. Do not include source credentials.
   The stored `AGENTS.md` is a sound default you can customize in Preferences;
   aicp's command and Run rules remain authoritative.
3. Translate the person's request into an active Interest for a current concern
   and an active Watcher for a bounded source. Use `aicp interest --help` and
   `aicp watch --help` for the accepted fields. Reuse the agent's available
   source tools and confirm any missing source scope or cadence.
   The Watcher must apply to that Interest. An unavailable source can be deferred;
   coverage must describe the limitation rather than imply it was inspected.
4. Run `aicp doctor --json` to see `health.setup`: owner context, active
   Interest, applicable active Watcher, and derived `done`. No onboarding
   progress is stored. `aicp brief --json` previews focus, due work, failures,
   Attention samples, and pending-change counts without claiming a Run. A completed setup still needs a real
   inspection before coverage is verified.

To connect an MCP agent, register the executable's absolute path as a stdio
server with `aicp mcp --server http://127.0.0.1:7331`. The server must be
running first. The agent can use `get_brief` to read the plane and direct
configuration tools to save approved changes without starting a Run.

## Inspection, delegation, and return visits

With source access, start one Run, inspect selected Watchers with the agent's
external tools, submit one terminal coverage result for each, and finish.
The MCP tools are `start_run`, `submit_watch_findings`, and
`finish_run`; CLI equivalents are under `aicp run --help`.
`examples/heartbeat-prompt.md` gives the detailed Run contract. A configured
cadence does not mean a scheduler is running. Add external scheduling only
after checking one real inspection and deciding how often you want it.

Advance actionable work within the person's authority. When delegation helps,
record a pending handoff on the existing Item before launching an executor
through the agent's own tools. Save its continuation reference, retrieve and
judge the result, repair it if needed, and close the handoff after persisting the
supported outcome. Executors need only their assigned Item and bounded task;
they do not need a source Run or the global inbox. Follow the
[delegation handoff](examples/delegation-handoff.md) for versioning and recovery.

Publish work for human review using the discovered report capabilities:
Markdown and tables, metric outcomes, supported diagrams, and structured
questions where a decision is needed. The person's saved answers and notes
are context for later agent work. A submitted answer does not itself launch an
agent or establish that an external action happened.

If the agent uses the CLI, pass `--json` for compact output.
`aicp run start --json` returns `{run, context}`. Read captured context and all
applicable Interests. Follow `context.continuations.interests` with
`aicp brief --cursor <cursor> --json`. Follow `context.continuations.changes`
using `run.after_seq` and `run.through_seq`:

```text
aicp changes --after-seq <after> --through-seq <through> --cursor <cursor> --json
```

Global Attention is optional via `context.available.attention.cursor` and the
same `brief --cursor` command. These pages say `consistency: captured`; a fresh
brief says `consistency: live` and can change between calls. Unchanged Todos and
due reminders remain user work, not automatic heartbeat obligations.
Use `item list --dedupe-key <key>` or bounded `--watch`/`--query` filters to find
an existing matter before updating it. Current Item reads retain the existing
version/conflict rules. Complete captured change handling is required before
acknowledgement; source coverage still requires a terminal result per Watcher.

On return, open Attention for findings and actions, Monitoring for due reasons
and coverage, Activity for Run history or interrupted Run recovery, and
Preferences for context. The next agent visit can read a fresh brief and
continue from durable state. A failed or partial Run does not establish
successful coverage; an interrupted Run can be explicitly abandoned in
Activity without discarding already committed findings.
