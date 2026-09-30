package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Version 7 moves retry receipts to the inspection contract. Full optional
// Attention remains in the original Run snapshot, never reconstructed live.
func migrateInspectionReceipts(ctx context.Context, tx *sql.Tx) error {
	ids, err := migrationIDs(ctx, tx, `SELECT request_id FROM command_receipts WHERE instr(response,'"brief"')>0`)
	if err != nil {
		return err
	}
	for _, id := range ids {
		var raw string
		if err = tx.QueryRowContext(ctx, "SELECT response FROM command_receipts WHERE request_id=?", id).Scan(&raw); err != nil {
			return err
		}
		var response map[string]json.RawMessage
		if err = json.Unmarshal([]byte(raw), &response); err != nil {
			return fmt.Errorf("receipt %s: %w", id, err)
		}
		if _, ok := response["brief"]; !ok {
			continue
		}
		var run struct {
			ID string `json:"id"`
		}
		if err = json.Unmarshal(response["run"], &run); err != nil {
			return err
		}
		var snapshotRaw, selectedRaw string
		var started int64
		if err = tx.QueryRowContext(ctx, "SELECT context_snapshot,selected_watches,started_at FROM runs WHERE id=?", run.ID).Scan(&snapshotRaw, &selectedRaw, &started); err != nil {
			return fmt.Errorf("receipt %s captured Run: %w", id, err)
		}
		var snapshot struct {
			Interests []map[string]json.RawMessage `json:"interests"`
			Attention []struct {
				WatchID  *string    `json:"watch_id"`
				RemindAt *time.Time `json:"remind_at"`
			} `json:"attention_items"`
			Contexts map[string]string `json:"contexts"`
		}
		if err = json.Unmarshal([]byte(snapshotRaw), &snapshot); err != nil {
			return err
		}
		var old struct {
			Watches []map[string]json.RawMessage `json:"watches"`
			Changes []json.RawMessage            `json:"changes"`
			Next    string                       `json:"changes_next_cursor"`
			More    int                          `json:"more_due_count"`
		}
		if err = json.Unmarshal(response["brief"], &old); err != nil {
			return err
		}
		// Older receipts may still have the single-parent Watch response. The
		// stored selected scope was already normalized by version 4.
		if err = json.Unmarshal([]byte(selectedRaw), &old.Watches); err != nil {
			return err
		}
		selected := map[string]bool{}
		applicable := map[string]bool{}
		for _, watch := range old.Watches {
			var watchID string
			if err = json.Unmarshal(watch["id"], &watchID); err != nil {
				return err
			}
			selected[watchID] = true
			var interests []struct {
				ID string `json:"id"`
			}
			if data, ok := watch["interests"]; ok {
				if err = json.Unmarshal(data, &interests); err != nil {
					return err
				}
			}
			for _, interest := range interests {
				applicable[interest.ID] = true
			}
			delete(watch, "related_items")
		}
		interests := []map[string]json.RawMessage{}
		for _, interest := range snapshot.Interests {
			var interestID string
			if err = json.Unmarshal(interest["id"], &interestID); err != nil {
				return err
			}
			if applicable[interestID] {
				interests = append(interests, interest)
			}
		}
		boundary := min(50, len(interests))
		for boundary > 1 {
			page, marshalErr := json.Marshal(interests[:boundary])
			if marshalErr != nil {
				return marshalErr
			}
			if len(page) <= 64<<10 {
				break
			}
			boundary--
		}
		continuation := map[string]any{}
		if boundary < len(interests) {
			continuation["interests"] = migrationContextCursor(run.ID, "interests", boundary)
		}
		if old.Next != "" {
			continuation["changes"] = old.Next
		}
		related, reminders := 0, 0
		for _, item := range snapshot.Attention {
			if item.WatchID != nil && selected[*item.WatchID] {
				related++
			}
			if item.RemindAt != nil && item.RemindAt.UnixMilli() <= started {
				reminders++
			}
		}
		if old.Watches == nil {
			old.Watches = []map[string]json.RawMessage{}
		}
		if old.Changes == nil {
			old.Changes = []json.RawMessage{}
		}
		packet := map[string]any{"contexts": snapshot.Contexts, "watches": old.Watches, "interests": interests[:boundary], "changes": old.Changes, "continuations": continuation,
			"overview":  map[string]any{"attention_count": len(snapshot.Attention), "selected_watch_attention_count": related, "due_reminder_count": reminders, "selected_watch_count": len(old.Watches), "more_due_count": old.More},
			"available": map[string]any{"attention": map[string]any{"count": len(snapshot.Attention), "cursor": migrationContextCursor(run.ID, "attention_items", 0)}}}
		encoded, marshalErr := json.Marshal(packet)
		if marshalErr != nil {
			return marshalErr
		}
		response["context"] = encoded
		delete(response, "brief")
		var runFields map[string]json.RawMessage
		if err = json.Unmarshal(response["run"], &runFields); err != nil {
			return err
		}
		delete(runFields, "selected_watches")
		delete(runFields, "lease_expires_at")
		response["run"], err = json.Marshal(runFields)
		if err != nil {
			return err
		}
		migrated, marshalErr := json.Marshal(response)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = tx.ExecContext(ctx, "UPDATE command_receipts SET response=? WHERE request_id=?", string(migrated), id); err != nil {
			return err
		}
	}
	var oldDefault string
	if err = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='default_agents_md'").Scan(&oldDefault); err != nil {
		return err
	}
	oldSentence := "For each heartbeat, start one Run and consume every Interest, Attention, and change continuation page."
	newSentence := "For each heartbeat, start one Run and read its captured AGENTS.md, USER.md, applicable Interest instructions, and every captured change page. Attention is optional, not a global Todo execution queue. Use bounded Item lookup by dedupe key or Watcher before updating an existing matter. A live brief is read-only orientation, not a Run snapshot."
	updated := strings.Replace(oldDefault, oldSentence, newSentence, 1)
	if updated != oldDefault {
		if _, err = tx.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='agents_md' AND value=?", updated, oldDefault); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='default_agents_md'", updated); err != nil {
			return err
		}
	}
	return nil
}

func migrationContextCursor(runID, collection string, offset int) string {
	data, _ := json.Marshal(struct {
		RunID      string `json:"run_id,omitempty"`
		Collection string `json:"collection"`
		Offset     int    `json:"offset"`
	}{runID, collection, offset})
	return base64.RawURLEncoding.EncodeToString(data)
}
