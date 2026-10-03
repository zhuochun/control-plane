# aicp heartbeat

Use the configured aicp MCP tools to complete one truthful heartbeat:

1. Call `start_run` and read its `{run, context}` packet: captured `AGENTS.md` and `USER.md`, every applicable Interest page, selected Watchers and their prior coverage/limitations, and the overview counts. Read every page in the Run's captured `(after_seq, through_seq]` change range before claiming complete change handling or acknowledging it. Interest cursors use `get_brief`; change cursors use `get_changes` with the captured bounds. Global Attention is optional under `context.available.attention`, not a Todo execution queue. A fresh `get_brief` is only a live overview and may drift between pages.
2. Inspect only the selected Watchers. Use each Watcher's bounded source instructions and cursor. Assess source input against only the Interest IDs and revisions captured for that Watcher; read each Interest's instructions from the Run's Interest context. A broad Watcher may include several Interests, while an explicit Watcher includes only its active links. Revisit linked open Items when useful.
3. When source evidence concerns an existing matter, use `list_items` with its exact dedupe key or bounded Watcher/text filters, then `get_item` or `get_context` when needed; retain identity and use the current content version. Shared source URLs do not require merging distinct matters. Read affected Items when captured user changes need further interpretation, even if those Items are source-free or no longer in Attention. If an optional summary has `truncated_fields`, read the full Item before relying on omitted text.
4. Call `submit_watch_findings` exactly once for every selected Watcher. Each source-derived Item needs at least one linked source reference and one or more captured Interest IDs with concise relevance reasons. A successful no-change scan submits an empty item list. Report incomplete or failed coverage honestly; never advance a cursor past unread content.
5. If useful agent context is not backed by a selected Watcher, `upsert_item` can retain it in the library but it will not enter Attention or count as Watcher coverage. Keep findings, source status, evidence, and next steps in concise Markdown. Use `propose_change` for justified Interest, Watcher, or grouped configuration changes; do not create topics merely to appear busy. Direct configuration tools are for an explicit user request and do not require this Run.
6. Call `finish_run` with a short source-coverage summary only after every selected Watcher has a terminal result. Acknowledge only the captured change range you fully consumed and durably reflected; otherwise finish without acknowledgement so it can replay. Do not claim global Attention was reviewed merely because the Run finished.

The aicp server records work; it does not browse sources or start another agent.

## Autonomous follow-through

Within the person's existing authority, advance actionable findings yourself or
delegate using your own tools. Read current applicable guidance and use
`list_items` with `delegation_status: "pending,blocked"` to recover unfinished
work, even when no Watcher is due. Follow-through needs no dummy source Run.
Do not treat Attention or a user's Todo as automatic authority to execute.
Choose and explain reasonable defaults for low-risk, reversible details within
scope. Do not manufacture a human question for every task; bring material
decisions with evidence and a recommendation.

Before external handoff, use `update_item_work` to record a pending delegation
on the existing Item. Retain its stable ID, intended executor, instructions,
input Item version and relevant source/instruction basis, and launch uncertainty.
After launch, save the returned external session/task reference and a concise
description of its system and continuation tools in delegation context. Give the executor
only the assigned Item ID, delegation ID, evidence and constraints it needs.

An executor can use `get_item` and `update_item_work` directly without starting
a Run, listing the global inbox, or reading unrelated configuration. Identify the
input version and requirements actually used in result context, preserve other
content/delegations, and leave delivery pending for primary-agent judgment.
Without aicp access, return through the original mechanism for the primary agent
to import. Use current content versions and request IDs. On `content_conflict`,
reread and merge with current content before retrying with its version and a new
request ID. The primary may have saved the session reference after your first
read: omit `external_ref` from executor patches to retain it. Only reuse a request
ID for an identical retry whose outcome is uncertain. Preserve earlier report
conclusions and continuation instructions when replacing Markdown fields.

Use the short [executor handoff example](delegation-handoff.md) rather than
repeating the global heartbeat protocol in each assignment.

Close follow-up only after supported results and repair references are persisted.
Rework uses the same delegation ID and external reference; record previous
executor/references in context when transferring. Omitted fields retain values,
empty delegation lists remove nothing, and user-owned state remains untouched.
Scan publication preserves delegated report/context. Use `update_item_work` to
reconcile new source evidence with research after publication. Ask the person
only for material decisions, missing authority, or unresolved blockers, with
evidence and a recommendation.
