package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const runLease = 30 * time.Minute

type SelectedWatch struct {
	ID                     string          `json:"id"`
	Revision               int64           `json:"revision"`
	InterestID             string          `json:"interest_id"`
	InterestRevision       int64           `json:"interest_revision"`
	Source                 WatchSource     `json:"source"`
	InstructionsMD         string          `json:"instructions_md"`
	InterestInstructionsMD string          `json:"interest_instructions_md"`
	Cursor                 json.RawMessage `json:"cursor"`
	IntervalSeconds        int64           `json:"interval_seconds"`
	LookbackSeconds        int64           `json:"lookback_seconds"`
	NextDueAt              time.Time       `json:"next_due_at"`
	RelatedItemIDs         []string        `json:"related_item_ids"`
}

type AttentionItem struct {
	ID                         string     `json:"id"`
	Kind                       string     `json:"kind"`
	InterestID                 *string    `json:"interest_id,omitempty"`
	WatchID                    *string    `json:"watch_id,omitempty"`
	ParentID                   *string    `json:"parent_id,omitempty"`
	Title                      string     `json:"title"`
	Summary                    string     `json:"summary"`
	ContentVersion             int64      `json:"content_version"`
	TodoState                  string     `json:"todo_state"`
	RemindAt                   *time.Time `json:"remind_at,omitempty"`
	AcknowledgedContentVersion int64      `json:"acknowledged_content_version"`
	StateVersion               int64      `json:"state_version"`
}

type Run struct {
	ID              string          `json:"id"`
	RunnerLabel     string          `json:"runner_label"`
	Status          string          `json:"status"`
	StartedAt       time.Time       `json:"started_at"`
	EndedAt         *time.Time      `json:"ended_at"`
	LeaseExpiresAt  time.Time       `json:"lease_expires_at"`
	SelectedWatches []SelectedWatch `json:"selected_watches"`
	AfterSeq        int64           `json:"after_seq"`
	ThroughSeq      int64           `json:"through_seq"`
	Summary         string          `json:"summary"`
}

type StartRun struct {
	RequestID   string          `json:"request_id"`
	RunnerLabel string          `json:"runner_label"`
	WatchIDs    Field[[]string] `json:"watch_ids"`
	Force       bool            `json:"force,omitempty"`
}
type RenewRun struct {
	RequestID string `json:"request_id"`
}
type FinishRun struct {
	RequestID     string `json:"request_id"`
	Summary       string `json:"summary"`
	AckThroughSeq *int64 `json:"ack_through_seq,omitempty"`
}

func scanRun(row scanner) (Run, error) {
	var run Run
	var started, lease int64
	var ended sql.NullInt64
	var selected string
	err := row.Scan(&run.ID, &run.RunnerLabel, &run.Status, &started, &ended, &lease, &selected, &run.AfterSeq, &run.ThroughSeq, &run.Summary)
	if err != nil {
		return run, missing(err)
	}
	run.StartedAt = time.UnixMilli(started).UTC()
	run.LeaseExpiresAt = time.UnixMilli(lease).UTC()
	if ended.Valid {
		value := time.UnixMilli(ended.Int64).UTC()
		run.EndedAt = &value
	}
	if err = json.Unmarshal([]byte(selected), &run.SelectedWatches); err != nil {
		return run, err
	}
	return run, nil
}

const runColumns = `id,runner_label,status,started_at,ended_at,lease_expires_at,selected_watches,after_seq,through_seq,summary`

