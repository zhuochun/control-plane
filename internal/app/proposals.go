package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
)

type Proposal struct {
	ID               string          `json:"id"`
	ProposalKey      string          `json:"proposal_key"`
	TargetType       string          `json:"target_type"`
	TargetID         *string         `json:"target_id"`
	ExpectedRevision *int64          `json:"expected_revision"`
	Operation        string          `json:"operation"`
	Payload          json.RawMessage `json:"payload"`
	RationaleMD      string          `json:"rationale_md"`
	State            string          `json:"state"`
}
type CreateProposal struct {
	RequestID        string          `json:"request_id"`
	ProposalKey      string          `json:"proposal_key"`
	TargetType       string          `json:"target_type"`
	TargetID         *string         `json:"target_id,omitempty"`
	ExpectedRevision *int64          `json:"expected_revision,omitempty"`
	Operation        string          `json:"operation"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	RationaleMD      string          `json:"rationale_md"`
}
type ResolveProposal struct {
	RequestID  string `json:"request_id"`
	Resolution string `json:"resolution"`
}
type interestCreatePayload struct {
	Title          string `json:"title"`
	InstructionsMD string `json:"instructions_md"`
	State          string `json:"state,omitempty"`
}
type interestUpdatePayload struct {
	Title          Field[string] `json:"title"`
	InstructionsMD Field[string] `json:"instructions_md"`
	State          Field[string] `json:"state"`
}
type watchCreatePayload struct {
	InterestID      string        `json:"interest_id"`
	Source          WatchSource   `json:"source"`
	InstructionsMD  string        `json:"instructions_md"`
	IntervalSeconds Field[int64]  `json:"interval_seconds"`
	LookbackSeconds Field[int64]  `json:"lookback_seconds"`
	State           Field[string] `json:"state"`
}
type watchUpdatePayload struct {
	Source          Field[WatchSource] `json:"source"`
	InstructionsMD  Field[string]      `json:"instructions_md"`
	IntervalSeconds Field[int64]       `json:"interval_seconds"`
	LookbackSeconds Field[int64]       `json:"lookback_seconds"`
	State           Field[string]      `json:"state"`
}

const proposalColumns = "id,proposal_key,target_type,target_id,expected_revision,operation,payload,rationale_md,state"

func scanProposal(row scanner) (Proposal, error) {
	var item Proposal
	var target, payload sql.NullString
	var revision sql.NullInt64
	err := row.Scan(&item.ID, &item.ProposalKey, &item.TargetType, &target, &revision, &item.Operation, &payload, &item.RationaleMD, &item.State)
	if err != nil {
		return item, missing(err)
	}
	if target.Valid {
		item.TargetID = &target.String
	}
	if revision.Valid {
		item.ExpectedRevision = &revision.Int64
	}
	if payload.Valid {
		item.Payload = json.RawMessage(payload.String)
	}
	return item, nil
}
func getProposal(ctx context.Context, db querier, id string) (Proposal, error) {
	return scanProposal(db.QueryRowContext(ctx, "SELECT "+proposalColumns+" FROM proposals WHERE id=?", id))
}
func (a *App) Proposal(ctx context.Context, id string) (Proposal, error) {
	return getProposal(ctx, a.Store.DB, id)
}
func (a *App) Proposals(ctx context.Context, state string) ([]Proposal, error) {
	query := "SELECT " + proposalColumns + " FROM proposals"
	args := []any{}
	if state != "" {
		query += " WHERE state=?"
		args = append(args, state)
	}
	query += " ORDER BY rowid,id"
	rows, err := a.Store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Proposal{}
	for rows.Next() {
		item, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func strictPayload(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return Invalid("payload is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return Invalid("invalid proposal payload: " + err.Error())
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Invalid("proposal payload must contain one object")
	}
	return nil
}
func validateProposal(input CreateProposal) error {
	if input.ProposalKey == "" || strings.TrimSpace(input.RationaleMD) == "" {
		return Invalid("proposal_key and rationale_md are required")
	}
	if input.TargetType != "interest" && input.TargetType != "watch" {
		return Invalid("target_type must be interest or watch")
	}
	if input.Operation != "create" && input.Operation != "update" && input.Operation != "deprecate" {
		return Invalid("operation must be create, update, or deprecate")
	}
	if input.Operation == "create" {
		if input.TargetID != nil || input.ExpectedRevision != nil {
			return Invalid("create proposals do not name an existing target")
		}
	} else if input.TargetID == nil || input.ExpectedRevision == nil {
		return Invalid("update and deprecate proposals need target_id and expected_revision")
	}
	if input.Operation == "deprecate" {
		if len(input.Payload) > 0 && !rawJSONEqual(input.Payload, nil) {
			return Invalid("deprecate proposals do not contain payload")
		}
		return nil
	}
	if input.TargetType == "interest" {
		if input.Operation == "create" {
			var value interestCreatePayload
			return strictPayload(input.Payload, &value)
		}
		var value interestUpdatePayload
		return strictPayload(input.Payload, &value)
	}
	if input.Operation == "create" {
		var value watchCreatePayload
		return strictPayload(input.Payload, &value)
	}
	var value watchUpdatePayload
	return strictPayload(input.Payload, &value)
}

func sameString(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
func sameInt64(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func (a *App) CreateProposal(ctx context.Context, input CreateProposal) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /proposals", input, func(tx *sql.Tx) (any, error) {
		if err := validateProposal(input); err != nil {
			return nil, err
		}
		var existing Proposal
		existing, err := scanProposal(tx.QueryRowContext(ctx, "SELECT "+proposalColumns+" FROM proposals WHERE proposal_key=?", input.ProposalKey))
		if err == nil {
			same := existing.TargetType == input.TargetType && existing.Operation == input.Operation && sameString(existing.TargetID, input.TargetID) && sameInt64(existing.ExpectedRevision, input.ExpectedRevision) && existing.RationaleMD == input.RationaleMD && rawJSONEqual(existing.Payload, input.Payload)
			if same {
				return existing, nil
			}
			return nil, &Error{Status: 409, Code: "proposal_key_conflict", Message: "proposal_key already describes a different proposal"}
		}
		var problem *Error
		if !errors.As(err, &problem) || problem.Status != 404 {
			return nil, err
		}
		id := uuid.NewString()
		var payload any
		if len(input.Payload) > 0 {
			payload = string(input.Payload)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO proposals(id,proposal_key,target_type,target_id,expected_revision,operation,payload,rationale_md,state) VALUES(?,?,?,?,?,?,?,?,'pending')`, id, input.ProposalKey, input.TargetType, input.TargetID, input.ExpectedRevision, input.Operation, payload, input.RationaleMD)
		if err != nil {
			return nil, err
		}
		item, err := getProposal(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return item, a.event(ctx, tx, "agent", "proposal", id, "proposal.created", map[string]any{"proposal_key": input.ProposalKey, "rationale_md": input.RationaleMD})
	})
}

