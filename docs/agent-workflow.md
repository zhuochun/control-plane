# Agent workflow reference

## Domain model

The durable configuration and work relationships are:

```text
Watcher = a bounded source input, cadence, and source checkpoint
Interest = a relevance filter and interpretation rule
Run = one inspection attempt across up to 20 due Watchers
Item = one continuing matter, with Item-local source references,
       an originating Watcher when source-derived, and Interest reasons
```

An **Interest** is the durable purpose: a title, free-form instructions, and
an `active`, `paused`, or `deprecated` lifecycle state. A **Watcher** owns a
bounded source, inspection cadence, optional validity end, and checkpoint.
Broad Watchers assess all active Interests; explicit Watchers assess only their
linked Interests. Changing links preserves Watcher identity and checkpoint.

An **Item** is retained work or knowledge. Its kind is
`note`, `report`, `task`, or `outcome`; all kinds can contain Markdown content,
source references, context, Todo state, reminders, acknowledgement state, and
user notes. A user-created Item can have no source or a source date without a
link. Agent-published source findings cite a Watcher, source reference, and
one or more Interest reasons. A **Proposal** is an agent-suggested configuration
change for review; it does not change configuration automatically.

An Item can also retain small **delegation records**: who received work, its
external continuation reference, instructions, and pending/blocked/closed
follow-up. The primary agent chooses and launches executors through its own
tools. Executors can read and update their assigned Item using
`update_item_work`, without starting a source Run or reading the global inbox.
Work updates preserve identity, provenance, Interest relevance, and user state.
See the [Item-local delegation contract](specs/20261002-agent-delegation-and-receipts-spec.md).
For a short executor assignment and work-update example, use the
[delegation handoff guide](../examples/delegation-handoff.md).

```powershell
.\dist\aicp.exe item list --delegation-status pending,blocked --json
.\dist\aicp.exe item work <item-id> --file work-update.json
```

Work payloads require `expected_content_version`; supplied report/context fields
replace those fields after reconciliation. Delegation patches merge by ID, omitted
fields remain, and empty lists remove nothing. Source scans retain delegated
report/context; use work updates to reconcile new evidence with research. Global
autonomous-follow-through guidance is in the application-configured `AGENTS.md`.

## Agent connection

Start the HTTP server first. Register the same executable as a local MCP stdio
adapter, using an absolute path in a real installation:

```powershell
codex mcp add aicp -- C:\absolute\path\aicp.exe mcp --server http://127.0.0.1:7331
```

`examples/heartbeat-prompt.md` is the reference run contract.
`examples/heartbeat.ps1` and `examples/heartbeat.sh` show how an external
scheduler can invoke a configured Codex harness with overlap protection. See
`examples/scheduling.md`. aicp records when Watches are due; it does not launch
an agent itself.

### How an agent run works

aicp is the local control plane, not the source connector or scheduler. An
external heartbeat starts the agent, and the agent uses its existing tools to
inspect sources:

1. Call `start_run`. aicp returns `{run, context}` with captured owner/agent
   context, applicable Interest instructions, selected Watcher snapshots and
   prior limitations, the captured change range, and overview counts. Global
   Attention is optional, not a mandatory backlog review.
2. Read all applicable Interest pages and captured changes, then inspect only the selected Watchers
   with the agent's Slack, browser, GitHub, Drive, or other tools. aicp never
   receives source credentials or fetches those systems. Find existing matters
   through bounded `list_items` / `item list` lookup before updating them.
3. Call `submit_watch_findings` once per selected Watcher. The result records
   coverage, limitations, the next cursor, and zero or more Item upserts. Each
   source-backed Item names the captured Interest reasons and concrete source references.
   `upsert_item` can retain an Interest-level agent note without Watcher
   coverage; such a note does not enter Attention. Stable dedupe keys and
   expected content versions let the agent update findings instead of
   creating duplicates.
4. Call `finish_run` only after every selected Watcher has a terminal result.
   Successful coverage advances the Watcher checkpoint and next due time; failed
   or partial coverage remains eligible for a later run.
   If the external agent cannot finish, review the active run in Activity and
   explicitly abandon it with a reason. Submitted findings and successful
   checkpoints remain; unreported Watchers stay due and captured changes replay.
5. Review the resulting Attention items. You can open sources, acknowledge
   findings, set Todo or Done, add reminders, and accept or reject proposals.
   The next run receives those local changes as context.

The server does not promise that an inspection is running merely because a
Watcher exists. Until an external runner connects, the portal reports that no
agent run has been received.
Monitoring shows each Watcher's latest inspection and due reason; Activity
shows the active run and the number of due Watchers. Due work beyond the
20-Watcher run limit remains visible for a later heartbeat.

