-- +goose Up
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO settings VALUES ('timezone', 'UTC'), ('acknowledged_event_seq', '0');

CREATE TABLE interests (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    instructions_md TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('active', 'paused', 'deprecated')),
    revision INTEGER NOT NULL CHECK (revision >= 1),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE watches (
    id TEXT PRIMARY KEY,
    interest_id TEXT NOT NULL REFERENCES interests(id),
    source TEXT NOT NULL CHECK (json_valid(source)),
    instructions_md TEXT NOT NULL,
    interval_seconds INTEGER NOT NULL CHECK (interval_seconds BETWEEN 1 AND 31536000),
    lookback_seconds INTEGER NOT NULL CHECK (lookback_seconds > 0),
    state TEXT NOT NULL CHECK (state IN ('active', 'paused', 'deprecated')),
    revision INTEGER NOT NULL CHECK (revision >= 1),
    cursor TEXT CHECK (cursor IS NULL OR json_valid(cursor)),
    next_due_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX watches_due ON watches(state, next_due_at);
CREATE INDEX watches_interest ON watches(interest_id);

CREATE TABLE items (
    id TEXT PRIMARY KEY,
    dedupe_key TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('note', 'report', 'task', 'outcome')),
    interest_id TEXT REFERENCES interests(id),
    watch_id TEXT REFERENCES watches(id),
    parent_id TEXT REFERENCES items(id),
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    content TEXT NOT NULL CHECK (json_valid(content)),
    content_version INTEGER NOT NULL CHECK (content_version >= 1),
    content_hash TEXT NOT NULL,
    todo_state TEXT NOT NULL CHECK (todo_state IN ('none', 'todo', 'done')),
    remind_at INTEGER,
    reminder_timezone TEXT,
    acknowledged_content_version INTEGER NOT NULL DEFAULT 0,
    user_note TEXT NOT NULL DEFAULT '',
    state_version INTEGER NOT NULL CHECK (state_version >= 1),
    created_at INTEGER NOT NULL,
    content_updated_at INTEGER NOT NULL,
    state_updated_at INTEGER NOT NULL,
    CHECK (acknowledged_content_version BETWEEN 0 AND content_version),
    CHECK ((remind_at IS NULL) = (reminder_timezone IS NULL))
);
CREATE INDEX items_todo ON items(todo_state);
CREATE INDEX items_reminder ON items(remind_at) WHERE remind_at IS NOT NULL;
CREATE INDEX items_interest ON items(interest_id);
CREATE INDEX items_watch ON items(watch_id);
CREATE INDEX items_parent ON items(parent_id);

CREATE TABLE item_versions (
    item_id TEXT NOT NULL REFERENCES items(id),
    content_version INTEGER NOT NULL,
    snapshot TEXT NOT NULL CHECK (json_valid(snapshot)),
    PRIMARY KEY (item_id, content_version)
);

CREATE TABLE proposals (
    id TEXT PRIMARY KEY,
    proposal_key TEXT NOT NULL UNIQUE,
    target_type TEXT NOT NULL CHECK (target_type IN ('interest', 'watch')),
    target_id TEXT,
    expected_revision INTEGER,
    operation TEXT NOT NULL CHECK (operation IN ('create', 'update', 'deprecate')),
    payload TEXT CHECK (payload IS NULL OR json_valid(payload)),
    rationale_md TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'accepted', 'rejected'))
);

CREATE TABLE runs (
    id TEXT PRIMARY KEY,
    runner_label TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'partial', 'failed', 'expired')),
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    lease_expires_at INTEGER NOT NULL,
    selected_watches TEXT NOT NULL CHECK (json_valid(selected_watches)),
    after_seq INTEGER NOT NULL,
    through_seq INTEGER NOT NULL,
    summary TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX runs_one_active ON runs(status) WHERE status = 'running';

CREATE TABLE watch_results (
    run_id TEXT NOT NULL REFERENCES runs(id),
    watch_id TEXT NOT NULL REFERENCES watches(id),
    status TEXT NOT NULL CHECK (status IN ('success', 'partial', 'failed')),
    recorded_at INTEGER NOT NULL,
    result TEXT NOT NULL CHECK (json_valid(result)),
    PRIMARY KEY (run_id, watch_id)
);
CREATE INDEX watch_results_history ON watch_results(watch_id, recorded_at DESC);

CREATE TABLE events (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    occurred_at INTEGER NOT NULL,
    actor TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    change_type TEXT NOT NULL,
    payload TEXT NOT NULL CHECK (json_valid(payload))
);
CREATE INDEX events_entity ON events(entity_type, entity_id, seq);

CREATE TABLE command_receipts (
    request_id TEXT PRIMARY KEY,
    request_hash TEXT NOT NULL,
    response TEXT NOT NULL CHECK (json_valid(response))
);

-- +goose Down
-- Downgrades are intentionally unsupported. Restore a stopped-server backup.
SELECT 1;
