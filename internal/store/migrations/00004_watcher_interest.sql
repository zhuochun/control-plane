-- +goose Up
ALTER TABLE interests ADD COLUMN slug TEXT;
UPDATE interests SET slug='interest-' || substr(replace(id, '-', ''), 1, 12);
CREATE UNIQUE INDEX interests_slug ON interests(slug);

ALTER TABLE watches ADD COLUMN slug TEXT;
ALTER TABLE watches ADD COLUMN matching_policy TEXT NOT NULL DEFAULT 'explicit' CHECK (matching_policy IN ('broad', 'explicit'));
ALTER TABLE watches ADD COLUMN valid_until INTEGER;
ALTER TABLE watches ADD COLUMN source_generation INTEGER NOT NULL DEFAULT 1 CHECK (source_generation >= 1);
UPDATE watches SET slug='watcher-' || substr(replace(id, '-', ''), 1, 12);
CREATE UNIQUE INDEX watches_slug ON watches(slug);

CREATE TABLE watch_interests (
    watch_id TEXT NOT NULL REFERENCES watches(id),
    interest_id TEXT NOT NULL REFERENCES interests(id),
    linked_at INTEGER NOT NULL,
    PRIMARY KEY (watch_id, interest_id)
);
CREATE INDEX watch_interests_interest ON watch_interests(interest_id, watch_id);
INSERT INTO watch_interests(watch_id, interest_id, linked_at)
SELECT id, interest_id, created_at FROM watches;

CREATE TABLE item_interests (
    item_id TEXT NOT NULL REFERENCES items(id),
    interest_id TEXT NOT NULL REFERENCES interests(id),
    reason TEXT NOT NULL,
    PRIMARY KEY (item_id, interest_id)
);
CREATE INDEX item_interests_interest ON item_interests(interest_id, item_id);
INSERT INTO item_interests(item_id, interest_id, reason)
SELECT id, interest_id, 'Legacy Interest assignment; original relevance explanation unavailable.'
FROM items WHERE interest_id IS NOT NULL;
ALTER TABLE items ADD COLUMN origin TEXT NOT NULL DEFAULT 'agent' CHECK (origin IN ('agent', 'user', 'legacy'));
UPDATE items SET origin='legacy' WHERE watch_id IS NULL;

UPDATE proposals SET payload=json_remove(json_set(payload,
    '$.matching_policy', 'explicit',
    '$.interest_ids', json_array(json_extract(payload, '$.interest_id'))
), '$.interest_id')
WHERE target_type='watch' AND operation='create' AND payload IS NOT NULL;
CREATE TABLE proposals_next (
    id TEXT PRIMARY KEY,
    proposal_key TEXT NOT NULL UNIQUE,
    target_type TEXT NOT NULL CHECK (target_type IN ('interest', 'watch', 'config_plan')),
    target_id TEXT,
    expected_revision INTEGER,
    operation TEXT NOT NULL CHECK (operation IN ('create', 'update', 'deprecate', 'apply')),
    payload TEXT CHECK (payload IS NULL OR json_valid(payload)),
    rationale_md TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'accepted', 'rejected')),
    evidence_links TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(evidence_links)),
    confidence REAL,
    duplicate_of TEXT REFERENCES proposals_next(id),
    snoozed_until INTEGER,
    expires_at INTEGER,
    merged_into TEXT REFERENCES proposals_next(id)
);
INSERT INTO proposals_next(id,proposal_key,target_type,target_id,expected_revision,operation,payload,rationale_md,state)
SELECT id,proposal_key,target_type,target_id,expected_revision,operation,payload,rationale_md,state FROM proposals;
DROP TABLE proposals;
ALTER TABLE proposals_next RENAME TO proposals;

-- Historical runs keep their captured association, never today's links.
UPDATE runs SET selected_watches = (
    SELECT COALESCE(json_group_array(
        json_remove(json_set(value, '$.source_generation', 1, '$.matching_policy', 'explicit', '$.interests', json_array(json_object(
            'id', json_extract(value, '$.interest_id'),
            'revision', json_extract(value, '$.interest_revision')
        ))), '$.interest_id', '$.interest_revision')
    ), '[]')
    FROM json_each(runs.selected_watches) AS w
);

-- Keep existing foreign keys valid while removing the single-parent model.
DROP INDEX watches_interest;
ALTER TABLE watches DROP COLUMN interest_id;
DROP INDEX items_interest;
ALTER TABLE items DROP COLUMN interest_id;

-- Upgrade only the v3 seeded agent guidance. Preserve owner edits.
UPDATE settings
SET value = '# Working with aicp' || char(10) || char(10) ||
  'aicp is a local control plane. Watchers define bounded source inputs; Interests explain relevance. aicp does not fetch sources or start an agent.' || char(10) || char(10) ||
  'For each heartbeat, start one Run and consume every Interest, Attention, and change continuation page. Inspect only selected due Watchers against the Interest IDs and revisions captured by the Run. Read an Item before updating it. Every source-derived finding needs a concrete Item source reference and a reason for each applicable Interest. Submit one truthful terminal result per selected Watcher; only successful coverage can advance its source cursor. Then finish the Run. Configuration reads and edits happen directly without starting a Run. New and revised Interests apply from a future Run, and in-flight results use their captured configuration. Preserve user Todo, reminder, acknowledgement, and note state.'
WHERE key='agents_md' AND value = '# Working with aicp' || char(10) || char(10) ||
  'aicp is your local control plane for Interests, Watches, Items, user changes, and source checkpoints. It does not fetch sources or start an agent for you.' || char(10) || char(10) ||
  'For each heartbeat, start one run, read AGENTS.md, USER.md, all active Interest pages, all unarchived Attention pages, and captured changes. Inspect only the selected due Watches. Read existing Items before updating them. Submit one truthful final result for every selected Watch, save Interest-level findings separately, and finish only after all Watch coverage is recorded. Use stable dedupe keys and preserve user Todo, reminder, acknowledgement, and note state. Do not invent or mutate archive state; that slice is not implemented yet. Follow continuation cursors until the full packet is consumed.';

-- +goose Down
-- Downgrades are intentionally unsupported. Restore a stopped-server backup.
SELECT 1;
