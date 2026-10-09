package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// UserInput retains the owner's exact submission independently of agent content.
type UserInput struct {
	ID           string          `json:"id"`
	ItemID       string          `json:"item_id"`
	Kind         string          `json:"kind"`
	Text         string          `json:"text"`
	Original     json.RawMessage `json:"original"`
	SubmittedAt  time.Time       `json:"submitted_at"`
	Status       string          `json:"status"`
	Archived     bool            `json:"archived"`
	Attempts     []InputAttempt  `json:"attempts,omitempty"`
	AttemptCount int             `json:"attempt_count"`
}

type InputAttempt struct {
	ID         string    `json:"id"`
	RunID      string    `json:"run_id"`
	RecordedAt time.Time `json:"recorded_at"`
	Outcome    string    `json:"outcome"`
	ResultMD   string    `json:"result_md"`
	References []string  `json:"references"`
}

type ProcessItemInput struct {
	RequestID              string            `json:"request_id,omitempty"`
	RunID                  string            `json:"run_id"`
	InputID                string            `json:"input_id"`
	ExpectedContentVersion int64             `json:"expected_content_version"`
	ExpectedStateVersion   int64             `json:"expected_state_version"`
	Outcome                string            `json:"outcome"`
	ResultMD               string            `json:"result_md"`
	References             []string          `json:"references,omitempty"`
	Archive                bool              `json:"archive,omitempty"`
	Report                 *Report           `json:"report,omitempty"`
	ContextMD              *string           `json:"context_md,omitempty"`
	Interests              *[]ItemInterest   `json:"interests,omitempty"`
	Delegations            []DelegationPatch `json:"delegations,omitempty"`
}

const inputColumns = `id,item_id,kind,text,original,submitted_at,status,archived`

func scanInput(row scanner) (UserInput, error) {
	var input UserInput
	var original string
	var submitted int64
	err := row.Scan(&input.ID, &input.ItemID, &input.Kind, &input.Text, &original, &submitted, &input.Status, &input.Archived)
	input.Original = json.RawMessage(original)
	input.SubmittedAt = time.UnixMilli(submitted).UTC()
	return input, missing(err)
}

func readInputs(ctx context.Context, db querier, where string, args ...any) ([]UserInput, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+inputColumns+" FROM item_inputs "+where, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []UserInput{}
	for rows.Next() {
		input, scanErr := scanInput(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, input)
	}
	return items, rows.Err()
}

func insertInput(ctx context.Context, tx *sql.Tx, item Item, kind, text string, now int64) error {
	original, err := json.Marshal(map[string]any{"title": item.Title, "sources": item.Sources})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO item_inputs(id,item_id,kind,text,original,submitted_at,status) VALUES(?,?,?,?,?,?,'pending')`, uuid.NewString(), item.ID, kind, text, string(original), now)
	return err
}

func saveNoteInput(ctx context.Context, tx *sql.Tx, item Item, text string, now int64) error {
	if text == item.UserNote {
		return nil
	}
	status := "superseded"
	if strings.TrimSpace(text) == "" {
		status = "withdrawn"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE item_inputs SET status=? WHERE item_id=? AND kind='note' AND status='pending'`, status, item.ID); err != nil {
		return err
	}
	if strings.TrimSpace(text) != "" {
		return insertInput(ctx, tx, item, "note", text, now)
	}
	return nil
}

