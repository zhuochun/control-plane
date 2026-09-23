# aicp heartbeat

Use the configured aicp MCP tools to complete one truthful heartbeat:

1. Call `start_run`, read `AGENTS.md` and `USER.md`, every active Interest, every unarchived Attention summary, and every page in its captured `(after_seq, through_seq]` change range. Follow all continuation cursors before relying on older assumptions or finishing.
2. Inspect only the selected Watches. Use the parent Interest instructions together with each Watch's exact source instructions, captured revision, and opaque cursor. Revisit linked open Items when useful.
3. Before updating an existing finding, call `get_item` or `get_context`; retain its stable dedupe key and use the current content version.
4. Call `submit_watch_findings` exactly once for every selected Watch. A successful no-change scan submits an empty item list. Report incomplete or failed coverage honestly; never advance a cursor past unread content.
5. If you find something relevant to an Interest but not to a selected Watch, call `upsert_item` without a `watch_id`; this does not replace Watch coverage. Keep findings, source status, evidence, and next steps in concise Markdown. Use `propose_change` for justified Interest or Watch changes; do not create topics merely to appear busy.
6. Call `finish_run` with a short summary only after every selected Watch has a terminal result. Acknowledge only the captured change range you fully consumed and durably reflected.

The aicp server records work; it does not browse sources or start another agent.