func getRun(ctx context.Context, db querier, id string) (Run, error) {
	return scanRun(db.QueryRowContext(ctx, "SELECT "+runColumns+" FROM runs WHERE id=?", id))
}
func (a *App) Run(ctx context.Context, id string) (Run, error) { return getRun(ctx, a.Store.DB, id) }
func (a *App) RunDetail(ctx context.Context, id string) (map[string]any, error) {
	run, err := a.Run(ctx, id)
	if err != nil {
		return nil, err
	}
	results, err := a.WatchResults(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"run": run, "results": results}, nil
}
func (a *App) Runs(ctx context.Context) ([]Run, error) {
	rows, err := a.Store.DB.QueryContext(ctx, "SELECT "+runColumns+" FROM runs ORDER BY started_at DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Run{}
	for rows.Next() {
		item, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func selectWatches(ctx context.Context, tx *sql.Tx, input StartRun, now int64) ([]SelectedWatch, int, error) {
	if input.Force && !input.WatchIDs.Set {
		return nil, 0, Invalid("force requires explicit watch_ids")
	}
	query := `SELECT w.id,w.revision,w.interest_id,i.revision,w.source,w.instructions_md,i.instructions_md,w.cursor,w.interval_seconds,w.lookback_seconds,w.next_due_at
FROM watches w JOIN interests i ON i.id=w.interest_id WHERE w.state='active' AND i.state='active'`
	args := []any{}
	if input.WatchIDs.Set {
		if len(input.WatchIDs.Value) == 0 {
			return []SelectedWatch{}, 0, nil
		}
		query += " AND w.id IN ("
		for index, id := range input.WatchIDs.Value {
			if index > 0 {
				query += ","
			}
			query += "?"
			args = append(args, id)
		}
		query += ")"
		if !input.Force {
			query += " AND w.next_due_at<=?"
			args = append(args, now)
		}
	} else {
		query += " AND w.next_due_at<=?"
		args = append(args, now)
	}
	var eligible int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM ("+query+")", args...).Scan(&eligible); err != nil {
		return nil, 0, err
	}
	if input.WatchIDs.Set && eligible != len(input.WatchIDs.Value) {
		return nil, 0, Invalid("Every selected Watch must exist, be active with an active Interest, and be due unless forced")
	}
	query += " ORDER BY w.next_due_at,w.id LIMIT 20"
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	selected := []SelectedWatch{}
	for rows.Next() {
		var item SelectedWatch
		var source string
		var due int64
		var nullable sql.NullString
		if err = rows.Scan(&item.ID, &item.Revision, &item.InterestID, &item.InterestRevision, &source, &item.InstructionsMD, &item.InterestInstructionsMD, &nullable, &item.IntervalSeconds, &item.LookbackSeconds, &due); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(source), &item.Source); err != nil {
			return nil, 0, err
		}
		if nullable.Valid {
			item.Cursor = json.RawMessage(nullable.String)
		}
		item.NextDueAt = time.UnixMilli(due).UTC()
		selected = append(selected, item)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	if err = rows.Close(); err != nil {
		return nil, 0, err
	}
	for index := range selected {
		selected[index].RelatedItemIDs = []string{}
		itemRows, itemErr := tx.QueryContext(ctx, `SELECT id FROM items WHERE watch_id=? ORDER BY content_updated_at DESC,id LIMIT 20`, selected[index].ID)
		if itemErr != nil {
			return nil, 0, itemErr
		}
		for itemRows.Next() {
			var id string
			if itemErr = itemRows.Scan(&id); itemErr != nil {
				itemRows.Close()
				return nil, 0, itemErr
			}
			selected[index].RelatedItemIDs = append(selected[index].RelatedItemIDs, id)
		}
		if itemErr = itemRows.Close(); itemErr != nil {
			return nil, 0, itemErr
		}
	}
	more := eligible - len(selected)
	return selected, more, nil
}

func (a *App) StartRun(ctx context.Context, input StartRun) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /runs", input, func(tx *sql.Tx) (any, error) {
		if input.RunnerLabel == "" {
			return nil, Invalid("runner_label is required")
		}
		now := a.Now().UTC()
		nowMillis := now.UnixMilli()
		_, err := tx.ExecContext(ctx, `UPDATE runs SET status='expired',ended_at=? WHERE status='running' AND lease_expires_at<=?`, nowMillis, nowMillis)
		if err != nil {
			return nil, err
		}
		var active string
		err = tx.QueryRowContext(ctx, `SELECT id FROM runs WHERE status='running'`).Scan(&active)
		if err == nil {
			return nil, &Error{Status: 409, Code: "run_in_progress", Message: "Another run holds the global lease.", Retryable: true, Details: map[string]any{"run_id": active}}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		selected, more, err := selectWatches(ctx, tx, input, nowMillis)
		if err != nil {
			return nil, err
		}
		var afterText string
		if err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='acknowledged_event_seq'`).Scan(&afterText); err != nil {
			return nil, err
		}
		after, err := strconv.ParseInt(afterText, 10, 64)
		if err != nil {
			return nil, err
		}
		var through int64
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM events`).Scan(&through); err != nil {
			return nil, err
		}
		id := uuid.NewString()
		selectedJSON, _ := json.Marshal(selected)
		lease := now.Add(runLease)
		_, err = tx.ExecContext(ctx, `INSERT INTO runs(id,runner_label,status,started_at,lease_expires_at,selected_watches,after_seq,through_seq) VALUES(?,?,'running',?,?,?,?,?)`, id, input.RunnerLabel, nowMillis, lease.UnixMilli(), string(selectedJSON), after, through)
		if err != nil {
			return nil, err
		}
		run, err := getRun(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"run": run, "brief": map[string]any{"watches": selected, "after_seq": after, "through_seq": through, "more_due_count": more}}, nil
	})
}

