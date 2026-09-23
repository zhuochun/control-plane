package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type Coverage struct {
	CursorBefore    json.RawMessage `json:"cursor_before"`
	CursorAfter     json.RawMessage `json:"cursor_after,omitempty"`
	ObservedThrough time.Time       `json:"observed_through"`
	Limitations     []string        `json:"limitations"`
}
type SubmitWatchFindings struct {
	RequestID             string    `json:"request_id"`
	ExpectedWatchRevision int64     `json:"expected_watch_revision"`
	Status                string    `json:"status"`
	Error                 string    `json:"error,omitempty"`
	Coverage              Coverage  `json:"coverage"`
	Items                 []PutItem `json:"items"`
}
type SubmittedItem struct {
	ID             string `json:"id"`
	ContentVersion int64  `json:"content_version"`
	StateVersion   int64  `json:"state_version"`
	Changed        bool   `json:"changed"`
}
type WatchResult struct {
	RunID      string          `json:"run_id"`
	WatchID    string          `json:"watch_id"`
	Status     string          `json:"status"`
	RecordedAt time.Time       `json:"recorded_at"`
	Result     json.RawMessage `json:"result"`
}

func rawJSONEqual(left, right json.RawMessage) bool {
	var a, b any
	if len(left) == 0 {
		left = []byte("null")
	}
	if len(right) == 0 {
		right = []byte("null")
	}
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	one, _ := json.Marshal(a)
	two, _ := json.Marshal(b)
	return bytes.Equal(one, two)
}
func selectedWatch(run Run, id string) (SelectedWatch, bool) {
	for _, watch := range run.SelectedWatches {
		if watch.ID == id {
			return watch, true
		}
	}
	return SelectedWatch{}, false
}

