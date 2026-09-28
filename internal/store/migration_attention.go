package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Version 6 replaces the old Attention response fields in captured Run data.
// Keep this conversion frozen: later response changes must get a new migration.
func migrateAttentionSnapshots(ctx context.Context, tx *sql.Tx) error {
	ids, err := migrationIDs(ctx, tx, `SELECT id FROM runs WHERE instr(context_snapshot, '"attention_items"')>0 OR instr(selected_watches, '"related_items"')>0`)
	if err != nil {
		return err
	}
	for _, id := range ids {
		var contextJSON, watchesJSON string
		if err = tx.QueryRowContext(ctx, `SELECT context_snapshot, selected_watches FROM runs WHERE id=?`, id).Scan(&contextJSON, &watchesJSON); err != nil {
			return err
		}
		oldContextJSON, oldWatchesJSON := contextJSON, watchesJSON
		contextJSON, err = migrateRunContextJSON(contextJSON)
		if err != nil {
			return fmt.Errorf("run %s context snapshot: %w", id, err)
		}
		watchesJSON, err = migrateSelectedWatchesJSON(watchesJSON)
		if err != nil {
			return fmt.Errorf("run %s selected watches: %w", id, err)
		}
		if contextJSON == oldContextJSON && watchesJSON == oldWatchesJSON {
			continue
		}
		if _, err = tx.ExecContext(ctx, `UPDATE runs SET context_snapshot=?, selected_watches=? WHERE id=?`, contextJSON, watchesJSON, id); err != nil {
			return err
		}
	}
	return migrateAttentionReceipts(ctx, tx)
}

func migrateAttentionReceipts(ctx context.Context, tx *sql.Tx) error {
	ids, err := migrationIDs(ctx, tx, `SELECT request_id FROM command_receipts WHERE instr(response, '"brief"')>0`)
	if err != nil {
		return err
	}
	for _, id := range ids {
		var raw string
		if err = tx.QueryRowContext(ctx, `SELECT response FROM command_receipts WHERE request_id=?`, id).Scan(&raw); err != nil {
			return err
		}
		converted, err := migrateRunReceiptJSON(raw)
		if err != nil {
			return fmt.Errorf("command receipt %s: %w", id, err)
		}
		if converted != raw {
			if _, err = tx.ExecContext(ctx, `UPDATE command_receipts SET response=? WHERE request_id=?`, converted, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// Collect IDs before updates so the SQLite cursor is closed during writes.
func migrationIDs(ctx context.Context, tx *sql.Tx, query string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func migrateRunReceiptJSON(raw string) (string, error) {
	var response map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return "", err
	}
	briefRaw, ok := response["brief"]
	if !ok {
		return raw, nil
	}
	var brief map[string]json.RawMessage
	if err := json.Unmarshal(briefRaw, &brief); err != nil {
		return "", err
	}
	changed, err := migrateAttentionArray(brief, "attention_items")
	if err != nil {
		return "", err
	}
	if watchRaw, ok := brief["watches"]; ok && string(watchRaw) != "null" {
		converted, err := migrateSelectedWatchesJSON(string(watchRaw))
		if err != nil {
			return "", err
		}
		if converted != string(watchRaw) {
			brief["watches"] = json.RawMessage(converted)
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	encoded, err := json.Marshal(brief)
	if err != nil {
		return "", err
	}
	response["brief"] = encoded
	return marshalMigrationJSON(response)
}

func migrateRunContextJSON(raw string) (string, error) {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return "", err
	}
	if len(snapshot) == 0 {
		return raw, nil
	}
	changed, err := migrateAttentionArray(snapshot, "attention_items")
	if err != nil || !changed {
		return raw, err
	}
	return marshalMigrationJSON(snapshot)
}

func migrateSelectedWatchesJSON(raw string) (string, error) {
	var watches []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &watches); err != nil {
		return "", err
	}
	changed := false
	for _, watch := range watches {
		converted, err := migrateAttentionArray(watch, "related_items")
		if err != nil {
			return "", err
		}
		changed = changed || converted
	}
	if !changed {
		return raw, nil
	}
	return marshalMigrationJSON(watches)
}

func migrateAttentionArray(parent map[string]json.RawMessage, key string) (bool, error) {
	raw, ok := parent[key]
	if !ok || string(raw) == "null" {
		return false, nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return false, err
	}
	if len(items) == 0 {
		return false, nil
	}
	for _, item := range items {
		if err := migrateAttentionItem(item); err != nil {
			return false, err
		}
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return false, err
	}
	parent[key] = encoded
	return true, nil
}

func migrateAttentionItem(item map[string]json.RawMessage) error {
	var contentVersion, acknowledgedVersion int64
	if err := json.Unmarshal(item["content_version"], &contentVersion); err != nil {
		return fmt.Errorf("content_version: %w", err)
	}
	if raw, ok := item["acknowledged_content_version"]; ok {
		if err := json.Unmarshal(raw, &acknowledgedVersion); err != nil {
			return fmt.Errorf("acknowledged_content_version: %w", err)
		}
	}
	delete(item, "acknowledged_content_version")
	delete(item, "state_version")
	if acknowledgedVersion < contentVersion {
		item["unacknowledged"] = json.RawMessage("true")
	} else {
		delete(item, "unacknowledged")
	}
	var truncated []string
	for _, field := range []struct {
		key   string
		limit int
	}{{"title", 256}, {"summary", 512}} {
		wasTruncated, err := capMigrationField(item, field.key, field.limit)
		if err != nil {
			return err
		}
		if wasTruncated {
			truncated = append(truncated, field.key)
		}
	}
	if raw, ok := item["interests"]; ok && string(raw) != "null" {
		var interests []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &interests); err != nil {
			return fmt.Errorf("interests: %w", err)
		}
		reasonTruncated := false
		for _, interest := range interests {
			clipped, err := capMigrationField(interest, "reason", 256)
			if err != nil {
				return err
			}
			reasonTruncated = reasonTruncated || clipped
		}
		if reasonTruncated {
			encoded, err := json.Marshal(interests)
			if err != nil {
				return err
			}
			item["interests"] = encoded
			truncated = append(truncated, "interest_reason")
		}
	}
	if len(truncated) > 0 {
		encoded, err := json.Marshal(truncated)
		if err != nil {
			return err
		}
		item["truncated_fields"] = encoded
	} else {
		delete(item, "truncated_fields")
	}
	return nil
}

func capMigrationField(item map[string]json.RawMessage, field string, limit int) (bool, error) {
	raw, ok := item[field]
	if !ok {
		return false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("%s: %w", field, err)
	}
	if len(value) <= limit {
		return false, nil
	}
	cut := 0
	for index := range value {
		if index > limit-len("…") {
			break
		}
		cut = index
	}
	encoded, err := json.Marshal(value[:cut] + "…")
	if err != nil {
		return false, err
	}
	item[field] = encoded
	return true, nil
}

func marshalMigrationJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	return string(encoded), err
}
