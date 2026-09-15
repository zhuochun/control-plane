package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

type WatchSource struct {
	Kind    string `json:"kind"`
	Locator string `json:"locator"`
}

type Watch struct {
	ID              string          `json:"id"`
	InterestID      string          `json:"interest_id"`
	Source          WatchSource     `json:"source"`
	InstructionsMD  string          `json:"instructions_md"`
	IntervalSeconds int64           `json:"interval_seconds"`
	LookbackSeconds int64           `json:"lookback_seconds"`
	State           string          `json:"state"`
	Revision        int64           `json:"revision"`
	Cursor          json.RawMessage `json:"cursor"`
	NextDueAt       time.Time       `json:"next_due_at"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type CreateWatch struct {
	RequestID       string        `json:"request_id"`
	InterestID      string        `json:"interest_id"`
	Source          WatchSource   `json:"source"`
	InstructionsMD  string        `json:"instructions_md"`
	IntervalSeconds Field[int64]  `json:"interval_seconds"`
	LookbackSeconds Field[int64]  `json:"lookback_seconds"`
	State           Field[string] `json:"state"`
}

type UpdateWatch struct {
	RequestID        string             `json:"request_id"`
	ExpectedRevision int64              `json:"expected_revision"`
	Source           Field[WatchSource] `json:"source"`
	InstructionsMD   Field[string]      `json:"instructions_md"`
	IntervalSeconds  Field[int64]       `json:"interval_seconds"`
	LookbackSeconds  Field[int64]       `json:"lookback_seconds"`
	State            Field[string]      `json:"state"`
}

const watchColumns = "id,interest_id,source,instructions_md,interval_seconds,lookback_seconds,state,revision,cursor,next_due_at,created_at,updated_at"

func scanWatch(row scanner) (Watch, error) {
	var item Watch
	var source string
	var cursor sql.NullString
	var due, created, updated int64
	err := row.Scan(&item.ID, &item.InterestID, &source, &item.InstructionsMD, &item.IntervalSeconds, &item.LookbackSeconds, &item.State, &item.Revision, &cursor, &due, &created, &updated)
	if err != nil {
		return item, missing(err)
	}
	if err = json.Unmarshal([]byte(source), &item.Source); err != nil {
		return item, err
	}
	if cursor.Valid {
		item.Cursor = json.RawMessage(cursor.String)
	}
	item.NextDueAt, item.CreatedAt, item.UpdatedAt = time.UnixMilli(due).UTC(), time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()
	return item, nil
}

func getWatch(ctx context.Context, db querier, id string) (Watch, error) {
	return scanWatch(db.QueryRowContext(ctx, "SELECT "+watchColumns+" FROM watches WHERE id=?", id))
}

func (a *App) Watch(ctx context.Context, id string) (Watch, error) {
	return getWatch(ctx, a.Store.DB, id)
}

func (a *App) Watches(ctx context.Context, interestID string) ([]Watch, error) {
	query := "SELECT " + watchColumns + " FROM watches"
	args := []any{}
	if interestID != "" {
		query += " WHERE interest_id=?"
		args = append(args, interestID)
	}
	rows, err := a.Store.DB.QueryContext(ctx, query+" ORDER BY created_at,id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Watch{}
	for rows.Next() {
		item, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func validateWatch(item Watch) error {
	if strings.TrimSpace(item.Source.Kind) == "" || strings.TrimSpace(item.Source.Locator) == "" {
		return Invalid("source.kind and source.locator are required")
	}
	if item.IntervalSeconds < 1 || item.IntervalSeconds > 31536000 {
		return Invalid("interval_seconds must be between 1 and 31536000")
	}
	if item.LookbackSeconds < 1 {
		return Invalid("lookback_seconds must be positive")
	}
	if !validState(item.State) {
		return Invalid("state must be active, paused, or deprecated")
	}
	return nil
}

func (a *App) CreateWatch(ctx context.Context, input CreateWatch) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /watches", input, func(tx *sql.Tx) (any, error) {
		if _, err := getInterest(ctx, tx, input.InterestID); err != nil {
			return nil, err
		}
		item := Watch{ID: uuid.NewString(), InterestID: input.InterestID, Source: input.Source, InstructionsMD: input.InstructionsMD, IntervalSeconds: 7200, LookbackSeconds: 604800, State: "active"}
		if input.IntervalSeconds.Set {
			item.IntervalSeconds = input.IntervalSeconds.Value
		}
		if input.LookbackSeconds.Set {
			item.LookbackSeconds = input.LookbackSeconds.Value
		}
		if input.State.Set {
			item.State = input.State.Value
		}
		if err := validateWatch(item); err != nil {
			return nil, err
		}
		source, err := json.Marshal(item.Source)
		if err != nil {
			return nil, err
		}
		now := a.Now().UTC().UnixMilli()
		_, err = tx.ExecContext(ctx, `INSERT INTO watches(id,interest_id,source,instructions_md,interval_seconds,lookback_seconds,state,revision,next_due_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,1,?,?,?)`, item.ID, item.InterestID, string(source), item.InstructionsMD, item.IntervalSeconds, item.LookbackSeconds, item.State, now, now, now)
		if err != nil {
			return nil, err
		}
		item, err = getWatch(ctx, tx, item.ID)
		if err != nil {
			return nil, err
		}
		return item, a.event(ctx, tx, "user", "watch", item.ID, "watch.created", item)
	})
}

func (a *App) UpdateWatch(ctx context.Context, id string, input UpdateWatch) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "PATCH /watches/"+id, input, func(tx *sql.Tx) (any, error) {
		item, err := getWatch(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if input.ExpectedRevision != item.Revision {
			return nil, revisionConflict(item.Revision)
		}
		previous := item
		if input.Source.Set {
			item.Source = input.Source.Value
		}
		if input.InstructionsMD.Set {
			item.InstructionsMD = input.InstructionsMD.Value
		}
		if input.IntervalSeconds.Set {
			item.IntervalSeconds = input.IntervalSeconds.Value
		}
		if input.LookbackSeconds.Set {
			item.LookbackSeconds = input.LookbackSeconds.Value
		}
		if input.State.Set {
			item.State = input.State.Value
		}
		if err = validateWatch(item); err != nil {
			return nil, err
		}
		now := a.Now().UTC()
		if previous.Source != item.Source || previous.InstructionsMD != item.InstructionsMD || previous.IntervalSeconds != item.IntervalSeconds || previous.LookbackSeconds != item.LookbackSeconds || (previous.State != "active" && item.State == "active") {
			item.NextDueAt = now
		}
		if previous.Source != item.Source {
			item.Cursor = nil
		}
		source, err := json.Marshal(item.Source)
		if err != nil {
			return nil, err
		}
		var cursor any
		if item.Cursor != nil {
			cursor = string(item.Cursor)
		}
		_, err = tx.ExecContext(ctx, `UPDATE watches SET source=?,instructions_md=?,interval_seconds=?,lookback_seconds=?,state=?,revision=revision+1,cursor=?,next_due_at=?,updated_at=? WHERE id=?`, string(source), item.InstructionsMD, item.IntervalSeconds, item.LookbackSeconds, item.State, cursor, item.NextDueAt.UnixMilli(), now.UnixMilli(), id)
		if err != nil {
			return nil, err
		}
		item, err = getWatch(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return item, a.event(ctx, tx, "user", "watch", id, "watch.updated", item)
	})
}