func (a *App) SubmitWatchFindings(ctx context.Context, runID, watchID string, input SubmitWatchFindings) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "PUT /runs/"+runID+"/watches/"+watchID+"/findings", input, func(tx *sql.Tx) (any, error) {
		run, err := getRun(ctx, tx, runID)
		if err != nil {
			return nil, err
		}
		runID = run.ID
		canonicalWatchID, err := resolveID(ctx, tx, "watches", watchID)
		if err != nil {
			return nil, err
		}
		watchID = canonicalWatchID
		now := a.Now().UTC()
		if err = liveRun(run); err != nil {
			return nil, err
		}
		captured, ok := selectedWatch(run, watchID)
		if !ok {
			return nil, Invalid("Watch was not selected for this run")
		}
		var prior int
		err = tx.QueryRowContext(ctx, `SELECT 1 FROM watch_results WHERE run_id=? AND watch_id=?`, runID, watchID).Scan(&prior)
		if err == nil {
			return nil, &Error{Status: 409, Code: "watch_result_exists", Message: "This Run and Watch already have a final result"}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		watch, err := getWatch(ctx, tx, watchID)
		if err != nil {
			return nil, err
		}
		interest, err := getInterest(ctx, tx, watch.InterestID)
		if err != nil {
			return nil, err
		}
		if watch.State != "active" || watch.Revision != captured.Revision || input.ExpectedWatchRevision != captured.Revision {
			return nil, &Error{Status: 409, Code: "watch_changed", Message: "Watch configuration changed during the run"}
		}
		if interest.State != "active" || interest.Revision != captured.InterestRevision {
			return nil, &Error{Status: 409, Code: "interest_changed", Message: "Interest configuration changed during the run"}
		}
		if !rawJSONEqual(input.Coverage.CursorBefore, captured.Cursor) {
			return nil, Invalid("coverage.cursor_before must match the captured Watch cursor")
		}
		if input.Coverage.ObservedThrough.IsZero() {
			return nil, Invalid("coverage.observed_through is required")
		}
		if input.Coverage.Limitations == nil {
			input.Coverage.Limitations = []string{}
		}
		switch input.Status {
		case "success":
			if input.Error != "" {
				return nil, Invalid("a successful result cannot contain error")
			}
			if len(input.Coverage.CursorAfter) == 0 {
				return nil, Invalid("successful coverage needs cursor_after, which may be null")
			}
		case "partial":
			if input.Error == "" {
				return nil, Invalid("a partial result needs error")
			}
			if len(input.Coverage.CursorAfter) > 0 && !rawJSONEqual(input.Coverage.CursorAfter, nil) {
				return nil, Invalid("partial coverage cannot advance cursor")
			}
		case "failed":
			if input.Error == "" || len(input.Items) > 0 {
				return nil, Invalid("a failed result needs error and cannot contain items")
			}
			if len(input.Coverage.CursorAfter) > 0 && !rawJSONEqual(input.Coverage.CursorAfter, nil) {
				return nil, Invalid("failed coverage cannot advance cursor")
			}
		default:
			return nil, Invalid("status must be success, partial, or failed")
		}
		submitted := make([]SubmittedItem, 0, len(input.Items))
		for _, entry := range input.Items {
			if entry.RequestID != "" {
				return nil, Invalid("nested item entries do not contain request_id")
			}
			if entry.WatchID != nil && *entry.WatchID != watchID {
				return nil, Invalid("item watch_id conflicts with the submitting Watch")
			}
			if entry.InterestID != nil && *entry.InterestID != watch.InterestID {
				return nil, Invalid("item interest_id conflicts with the submitting Watch")
			}
			if len(entry.Sources) == 0 {
				return nil, Invalid("items submitted by a Watch need at least one source")
			}
			entry.WatchID = &watchID
			entry.InterestID = &watch.InterestID
			result, err := a.putItemTx(ctx, tx, "", entry)
			if err != nil {
				return nil, err
			}
			item := result.(Item)
			submitted = append(submitted, SubmittedItem{ID: item.ID, ContentVersion: item.ContentVersion, StateVersion: item.StateVersion, Changed: item.ContentVersion != entry.ExpectedContentVersion})
		}
		if input.Status == "success" {
			var cursor any
			if !rawJSONEqual(input.Coverage.CursorAfter, nil) {
				cursor = string(input.Coverage.CursorAfter)
			}
			next := watch.NextDueAt
			if !run.StartedAt.Before(next) {
				interval := time.Duration(watch.IntervalSeconds) * time.Second
				steps := run.StartedAt.Sub(next)/interval + 1
				next = next.Add(steps * interval)
			}
			_, err = tx.ExecContext(ctx, `UPDATE watches SET cursor=?,next_due_at=? WHERE id=?`, cursor, next.UnixMilli(), watchID)
			if err != nil {
				return nil, err
			}
		}
		record, err := json.Marshal(struct {
			Error    string          `json:"error,omitempty"`
			Coverage Coverage        `json:"coverage"`
			Items    []SubmittedItem `json:"items"`
		}{Error: input.Error, Coverage: input.Coverage, Items: submitted})
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO watch_results(run_id,watch_id,status,recorded_at,result) VALUES(?,?,?,?,?)`, runID, watchID, input.Status, now.UnixMilli(), string(record)); err != nil {
			return nil, err
		}
		return map[string]any{"run_id": runID, "watch_id": watchID, "status": input.Status, "items": submitted}, nil
	})
}

func (a *App) SubmitActiveWatchFindings(ctx context.Context, watchID string, input SubmitWatchFindings) (json.RawMessage, error) {
	run, err := a.ActiveRun(ctx)
	if err != nil {
		return nil, err
	}
	return a.SubmitWatchFindings(ctx, run.ID, watchID, input)
}

func (a *App) WatchResults(ctx context.Context, runID string) ([]WatchResult, error) {
	canonical, err := resolveID(ctx, a.Store.DB, "runs", runID)
	if err != nil {
		return nil, err
	}
	rows, err := a.Store.DB.QueryContext(ctx, `SELECT run_id,watch_id,status,recorded_at,result FROM watch_results WHERE run_id=? ORDER BY recorded_at,watch_id`, canonical)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []WatchResult{}
	for rows.Next() {
		var item WatchResult
		var recorded int64
		var result string
		if err = rows.Scan(&item.RunID, &item.WatchID, &item.Status, &recorded, &result); err != nil {
			return nil, err
		}
		item.RecordedAt = time.UnixMilli(recorded).UTC()
		item.Result = json.RawMessage(result)
		results = append(results, item)
	}
	return results, rows.Err()
}
