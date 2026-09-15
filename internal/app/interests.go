package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Field distinguishes omission from an explicit value in configuration patches.
// Configuration fields cannot be cleared with null.
type Field[T any] struct {
	Set   bool
	Value T
}

func (f *Field[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return errors.New("configuration fields cannot be null")
	}
	f.Set = true
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(&f.Value)
}

type Interest struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	InstructionsMD string    `json:"instructions_md"`
	State          string    `json:"state"`
	Revision       int64     `json:"revision"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CreateInterest struct {
	RequestID      string `json:"request_id"`
	Title          string `json:"title"`
	InstructionsMD string `json:"instructions_md"`
	State          string `json:"state,omitempty"`
}

type UpdateInterest struct {
	RequestID        string        `json:"request_id"`
	ExpectedRevision int64         `json:"expected_revision"`
	Title            Field[string] `json:"title"`
	InstructionsMD   Field[string] `json:"instructions_md"`
	State            Field[string] `json:"state"`
}

type scanner interface{ Scan(...any) error }
type querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func missing(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return &Error{Status: 404, Code: "not_found", Message: "Record not found"}
	}
	return err
}

func revisionConflict(current int64) error {
	return &Error{Status: 409, Code: "revision_conflict", Message: "This configuration changed. Read it again before saving.", Details: map[string]any{"current_revision": current}}
}

func validState(state string) bool {
	return state == "active" || state == "paused" || state == "deprecated"
}

const interestColumns = "id, title, instructions_md, state, revision, created_at, updated_at"

func scanInterest(row scanner) (Interest, error) {
	var item Interest
	var created, updated int64
	err := row.Scan(&item.ID, &item.Title, &item.InstructionsMD, &item.State, &item.Revision, &created, &updated)
	item.CreatedAt, item.UpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()
	return item, missing(err)
}

func getInterest(ctx context.Context, db querier, id string) (Interest, error) {
	return scanInterest(db.QueryRowContext(ctx, "SELECT "+interestColumns+" FROM interests WHERE id = ?", id))
}

func (a *App) Interest(ctx context.Context, id string) (Interest, error) {
	return getInterest(ctx, a.Store.DB, id)
}

func (a *App) Interests(ctx context.Context) ([]Interest, error) {
	rows, err := a.Store.DB.QueryContext(ctx, "SELECT "+interestColumns+" FROM interests ORDER BY created_at, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Interest{}
	for rows.Next() {
		item, err := scanInterest(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) CreateInterest(ctx context.Context, input CreateInterest) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /interests", input, func(tx *sql.Tx) (any, error) {
		if input.State == "" {
			input.State = "active"
		}
		if strings.TrimSpace(input.Title) == "" || !validState(input.State) {
			return nil, Invalid("A title and valid lifecycle state are required")
		}
		now := a.Now().UTC().UnixMilli()
		id := uuid.NewString()
		_, err := tx.ExecContext(ctx, `INSERT INTO interests(id,title,instructions_md,state,revision,created_at,updated_at) VALUES(?,?,?,?,1,?,?)`, id, input.Title, input.InstructionsMD, input.State, now, now)
		if err != nil {
			return nil, err
		}
		item, err := getInterest(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return item, a.event(ctx, tx, "user", "interest", id, "interest.created", item)
	})
}

func (a *App) UpdateInterest(ctx context.Context, id string, input UpdateInterest) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "PATCH /interests/"+id, input, func(tx *sql.Tx) (any, error) {
		item, err := getInterest(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if input.ExpectedRevision != item.Revision {
			return nil, revisionConflict(item.Revision)
		}
		previous := item
		if input.Title.Set {
			item.Title = input.Title.Value
		}
		if input.InstructionsMD.Set {
			item.InstructionsMD = input.InstructionsMD.Value
		}
		if input.State.Set {
			item.State = input.State.Value
		}
		if strings.TrimSpace(item.Title) == "" || !validState(item.State) {
			return nil, Invalid("A title and valid lifecycle state are required")
		}
		now := a.Now().UTC().UnixMilli()
		_, err = tx.ExecContext(ctx, `UPDATE interests SET title=?,instructions_md=?,state=?,revision=revision+1,updated_at=? WHERE id=?`, item.Title, item.InstructionsMD, item.State, now, id)
		if err != nil {
			return nil, err
		}
		if previous.InstructionsMD != item.InstructionsMD || (previous.State != "active" && item.State == "active") {
			if _, err = tx.ExecContext(ctx, `UPDATE watches SET next_due_at=? WHERE interest_id=? AND state='active'`, now, id); err != nil {
				return nil, err
			}
		}
		item, err = getInterest(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return item, a.event(ctx, tx, "user", "interest", id, "interest.updated", item)
	})
}
