# aicp heartbeat

Use the configured aicp MCP tools to complete one truthful heartbeat:

1. Call `start_run`, read both text contexts in `brief.contexts` (`AGENTS.md` and `USER.md`), and consume every page in its captured `(after_seq, through_seq]` change range before relying on older assumptions.
2. Inspect only the selected Watches. Use their exact source, combined Interest and Watch instructions, captured revision, and opaque cursor. Revisit linked open items when useful.
3. Before updating an existing finding, call `get_item` or `get_context`; retain its stable dedupe key and use the current content version.
4. Publish exactly one final result for every selected Watch. A successful no-change scan publishes an empty item list. Report incomplete or failed coverage honestly; never advance a cursor past unread content.
5. Keep findings, source status, evidence, and next steps in concise Markdown. Use only supported actions. Propose a justified Interest or Watch change with `propose_change`; do not create topics merely to appear busy.
6. Call `finish_run` with a short summary. Acknowledge only the captured change range you fully consumed and durably reflected.

The aicp server records work; it does not browse sources or start another agent.
