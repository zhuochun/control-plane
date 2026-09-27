# aicp project principles

Read [docs/glossary.md](docs/glossary.md) for domain language and the
[Watcher–Interest specification](docs/specs/20260924-watcher-interest-model-spec.md)
for the current Watcher–Interest behavior. Check code, migrations, and public
adapters before changing behavior; older MVP documents describe the prior model.

- Keep aicp a small local control plane. External agents inspect sources with
  their existing tools; aicp owns durable configuration, findings, user state,
  checkpoints, and history. It does not own source credentials or scheduling.
- Model Watchers as input streams and Interests as reasons to recognize and
  retain information. Their relationship can change without replacing the
  source stream. New and revised Interests apply from the next Run snapshot;
  do not claim earlier source content was assessed under them.
- Reuse `Item.sources[]` for concrete evidence. Do not add an Anchor,
  Observation, or source-to-Items index merely because several Items cite the
  same URL. Add structured fields only for a concrete operation or invariant;
  keep descriptive meaning in text.
- Let a running inspection finish against its captured configuration while
  configuration edits commit independently for later Runs. Attribute old
  results to that snapshot. Never apply an old source cursor to a newly
  configured source scope or let an old result undo newer scheduling changes.
- User-created Items may have no source or only user-supplied date context.
  Agent-generated source findings remain source-backed. Source content updates
  must preserve user-owned Todo, reminder, acknowledgement, and note state.
- Configuration reading and editing do not start a heartbeat or claim due
  Watchers. Keep CLI, MCP, portal, and application rules aligned; use readable
  slugs for human configuration and immutable IDs for durable references.
- Convert persisted data with versioned migration scripts before the new
  runtime serves requests. Runtime code handles one canonical data shape;
  preserve history and user decisions, and stop on migration failure.
- Keep revisions, request receipts, truthful terminal Watcher results, and
  atomic Item/checkpoint publication. Report what was actually inspected.

Preserve unrelated working-tree changes. Treat discussion and specifications
as design work; implementation needs an explicit request. Verify behavior at
the public interface when implementation is requested.