func liveRun(run Run, now time.Time) error {
	if run.Status != "running" {
		return &Error{Status: 409, Code: "run_finished", Message: "Run is no longer active"}
	}
	if !run.LeaseExpiresAt.After(now) {
		return &Error{Status: 409, Code: "lease_expired", Message: "Run lease expired"}
	}
	return nil
}
func (a *App) RenewRun(ctx context.Context, id string, input RenewRun) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /runs/"+id+"/renew", input, func(tx *sql.Tx) (any, error) {
		run, err := getRun(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		now := a.Now().UTC()
		if err = liveRun(run, now); err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE runs SET lease_expires_at=? WHERE id=?`, now.Add(runLease).UnixMilli(), id)
		if err != nil {
			return nil, err
		}
		return getRun(ctx, tx, id)
	})
}

func (a *App) FinishRun(ctx context.Context, id string, input FinishRun) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /runs/"+id+"/finish", input, func(tx *sql.Tx) (any, error) {
		run, err := getRun(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		now := a.Now().UTC()
		if err = liveRun(run, now); err != nil {
			return nil, err
		}
		var success, partial int
		rows, err := tx.QueryContext(ctx, `SELECT status,count(*) FROM watch_results WHERE run_id=? GROUP BY status`, id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var status string
			var count int
			if err = rows.Scan(&status, &count); err != nil {
				rows.Close()
				return nil, err
			}
			switch status {
			case "success":
				success = count
			case "partial":
				partial = count
			case "failed":
			}
		}
		rows.Close()
		total := len(run.SelectedWatches)
		status := "failed"
		if total == 0 || success == total {
			status = "completed"
		} else if success+partial > 0 {
			status = "partial"
		}
		if input.AckThroughSeq != nil {
			if *input.AckThroughSeq != run.ThroughSeq || *input.AckThroughSeq < run.AfterSeq {
				return nil, Invalid("ack_through_seq must equal this run's captured through_seq")
			}
			if status == "failed" {
				return nil, Invalid("a failed run cannot acknowledge changes")
			}
			_, err = tx.ExecContext(ctx, `UPDATE settings SET value=? WHERE key='acknowledged_event_seq'`, fmt.Sprint(*input.AckThroughSeq))
			if err != nil {
				return nil, err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE runs SET status=?,ended_at=?,summary=? WHERE id=?`, status, now.UnixMilli(), input.Summary, id)
		if err != nil {
			return nil, err
		}
		return getRun(ctx, tx, id)
	})
}

