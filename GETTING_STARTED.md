# Getting started with aicp

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

## Set up together

1. Run `aicp init`, then `aicp serve` in a persistent terminal. In another
   terminal, run `aicp doctor`; `aicp open` opens the local portal. Initialization
   creates useful stored `AGENTS.md` and a starting `USER.md`. They are editable
   settings, not files you need to place beside the executable.
2. Tell your agent your current priorities, relevant background, and preferred
   follow-through. Review its `USER.md` draft. After you approve it, the agent
   can save it with `aicp config user-context set --file USER.md`, the MCP
   `set_user_context` tool, or Preferences. Do not include source credentials.
   The stored `AGENTS.md` is a sound default you can customize in Preferences;
   aicp's command and Run rules remain authoritative.
3. With your agent, add an active Interest for a current concern and an active
   Watcher for a concrete source. Use `aicp interest --help` and
   `aicp watch --help` for the accepted fields, or configure them in Monitoring.
   The Watcher must apply to that Interest. You may defer a source and still
   create personal Items; setup will keep showing what is missing.
4. Run `aicp doctor --json` to see `health.setup`: owner context, active
   Interest, applicable active Watcher, and derived `done`. No onboarding
   progress is stored. `aicp brief --json` previews context, due work, and
   health without claiming a Run. A completed setup still needs a real
   inspection before coverage is verified.

To connect an MCP agent, register the executable's absolute path as a stdio
server with `aicp mcp --server http://127.0.0.1:7331`. The server must be
running first. The agent can use `get_brief` to read the plane and direct
configuration tools to save approved changes without starting a Run.

## First inspection and later visits

When the agent has source access, let it start one Run, inspect selected
Watchers with its external tools, submit one terminal coverage result for each,
and finish. The MCP tools are `start_run`, `submit_watch_findings`, and
`finish_run`; CLI equivalents are under `aicp run --help`.
`examples/heartbeat-prompt.md` gives the detailed Run contract. A configured
cadence does not mean a scheduler is running. Add external scheduling only
after checking one real inspection and deciding how often you want it.

If the agent uses the CLI, pass `--json` for compact output.
`aicp run start --json` returns the first captured packet page. Follow every cursor under
`brief.continuations` with `aicp brief --cursor <cursor> --json`. Follow
`brief.changes_next_cursor` with the captured sequence bounds:

```text
aicp changes --after-seq <after> --through-seq <through> --cursor <cursor> --json
```

The CLI's `item list` returns current full Items; it does not replace the
captured compact Attention pages.

On return, open Attention for findings and actions, Monitoring for due reasons
and coverage, Activity for Run history or interrupted Run recovery, and
Preferences for context. The next agent visit can read a fresh brief and
continue from durable state. A failed or partial Run does not establish
successful coverage; an interrupted Run can be explicitly abandoned in
Activity without discarding already committed findings.