func inputAttempts(ctx context.Context, db querier, id string, offset, limit int, latest bool) ([]InputAttempt, error) {
	order := "rowid"
	if latest {
		order = "rowid DESC"
	}
	rows, err := db.QueryContext(ctx, `SELECT id,run_id,recorded_at,outcome,result_md,references_json FROM input_attempts WHERE input_id=? ORDER BY `+order+` LIMIT ? OFFSET ?`, id, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []InputAttempt{}
	for rows.Next() {
		var attempt InputAttempt
		var recorded int64
		var refs string
		if err = rows.Scan(&attempt.ID, &attempt.RunID, &recorded, &attempt.Outcome, &attempt.ResultMD, &refs); err != nil {
			return nil, err
		}
		attempt.RecordedAt = time.UnixMilli(recorded).UTC()
		if err = json.Unmarshal([]byte(refs), &attempt.References); err != nil {
			return nil, err
		}
		items = append(items, attempt)
	}
	return items, rows.Err()
}

func completeAttemptSummary(ctx context.Context, db querier, input *UserInput) error {
	var err error
	input.Attempts, err = inputAttempts(ctx, db, input.ID, 0, 1, true)
	if err != nil {
		return err
	}
	return db.QueryRowContext(ctx, "SELECT count(*) FROM input_attempts WHERE input_id=?", input.ID).Scan(&input.AttemptCount)
}

func (a *App) InputAttempts(ctx context.Context, id, inputID string, offset int) (map[string]any, error) {
	if offset < 0 {
		return nil, Invalid("offset must be nonnegative")
	}
	item, err := getItem(ctx, a.Store.DB, id)
	if err != nil {
		return nil, err
	}
	entry, err := scanInput(a.Store.DB.QueryRowContext(ctx, "SELECT "+inputColumns+" FROM item_inputs WHERE id=? AND item_id=?", inputID, item.ID))
	if err != nil {
		return nil, err
	}
	attempts, err := inputAttempts(ctx, a.Store.DB, entry.ID, offset, 21, false)
	if err != nil {
		return nil, err
	}
	more := len(attempts) > 20
	if more {
		attempts = attempts[:20]
	}
	result := map[string]any{"items": attempts}
	if more {
		result["next_offset"] = offset + len(attempts)
	}
	return result, nil
}

func (a *App) InputHistory(ctx context.Context, id string, offset int) (map[string]any, error) {
	item, err := getItem(ctx, a.Store.DB, id)
	if err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, Invalid("offset must be nonnegative")
	}
	inputs, err := readInputs(ctx, a.Store.DB, "WHERE item_id=? ORDER BY submitted_at,id LIMIT 51 OFFSET ?", item.ID, offset)
	if err != nil {
		return nil, err
	}
	more := len(inputs) > 50
	if more {
		inputs = inputs[:50]
	}
	for i := range inputs {
		err = completeAttemptSummary(ctx, a.Store.DB, &inputs[i])
		if err != nil {
			return nil, err
		}
	}
	result := map[string]any{"items": inputs}
	if more {
		result["next_offset"] = offset + len(inputs)
	}
	return result, nil
}

func completeInputState(ctx context.Context, db querier, item Item) (Item, error) {
	var err error
	item.PendingInputs, err = readInputs(ctx, db, "WHERE item_id=? AND status='pending' ORDER BY submitted_at,id", item.ID)
	if err != nil {
		return item, err
	}
	for i := range item.PendingInputs {
		err = completeAttemptSummary(ctx, db, &item.PendingInputs[i])
		if err != nil {
			return item, err
		}
	}
	err = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM item_inputs WHERE item_id=? AND archived=1)`, item.ID).Scan(&item.InboxArchived)
	return item, err
}

func inputConflict(message string) error {
	return &Error{Status: 409, Code: "input_conflict", Message: message, Retryable: true}
}

func (a *App) ProcessItemInput(ctx context.Context, id string, input ProcessItemInput) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /items/"+id+"/inputs/process", input, func(tx *sql.Tx) (any, error) {
		run, err := getRun(ctx, tx, input.RunID)
		if err != nil {
			return nil, err
		}
		if err = liveRun(run); err != nil {
			return nil, err
		}
		snapshot, err := loadRunContext(ctx, tx, run.ID)
		if err != nil {
			return nil, err
		}
		captured := false
		for _, entry := range snapshot.UserInputs {
			if entry.ID == input.InputID {
				captured = true
				break
			}
		}
		if !captured {
			return nil, inputConflict("Input was not captured by this Run; read its user_inputs pages")
		}
		entry, err := scanInput(tx.QueryRowContext(ctx, "SELECT "+inputColumns+" FROM item_inputs WHERE id=?", input.InputID))
		if err != nil {
			return nil, err
		}
		item, err := getItem(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if entry.ItemID != item.ID || entry.Status != "pending" {
			return nil, inputConflict("Input no longer pending on this Item; reread and reconcile")
		}
		if input.ExpectedStateVersion != item.StateVersion {
			return nil, stateConflict(item)
		}
		if input.ExpectedContentVersion != item.ContentVersion {
			return nil, &Error{Status: 409, Code: "content_conflict", Message: "Item changed; reread before processing", Retryable: true}
		}
		success := input.Outcome == "responded" || input.Outcome == "incorporated" || input.Outcome == "follow_up"
		if !success && input.Outcome != "blocked" && input.Outcome != "failed" {
			return nil, Invalid("outcome must be responded, incorporated, follow_up, blocked, or failed")
		}
		if strings.TrimSpace(input.ResultMD) == "" || len(input.ResultMD) > 64<<10 {
			return nil, Invalid("result_md must contain 1 to 65536 bytes explaining the result and next step")
		}
		if input.Archive && (!success || entry.Kind != "inbox") {
			return nil, Invalid("only successfully handled inbox input may be archived")
		}
		if input.Interests != nil && entry.Kind != "inbox" {
			return nil, Invalid("relevance changes are limited to inbox processing")
		}
		if input.Interests != nil {
			for _, interest := range *input.Interests {
				if strings.TrimSpace(interest.Reason) == "" {
					return nil, Invalid("processing relevance needs a reason for each Interest")
				}
			}
		}
		if len(input.References) > 50 {
			return nil, Invalid("at most 50 result references are allowed")
		}
		for _, ref := range input.References {
			if strings.TrimSpace(ref) == "" || len(ref) > 2000 {
				return nil, Invalid("result references must contain 1 to 2000 bytes")
			}
		}
		body := PutItem{ExpectedContentVersion: item.ContentVersion, DedupeKey: item.DedupeKey, Kind: item.Kind, Title: item.Title, Summary: item.Summary, WatchID: item.WatchID, ParentID: item.ParentID, Interests: item.Interests, Sources: item.Sources, ContextMD: item.ContextMD, Report: item.Report, Delegations: input.Delegations, workUpdate: true}
		if input.Report != nil {
			body.Report = *input.Report
		}
		if input.ContextMD != nil {
			body.ContextMD = *input.ContextMD
		}
		if input.Outcome == "incorporated" {
			body.ContextMD += "\n\n" + input.ResultMD
		}
		if input.Interests != nil {
			body.Interests = *input.Interests
		}
		// The result is visible even when a caller supplied no replacement report.
		body.Report.BodyMD += "\n\n### User input: " + input.Outcome + "\n\n" + input.ResultMD
		if _, err = a.putItemTx(ctx, tx, item.ID, body, item.Origin); err != nil {
			return nil, err
		}
		if input.Outcome == "follow_up" && len(input.References) == 0 {
			updated, readErr := getItem(ctx, tx, item.ID)
			if readErr != nil {
				return nil, readErr
			}
			open := false
			for _, delegation := range updated.Delegations {
				if delegation.Status == "pending" || delegation.Status == "blocked" {
					open = true
					break
				}
			}
			if !open {
				return nil, Invalid("follow_up needs a durable reference or an open Item-local delegation")
			}
		}
		now := a.Now().UTC().UnixMilli()
		refs, err := json.Marshal(input.References)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO input_attempts(id,input_id,run_id,recorded_at,outcome,result_md,references_json) VALUES(?,?,?,?,?,?,?)`, uuid.NewString(), entry.ID, run.ID, now, input.Outcome, input.ResultMD, string(refs)); err != nil {
			return nil, err
		}
		if success {
			if _, err = tx.ExecContext(ctx, `UPDATE item_inputs SET status='processed',archived=? WHERE id=?`, input.Archive, entry.ID); err != nil {
				return nil, err
			}
			if entry.Kind == "note" {
				if _, err = tx.ExecContext(ctx, `UPDATE items SET user_note='',state_version=state_version+1,state_updated_at=? WHERE id=?`, now, item.ID); err != nil {
					return nil, err
				}
			}
		}
		if err = a.event(ctx, tx, "agent", "item", item.ID, "item.input_processed", map[string]any{"input_id": entry.ID, "outcome": input.Outcome, "run_id": run.ID}); err != nil {
			return nil, err
		}
		item, err = getItem(ctx, tx, item.ID)
		if err != nil {
			return nil, err
		}
		return completeItem(ctx, tx, item)
	})
}

