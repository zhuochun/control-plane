# aicp heartbeat

Use the configured aicp MCP tools to complete one truthful heartbeat:

1. Call `start_run`, read `AGENTS.md` and `USER.md`, every active Interest, every unarchived Attention summary, and every page in its captured `(after_seq, through_seq]` change range. Follow all continuation cursors before relying on older assumptions or finishing.
2. Inspect only the selected Watchers. Use each Watcher's bounded source instructions and cursor. Assess source input against only the Interest IDs and revisions captured for that Watcher; read each Interest's instructions from the Run's Interest context. A broad Watcher may include several Interests, while an explicit Watcher includes only its active links. Revisit linked open Items when useful.
3. Before updating an existing finding, call `get_item` or `get_context`; retain its stable dedupe key and use the current content version.
4. Call `submit_watch_findings` exactly once for every selected Watcher. Each source-derived Item needs at least one linked source reference and one or more captured Interest IDs with concise relevance reasons. A successful no-change scan submits an empty item list. Report incomplete or failed coverage honestly; never advance a cursor past unread content.
5. If useful agent context is not backed by a selected Watcher, `upsert_item` can retain it in the library but it will not enter Attention or count as Watcher coverage. Keep findings, source status, evidence, and next steps in concise Markdown. Use `propose_change` for justified Interest, Watcher, or grouped configuration changes; do not create topics merely to appear busy. Direct configuration tools are for an explicit user request and do not require this Run.
6. Call `finish_run` with a short summary only after every selected Watcher has a terminal result. Acknowledge only the captured change range you fully consumed and durably reflected.

The aicp server records work; it does not browse sources or start another agent.
