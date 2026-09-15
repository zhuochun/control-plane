-- +goose Up
INSERT INTO settings(key, value)
SELECT 'timezone_mode', CASE WHEN value = 'UTC' THEN 'auto' ELSE 'custom' END
FROM settings
WHERE key = 'timezone'
  AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'timezone_mode');

INSERT INTO settings(key, value)
SELECT 'agents_md',
       '# Working with aicp' || char(10) || char(10) ||
       'Read the brief and relevant context before inspecting sources. Publish concise, source-backed findings with stable dedupe keys. Respect local Todo, reminder, and note state; do not overwrite human decisions.'
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'agents_md');

INSERT INTO settings(key, value)
SELECT 'user_md',
       '# Owner context' || char(10) || char(10) ||
       'This local control plane belongs to one human. Keep updates concise, source-backed, and useful for follow-through. Treat decisions, reminders, and notes here as the owner''s final say.'
WHERE NOT EXISTS (SELECT 1 FROM settings WHERE key = 'user_md');

-- +goose Down
-- Downgrades are intentionally unsupported. Restore a stopped-server backup.
SELECT 1;