type Event struct {
	Seq        int64           `json:"seq"`
	OccurredAt time.Time       `json:"occurred_at"`
	Actor      string          `json:"actor"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	ChangeType string          `json:"change_type"`
	Payload    json.RawMessage `json:"payload"`
}

func (a *App) Changes(ctx context.Context, after, through int64) ([]Event, error) {
	if after < 0 || through < after {
		return nil, Invalid("expected 0 <= after_seq <= through_seq")
	}
	rows, err := a.Store.DB.QueryContext(ctx, `SELECT seq,occurred_at,actor,entity_type,entity_id,change_type,payload FROM events WHERE seq>? AND seq<=? AND (actor='user' OR entity_type='proposal') ORDER BY seq`, after, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Event{}
	for rows.Next() {
		var item Event
		var occurred int64
		var raw string
		if err = rows.Scan(&item.Seq, &occurred, &item.Actor, &item.EntityType, &item.EntityID, &item.ChangeType, &raw); err != nil {
			return nil, err
		}
		item.OccurredAt = time.UnixMilli(occurred).UTC()
		item.Payload = json.RawMessage(raw)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) Brief(ctx context.Context) (map[string]any, error) {
	tx, err := a.Store.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := a.Now().UTC()
	selected, more, err := selectWatches(ctx, tx, StartRun{}, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	var afterText string
	if err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='acknowledged_event_seq'`).Scan(&afterText); err != nil {
		return nil, err
	}
	after, _ := strconv.ParseInt(afterText, 10, 64)
	var through int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM events`).Scan(&through); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	items, err := a.Items(ctx, ItemFilters{View: "attention"})
	if err != nil {
		return nil, err
	}
	attention := make([]AttentionItem, len(items))
	for index, item := range items {
		attention[index] = AttentionItem{ID: item.ID, Kind: item.Kind, InterestID: item.InterestID, WatchID: item.WatchID, ParentID: item.ParentID, Title: item.Title, Summary: item.Summary, ContentVersion: item.ContentVersion, TodoState: item.TodoState, RemindAt: item.RemindAt, AcknowledgedContentVersion: item.AcknowledgedContentVersion, StateVersion: item.StateVersion}
	}
	health, err := a.OperationalHealth(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"now": now, "watches": selected, "more_due_count": more, "changes": map[string]int64{"after_seq": after, "through_seq": through}, "attention_items": attention, "health": health}, nil
}

type WatchHealth struct {
	WatchID       string     `json:"watch_id"`
	LastStatus    string     `json:"last_status,omitempty"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}
type LastRunHealth struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Summary   string     `json:"summary,omitempty"`
}

func (a *App) OperationalHealth(ctx context.Context) (map[string]any, error) {
	var lastRun any
	run, err := scanRun(a.Store.DB.QueryRowContext(ctx, "SELECT "+runColumns+" FROM runs ORDER BY started_at DESC,id DESC LIMIT 1"))
	if err == nil {
		lastRun = LastRunHealth{ID: run.ID, Status: run.Status, StartedAt: run.StartedAt, EndedAt: run.EndedAt, Summary: run.Summary}
	} else {
		var problem *Error
		if !errors.As(err, &problem) || problem.Status != 404 {
			return nil, err
		}
	}
	rows, err := a.Store.DB.QueryContext(ctx, `SELECT w.id,
  (SELECT status FROM watch_results wr WHERE wr.watch_id=w.id ORDER BY recorded_at DESC,run_id DESC LIMIT 1),
  (SELECT recorded_at FROM watch_results wr WHERE wr.watch_id=w.id ORDER BY recorded_at DESC,run_id DESC LIMIT 1),
  (SELECT MAX(recorded_at) FROM watch_results wr WHERE wr.watch_id=w.id AND status='success'),
  (SELECT json_extract(result,'$.error') FROM watch_results wr WHERE wr.watch_id=w.id ORDER BY recorded_at DESC,run_id DESC LIMIT 1)
FROM watches w WHERE w.state='active' ORDER BY w.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	watches := []WatchHealth{}
	for rows.Next() {
		var item WatchHealth
		var status, message sql.NullString
		var attempt, success sql.NullInt64
		if err = rows.Scan(&item.WatchID, &status, &attempt, &success, &message); err != nil {
			return nil, err
		}
		item.LastStatus, item.LastError = status.String, message.String
		if attempt.Valid {
			value := time.UnixMilli(attempt.Int64).UTC()
			item.LastAttemptAt = &value
		}
		if success.Valid {
			value := time.UnixMilli(success.Int64).UTC()
			item.LastSuccessAt = &value
		}
		watches = append(watches, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"last_run": lastRun, "watches": watches}, nil
}
