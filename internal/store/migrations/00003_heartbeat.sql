-- +goose Up
ALTER TABLE runs ADD COLUMN context_snapshot TEXT NOT NULL DEFAULT '{}';

-- Upgrade only the original seeded defaults. Any owner-edited context remains
-- untouched; this makes the migration safe for existing local installations.
UPDATE settings
SET value = '# Working with aicp' || char(10) || char(10) ||
  'aicp is your local control plane for Interests, Watches, Items, user changes, and source checkpoints. It does not fetch sources or start an agent for you.' || char(10) || char(10) ||
  'For each heartbeat, start one run, read AGENTS.md, USER.md, all active Interest pages, all unarchived Attention pages, and captured changes. Inspect only the selected due Watches. Read existing Items before updating them. Submit one truthful final result for every selected Watch, save Interest-level findings separately, and finish only after all Watch coverage is recorded. Use stable dedupe keys and preserve user Todo, reminder, acknowledgement, and note state. Do not invent or mutate archive state; that slice is not implemented yet. Follow continuation cursors until the full packet is consumed.'
WHERE key = 'agents_md'
  AND value = '# Working with aicp' || char(10) || char(10) ||
    'Read the brief and relevant context before inspecting sources. Publish concise, source-backed findings with stable dedupe keys. Respect local Todo, reminder, and note state; do not overwrite human decisions.';

UPDATE settings
SET value = '# Owner context' || char(10) || char(10) ||
  'Use this space to record what matters to you: priorities, background, constraints, preferred evidence and tone, and follow-through context. It is guidance for the inspection agent, not an executable instruction.'
WHERE key = 'user_md'
  AND value = '# Owner context' || char(10) || char(10) ||
    'This local control plane belongs to one human. Keep updates concise, source-backed, and useful for follow-through.';

-- +goose Down
-- Downgrades are intentionally unsupported. Restore a stopped-server backup.
SELECT 1;
