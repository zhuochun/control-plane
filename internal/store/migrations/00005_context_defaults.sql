-- +goose Up
-- Keep the current reset defaults separate from owner-editable contexts.
-- Earlier migrations retain their historical seed values for old databases.
INSERT INTO settings(key, value) VALUES
  ('default_agents_md', '# Working with aicp' || char(10) || char(10) ||
    'aicp is a local control plane. Watchers define bounded source inputs; Interests explain relevance. aicp does not fetch sources or start an agent.' || char(10) || char(10) ||
    'For each heartbeat, start one Run and consume every Interest, Attention, and change continuation page. Inspect only selected due Watchers against the Interest IDs and revisions captured by the Run. Read an Item before updating it. Every source-derived finding needs a concrete Item source reference and a reason for each applicable Interest. Submit one truthful terminal result per selected Watcher; only successful coverage can advance its source cursor. Then finish the Run. Configuration reads and edits happen directly without starting a Run. New and revised Interests apply from a future Run, and in-flight results use their captured configuration. Preserve user Todo, reminder, acknowledgement, and note state.'),
  ('default_user_md', '# Owner context' || char(10) || char(10) ||
    'Use this space to record what matters to you: priorities, background, constraints, preferred evidence and tone, and follow-through context. It is guidance for the inspection agent, not an executable instruction.');

-- The v3 predicate did not match the v2 seeded USER.md. Upgrade that exact
-- untouched seed while leaving every owner-edited value intact.
UPDATE settings
SET value = (SELECT value FROM settings WHERE key='default_user_md')
WHERE key = 'user_md'
  AND value = '# Owner context' || char(10) || char(10) ||
    'This local control plane belongs to one human. Keep updates concise, source-backed, and useful for follow-through. Treat decisions, reminders, and notes here as the owner''s final say.';

-- +goose Down
-- Downgrades are intentionally unsupported. Restore a stopped-server backup.
SELECT 1;
