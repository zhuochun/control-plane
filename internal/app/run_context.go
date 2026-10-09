package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// WatchAttempt is attributed to the selected source generation, never today's
// configuration. Coverage and limitations are captured together with the Run.
type WatchAttempt struct {
	RunID      string    `json:"run_id"`
	Status     string    `json:"status"`
	RecordedAt time.Time `json:"recorded_at"`
	Error      string    `json:"error,omitempty"`
	Coverage   Coverage  `json:"coverage"`
}

func watchAttempt(ctx context.Context, db querier, id string, generation int64, success bool) (*WatchAttempt, error) {
	query := `SELECT wr.run_id,wr.status,wr.recorded_at,wr.result FROM watch_results wr JOIN runs r ON r.id=wr.run_id
 WHERE wr.watch_id=? AND EXISTS (SELECT 1 FROM json_each(r.selected_watches) sw
 WHERE json_extract(sw.value,'$.id')=wr.watch_id AND json_extract(sw.value,'$.source_generation')=?)`
	if success {
		query += " AND wr.status='success'"
	}
	query += " ORDER BY wr.recorded_at DESC, r.started_at DESC, r.rowid DESC LIMIT 1"
	var item WatchAttempt
	var recorded int64
	var raw string
	err := db.QueryRowContext(ctx, query, id, generation).Scan(&item.RunID, &item.Status, &recorded, &raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result struct {
		Error    string   `json:"error"`
		Coverage Coverage `json:"coverage"`
	}
	if err = json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	item.RecordedAt = time.UnixMilli(recorded).UTC()
	item.Error, item.Coverage = result.Error, result.Coverage
	return &item, nil
}

func scanRunSummary(row scanner) (RunSummary, error) {
	var item RunSummary
	var started int64
	var ended sql.NullInt64
	err := row.Scan(&item.ID, &item.RunnerLabel, &item.Status, &started, &ended, &item.AfterSeq, &item.ThroughSeq, &item.Summary, &item.SelectedCount, &item.SubmittedCount)
	if err != nil {
		return item, missing(err)
	}
	item.StartedAt = time.UnixMilli(started).UTC()
	if ended.Valid {
		value := time.UnixMilli(ended.Int64).UTC()
		item.EndedAt = &value
	}
	return item, nil
}

func requiredInterests(snapshot runContextSnapshot, watches []SelectedWatch) []Interest {
	ids := map[string]bool{}
	for _, watch := range watches {
		for _, interest := range watch.Interests {
			ids[interest.ID] = true
		}
	}
	result := []Interest{}
	for _, interest := range snapshot.Interests {
		if ids[interest.ID] {
			result = append(result, interest)
		}
	}
	return result
}

func runOverview(snapshot runContextSnapshot, run Run, more int) map[string]any {
	selected := map[string]bool{}
	for _, watch := range run.SelectedWatches {
		selected[watch.ID] = true
	}
	related, reminders := 0, 0
	for _, item := range snapshot.Attention {
		if item.WatchID != nil && selected[*item.WatchID] {
			related++
		}
		if item.RemindAt != nil && !item.RemindAt.After(run.StartedAt) {
			reminders++
		}
	}
	return map[string]any{"attention_count": len(snapshot.Attention), "selected_watch_attention_count": related, "due_reminder_count": reminders, "selected_watch_count": len(run.SelectedWatches), "more_due_count": more}
}

func inspectionContext(snapshot runContextSnapshot, run Run, more int, changes []Event, next string) (map[string]any, error) {
	page, err := contextPage(run.ID, "interests", requiredInterests(snapshot, run.SelectedWatches), 0, contextPageSize)
	if err != nil {
		return nil, err
	}
	continuations := map[string]any{}
	if cursor, ok := page["next_cursor"]; ok {
		continuations["interests"] = cursor
	}
	if next != "" {
		continuations["changes"] = next
	}
	inputPage, err := contextPage(run.ID, "user_inputs", snapshot.UserInputs, 0, contextPageSize)
	if err != nil {
		return nil, err
	}
	if cursor, ok := inputPage["next_cursor"]; ok {
		continuations["user_inputs"] = cursor
	}
	watches := append([]SelectedWatch{}, run.SelectedWatches...)
	for i := range watches {
		watches[i].RelatedItems = nil
	}
	return map[string]any{
		"contexts": snapshot.Contexts, "watches": watches, "interests": page["items"], "changes": changes,
		"user_inputs": inputPage,
		"overview":    runOverview(snapshot, run, more), "continuations": continuations,
		"available": map[string]any{"attention": map[string]any{"count": len(snapshot.Attention), "cursor": encodeContextCursor(contextCursor{RunID: run.ID, Collection: "attention_items"})}},
	}, nil
}