// inputCompletion counts effective dispositions; historical failed attempts do
// not downgrade a recovered success. Old Run snapshots have no new obligation.
func inputCompletion(ctx context.Context, db querier, snapshot runContextSnapshot, runID string) (string, map[string]int, error) {
	counts := map[string]int{"processed": 0, "superseded": 0, "withdrawn": 0, "unresolved": 0, "attempts": 0}
	missingInputs := []string{}
	for _, captured := range snapshot.UserInputs {
		var status string
		if err := db.QueryRowContext(ctx, "SELECT status FROM item_inputs WHERE id=?", captured.ID).Scan(&status); err != nil {
			return "", nil, err
		}
		switch status {
		case "processed", "superseded", "withdrawn":
			counts[status]++
		default:
			var count int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM input_attempts WHERE input_id=? AND run_id=?", captured.ID, runID).Scan(&count); err != nil {
				return "", nil, err
			}
			if count == 0 {
				missingInputs = append(missingInputs, captured.ID)
			} else {
				counts["unresolved"]++
			}
		}
	}
	if len(missingInputs) > 0 {
		return "", nil, &Error{Status: 409, Code: "input_handling_missing", Message: "Every captured input needs a visible handling result or failed attempt before Finish", Retryable: true, Details: map[string]any{"input_ids": missingInputs, "read": "get_brief Run user_inputs cursor", "process": "process_item_input"}}
	}
	var countsAttemptValue int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM input_attempts WHERE run_id=?", runID).Scan(&countsAttemptValue); err != nil {
		return "", nil, err
	}
	counts["attempts"] = countsAttemptValue
	if counts["unresolved"] == 0 {
		return "completed", counts, nil
	}
	if counts["processed"]+counts["superseded"]+counts["withdrawn"] > 0 {
		return "partial", counts, nil
	}
	return "failed", counts, nil
}

func pendingInputCount(ctx context.Context, db querier) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM item_inputs WHERE status='pending'").Scan(&count)
	return count, err
}
