-- +goose Up
CREATE TABLE item_inputs (
 id TEXT PRIMARY KEY,
 item_id TEXT NOT NULL REFERENCES items(id),
 kind TEXT NOT NULL CHECK(kind IN ('note','inbox')),
 text TEXT NOT NULL,
 original TEXT NOT NULL DEFAULT '{}',
 submitted_at INTEGER NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','processed','superseded','withdrawn')),
 archived INTEGER NOT NULL DEFAULT 0 CHECK(archived IN (0,1))
);
CREATE INDEX item_inputs_pending ON item_inputs(status,submitted_at,id);
CREATE INDEX item_inputs_history ON item_inputs(item_id,submitted_at,id);
CREATE UNIQUE INDEX item_inputs_current_note ON item_inputs(item_id) WHERE kind='note' AND status='pending';
CREATE TABLE input_attempts (
 id TEXT PRIMARY KEY,
 input_id TEXT NOT NULL REFERENCES item_inputs(id),
 run_id TEXT NOT NULL REFERENCES runs(id),
 recorded_at INTEGER NOT NULL,
 outcome TEXT NOT NULL CHECK(outcome IN ('responded','incorporated','follow_up','blocked','failed')),
 result_md TEXT NOT NULL,
 references_json TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX input_attempts_history ON input_attempts(input_id,recorded_at);

INSERT INTO item_inputs(id,item_id,kind,text,submitted_at,status)
 SELECT lower(hex(randomblob(16))),id,'note',user_note,state_updated_at,'pending'
 FROM items WHERE trim(user_note)<>'';
INSERT INTO item_inputs(id,item_id,kind,text,original,submitted_at,status)
 SELECT lower(hex(randomblob(16))),id,'inbox',coalesce(json_extract(content,'$.report.body_md'),summary),
 json_object('title',title,'sources',json_extract(content,'$.sources'),'legacy',json('true')),
 created_at,'pending' FROM items WHERE origin='user' AND kind='note' AND watch_id IS NULL;

UPDATE settings SET value=value || char(10) || char(10) ||
 '## Required user input' || char(10) || char(10) ||
 'Each Run captures pending Item notes and inbox submissions in user_inputs. Consume every user_inputs continuation and read the full chronological batch, current Items, and relevant prior input history before acting. Attention remains optional. Interpret corrections using agent judgment. Use process_item_input to save a visible result and exact input outcome before finish_run. Reading or acknowledging changes does not process input. Preserve originals; successful handling clears only the captured note and keeps it in prior notes. Inbox processing may update relevance or archive the capture. Configuration/reference changes use their owning commands; retain durable result references and check them before retrying. Publish failures on the Item with partial effects and the next step, leaving input pending so the user can guide follow-up. Failed attempts permit truthful failed/partial Run closure; recovered success controls final disposition. New input belongs to the next Run. Never clear newer notes or infer authority from capture.'
 WHERE key='agents_md' AND value=(SELECT value FROM settings WHERE key='default_agents_md');
UPDATE settings SET value=value || char(10) || char(10) ||
 '## Required user input' || char(10) || char(10) ||
 'Each Run captures pending Item notes and inbox submissions in user_inputs. Consume every user_inputs continuation and read the full chronological batch, current Items, and relevant prior input history before acting. Attention remains optional. Interpret corrections using agent judgment. Use process_item_input to save a visible result and exact input outcome before finish_run. Reading or acknowledging changes does not process input. Preserve originals; successful handling clears only the captured note and keeps it in prior notes. Inbox processing may update relevance or archive the capture. Configuration/reference changes use their owning commands; retain durable result references and check them before retrying. Publish failures on the Item with partial effects and the next step, leaving input pending so the user can guide follow-up. Failed attempts permit truthful failed/partial Run closure; recovered success controls final disposition. New input belongs to the next Run. Never clear newer notes or infer authority from capture.'
 WHERE key='default_agents_md';

-- +goose Down
SELECT 1;