func staleProposal(current int64) error {
	return &Error{Status: 409, Code: "proposal_stale", Message: "The proposed target changed before this proposal was accepted.", Details: map[string]any{"current_revision": current}}
}
func (a *App) applyProposal(ctx context.Context, tx *sql.Tx, p Proposal) error {
	now := a.Now().UTC().UnixMilli()
	if p.Operation == "create" && p.TargetType == "interest" {
		var value interestCreatePayload
		if err := strictPayload(p.Payload, &value); err != nil {
			return err
		}
		if value.State == "" {
			value.State = "active"
		}
		if strings.TrimSpace(value.Title) == "" || !validState(value.State) {
			return Invalid("proposal contains invalid Interest configuration")
		}
		id := uuid.NewString()
		_, err := tx.ExecContext(ctx, `INSERT INTO interests(id,title,instructions_md,state,revision,created_at,updated_at) VALUES(?,?,?,?,1,?,?)`, id, value.Title, value.InstructionsMD, value.State, now, now)
		if err != nil {
			return err
		}
		return a.event(ctx, tx, "user", "interest", id, "interest.created", map[string]any{"proposal_id": p.ID})
	}
	if p.Operation == "create" && p.TargetType == "watch" {
		var value watchCreatePayload
		if err := strictPayload(p.Payload, &value); err != nil {
			return err
		}
		if _, err := getInterest(ctx, tx, value.InterestID); err != nil {
			return err
		}
		item := Watch{ID: uuid.NewString(), InterestID: value.InterestID, Source: value.Source, InstructionsMD: value.InstructionsMD, IntervalSeconds: 7200, LookbackSeconds: 604800, State: "active"}
		if value.IntervalSeconds.Set {
			item.IntervalSeconds = value.IntervalSeconds.Value
		}
		if value.LookbackSeconds.Set {
			item.LookbackSeconds = value.LookbackSeconds.Value
		}
		if value.State.Set {
			item.State = value.State.Value
		}
		if err := validateWatch(item); err != nil {
			return err
		}
		source, _ := json.Marshal(item.Source)
		_, err := tx.ExecContext(ctx, `INSERT INTO watches(id,interest_id,source,instructions_md,interval_seconds,lookback_seconds,state,revision,next_due_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,1,?,?,?)`, item.ID, item.InterestID, string(source), item.InstructionsMD, item.IntervalSeconds, item.LookbackSeconds, item.State, now, now, now)
		if err != nil {
			return err
		}
		return a.event(ctx, tx, "user", "watch", item.ID, "watch.created", map[string]any{"proposal_id": p.ID})
	}
	if p.TargetID == nil || p.ExpectedRevision == nil {
		return Invalid("proposal target is missing")
	}
	if p.TargetType == "interest" {
		item, err := getInterest(ctx, tx, *p.TargetID)
		if err != nil {
			return err
		}
		if item.Revision != *p.ExpectedRevision {
			return staleProposal(item.Revision)
		}
		previous := item
		if p.Operation == "deprecate" {
			item.State = "deprecated"
		} else {
			var value interestUpdatePayload
			if err = strictPayload(p.Payload, &value); err != nil {
				return err
			}
			if value.Title.Set {
				item.Title = value.Title.Value
			}
			if value.InstructionsMD.Set {
				item.InstructionsMD = value.InstructionsMD.Value
			}
			if value.State.Set {
				item.State = value.State.Value
			}
		}
		if strings.TrimSpace(item.Title) == "" || !validState(item.State) {
			return Invalid("proposal contains invalid Interest configuration")
		}
		_, err = tx.ExecContext(ctx, `UPDATE interests SET title=?,instructions_md=?,state=?,revision=revision+1,updated_at=? WHERE id=?`, item.Title, item.InstructionsMD, item.State, now, item.ID)
		if err != nil {
			return err
		}
		if previous.InstructionsMD != item.InstructionsMD || (previous.State != "active" && item.State == "active") {
			if _, err = tx.ExecContext(ctx, `UPDATE watches SET next_due_at=? WHERE interest_id=? AND state='active'`, now, item.ID); err != nil {
				return err
			}
		}
		return a.event(ctx, tx, "user", "interest", item.ID, "interest.updated", map[string]any{"proposal_id": p.ID})
	}
	item, err := getWatch(ctx, tx, *p.TargetID)
	if err != nil {
		return err
	}
	if item.Revision != *p.ExpectedRevision {
		return staleProposal(item.Revision)
	}
	if p.Operation == "deprecate" {
		item.State = "deprecated"
	} else {
		var value watchUpdatePayload
		if err = strictPayload(p.Payload, &value); err != nil {
			return err
		}
		if value.Source.Set {
			if value.Source.Value != item.Source {
				item.Cursor = nil
			}
			item.Source = value.Source.Value
		}
		if value.InstructionsMD.Set {
			item.InstructionsMD = value.InstructionsMD.Value
		}
		if value.IntervalSeconds.Set {
			item.IntervalSeconds = value.IntervalSeconds.Value
		}
		if value.LookbackSeconds.Set {
			item.LookbackSeconds = value.LookbackSeconds.Value
		}
		if value.State.Set {
			item.State = value.State.Value
		}
	}
	if err = validateWatch(item); err != nil {
		return err
	}
	source, _ := json.Marshal(item.Source)
	var cursor any
	if item.Cursor != nil {
		cursor = string(item.Cursor)
	}
	_, err = tx.ExecContext(ctx, `UPDATE watches SET source=?,instructions_md=?,interval_seconds=?,lookback_seconds=?,state=?,revision=revision+1,cursor=?,next_due_at=?,updated_at=? WHERE id=?`, string(source), item.InstructionsMD, item.IntervalSeconds, item.LookbackSeconds, item.State, cursor, now, now, item.ID)
	if err != nil {
		return err
	}
	return a.event(ctx, tx, "user", "watch", item.ID, "watch.updated", map[string]any{"proposal_id": p.ID})
}

func (a *App) ResolveProposal(ctx context.Context, id string, input ResolveProposal) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /proposals/"+id+"/resolve", input, func(tx *sql.Tx) (any, error) {
		if input.Resolution != "accepted" && input.Resolution != "rejected" {
			return nil, Invalid("resolution must be accepted or rejected")
		}
		item, err := getProposal(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if item.State != "pending" {
			if item.State == input.Resolution {
				return item, nil
			}
			return nil, &Error{Status: 409, Code: "proposal_resolved", Message: "Proposal already has a different resolution"}
		}
		if input.Resolution == "accepted" {
			if err = a.applyProposal(ctx, tx, item); err != nil {
				return nil, err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE proposals SET state=? WHERE id=?`, input.Resolution, id)
		if err != nil {
			return nil, err
		}
		item, err = getProposal(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return item, a.event(ctx, tx, "user", "proposal", id, "proposal."+input.Resolution, map[string]any{"proposal_key": item.ProposalKey})
	})
}
