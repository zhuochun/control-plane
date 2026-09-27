package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/url"
	"slices"
	"strings"
	"time"

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
	EvidenceLinks    []string        `json:"evidence_links"`
	Confidence       *float64        `json:"confidence,omitempty"`
	DuplicateOf      *string         `json:"duplicate_of,omitempty"`
	SnoozedUntil     *time.Time      `json:"snoozed_until,omitempty"`
	ExpiresAt        *time.Time      `json:"expires_at,omitempty"`
	MergedInto       *string         `json:"merged_into,omitempty"`
	CreatedID        string          `json:"-"`
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
	EvidenceLinks    []string        `json:"evidence_links,omitempty"`
	Confidence       *float64        `json:"confidence,omitempty"`
	DuplicateOf      *string         `json:"duplicate_of,omitempty"`
	ExpiresAt        *time.Time      `json:"expires_at,omitempty"`
}
type ResolveProposal struct {
	RequestID   string     `json:"request_id"`
	Resolution  string     `json:"resolution"`
	SnoozeUntil *time.Time `json:"snooze_until,omitempty"`
	MergeInto   string     `json:"merge_into,omitempty"`
}
type interestCreatePayload struct {
	Slug           string `json:"slug,omitempty"`
	Title          string `json:"title"`
	InstructionsMD string `json:"instructions_md"`
	State          string `json:"state,omitempty"`
}
type interestUpdatePayload struct {
	Slug           Field[string] `json:"slug"`
	Title          Field[string] `json:"title"`
	InstructionsMD Field[string] `json:"instructions_md"`
	State          Field[string] `json:"state"`
}
type watchCreatePayload struct {
	Slug            string        `json:"slug,omitempty"`
	InterestIDs     []string      `json:"interest_ids"`
	MatchingPolicy  string        `json:"matching_policy"`
	ValidUntil      *time.Time    `json:"valid_until,omitempty"`
	Source          WatchSource   `json:"source"`
	InstructionsMD  string        `json:"instructions_md"`
	IntervalSeconds Field[int64]  `json:"interval_seconds"`
	LookbackSeconds Field[int64]  `json:"lookback_seconds"`
	State           Field[string] `json:"state"`
}
type watchUpdatePayload struct {
	Slug            Field[string]      `json:"slug"`
	InterestIDs     Field[[]string]    `json:"interest_ids"`
	MatchingPolicy  Field[string]      `json:"matching_policy"`
	ValidUntil      Field[*time.Time]  `json:"valid_until"`
	ClearValidUntil bool               `json:"clear_valid_until,omitempty"`
	Source          Field[WatchSource] `json:"source"`
	InstructionsMD  Field[string]      `json:"instructions_md"`
	IntervalSeconds Field[int64]       `json:"interval_seconds"`
	LookbackSeconds Field[int64]       `json:"lookback_seconds"`
	State           Field[string]      `json:"state"`
}

const proposalColumns = "id,proposal_key,target_type,target_id,expected_revision,operation,payload,rationale_md,state,evidence_links,confidence,duplicate_of,snoozed_until,expires_at,merged_into"

func scanProposal(row scanner) (Proposal, error) {
	var item Proposal
	var target, payload, evidence, duplicateOf, mergedInto sql.NullString
	var revision, snoozedUntil, expiresAt sql.NullInt64
	var confidence sql.NullFloat64
	err := row.Scan(&item.ID, &item.ProposalKey, &item.TargetType, &target, &revision, &item.Operation, &payload, &item.RationaleMD, &item.State, &evidence, &confidence, &duplicateOf, &snoozedUntil, &expiresAt, &mergedInto)
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
	if err = json.Unmarshal([]byte(evidence.String), &item.EvidenceLinks); err != nil {
		return item, err
	}
	if confidence.Valid {
		item.Confidence = &confidence.Float64
	}
	if duplicateOf.Valid {
		item.DuplicateOf = &duplicateOf.String
	}
	if snoozedUntil.Valid {
		value := time.UnixMilli(snoozedUntil.Int64).UTC()
		item.SnoozedUntil = &value
	}
	if expiresAt.Valid {
		value := time.UnixMilli(expiresAt.Int64).UTC()
		item.ExpiresAt = &value
	}
	if mergedInto.Valid {
		item.MergedInto = &mergedInto.String
	}
	return item, nil
}
func getProposal(ctx context.Context, db querier, id string) (Proposal, error) {
	canonical, err := resolveID(ctx, db, "proposals", id)
	if err != nil {
		return Proposal{}, err
	}
	return scanProposal(db.QueryRowContext(ctx, "SELECT "+proposalColumns+" FROM proposals WHERE id=?", canonical))
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
	if state == "pending" {
		query += " AND (snoozed_until IS NULL OR snoozed_until<=?) AND (expires_at IS NULL OR expires_at>?)"
		now := a.Now().UTC().UnixMilli()
		args = append(args, now, now)
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
	if input.Confidence != nil && (math.IsNaN(*input.Confidence) || *input.Confidence < 0 || *input.Confidence > 1) {
		return Invalid("confidence must be between 0 and 1")
	}
	for _, link := range input.EvidenceLinks {
		parsed, err := url.Parse(link)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return Invalid("evidence_links must be HTTP(S) URLs")
		}
	}
	if input.TargetType != "interest" && input.TargetType != "watch" && input.TargetType != "config_plan" {
		return Invalid("target_type must be interest, watch, or config_plan")
	}
	if input.TargetType == "config_plan" {
		if input.Operation != "apply" || input.TargetID != nil || input.ExpectedRevision != nil {
			return Invalid("config_plan proposal needs apply without a target")
		}
		var plan ConfigPlan
		if err := strictPayload(input.Payload, &plan); err != nil {
			return err
		}
		if len(plan.Operations) == 0 || len(plan.Operations) > 50 {
			return Invalid("config_plan proposal needs 1-50 operations")
		}
		return nil
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
func sameFloat64(left, right *float64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
func sameTime(left, right *time.Time) bool {
	return left == nil && right == nil || left != nil && right != nil && left.Equal(*right)
}

func (a *App) CreateProposal(ctx context.Context, input CreateProposal) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /proposals", input, func(tx *sql.Tx) (any, error) {
		if err := validateProposal(input); err != nil {
			return nil, err
		}
		if input.EvidenceLinks == nil {
			input.EvidenceLinks = []string{}
		}
		if input.ExpiresAt != nil && !input.ExpiresAt.After(a.Now().UTC()) {
			return nil, Invalid("expires_at must be in the future")
		}
		if input.DuplicateOf != nil {
			canonical, err := resolveID(ctx, tx, "proposals", *input.DuplicateOf)
			if err != nil {
				return nil, err
			}
			input.DuplicateOf = &canonical
		}
		var existing Proposal
		existing, err := scanProposal(tx.QueryRowContext(ctx, "SELECT "+proposalColumns+" FROM proposals WHERE proposal_key=?", input.ProposalKey))
		if err == nil {
			same := existing.TargetType == input.TargetType && existing.Operation == input.Operation && sameString(existing.TargetID, input.TargetID) && sameInt64(existing.ExpectedRevision, input.ExpectedRevision) && existing.RationaleMD == input.RationaleMD && rawJSONEqual(existing.Payload, input.Payload) && slices.Equal(existing.EvidenceLinks, input.EvidenceLinks) && sameFloat64(existing.Confidence, input.Confidence) && sameString(existing.DuplicateOf, input.DuplicateOf) && sameTime(existing.ExpiresAt, input.ExpiresAt)
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
		evidence, _ := json.Marshal(input.EvidenceLinks)
		var expires any
		if input.ExpiresAt != nil {
			expires = input.ExpiresAt.UTC().UnixMilli()
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO proposals(id,proposal_key,target_type,target_id,expected_revision,operation,payload,rationale_md,state,evidence_links,confidence,duplicate_of,expires_at) VALUES(?,?,?,?,?,?,?,?,'pending',?,?,?,?)`, id, input.ProposalKey, input.TargetType, input.TargetID, input.ExpectedRevision, input.Operation, payload, input.RationaleMD, string(evidence), input.Confidence, input.DuplicateOf, expires)
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
	if p.TargetType == "config_plan" && p.Operation == "apply" {
		var plan ConfigPlan
		if err := strictPayload(p.Payload, &plan); err != nil {
			return err
		}
		seq, err := configSequence(ctx, tx)
		if err != nil {
			return err
		}
		token, err := configToken(plan, seq)
		if err != nil {
			return err
		}
		changes, err := a.executeConfigPlan(ctx, tx, plan, token)
		if err != nil {
			return err
		}
		return a.event(ctx, tx, "user", "config_plan", p.ID, "config_plan.applied", map[string]any{"proposal_id": p.ID, "changes": len(changes)})
	}
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
		id := p.CreatedID
		if id == "" {
			id = uuid.NewString()
		}
		if value.Slug == "" {
			value.Slug = defaultSlug("interest", id)
		}
		if !validSlug(value.Slug) {
			return Invalid("proposal contains invalid Interest slug")
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO interests(id,slug,title,instructions_md,state,revision,created_at,updated_at) VALUES(?,?,?,?,?,1,?,?)`, id, value.Slug, value.Title, value.InstructionsMD, value.State, now, now)
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
		ids, err := canonicalInterestIDs(ctx, tx, value.InterestIDs)
		if err != nil {
			return err
		}
		id := p.CreatedID
		if id == "" {
			id = uuid.NewString()
		}
		item := Watch{ID: id, Slug: value.Slug, InterestIDs: ids, MatchingPolicy: value.MatchingPolicy, ValidUntil: value.ValidUntil, Source: value.Source, InstructionsMD: value.InstructionsMD, IntervalSeconds: 7200, LookbackSeconds: 604800, State: "active"}
		if item.Slug == "" {
			item.Slug = defaultSlug("watcher", item.ID)
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
		if err := validateWatch(item); err != nil {
			return err
		}
		if err := checkWatchSlug(ctx, tx, item.Slug, item.ID); err != nil {
			return err
		}
		source, _ := json.Marshal(item.Source)
		var validUntil any
		if item.ValidUntil != nil {
			validUntil = item.ValidUntil.UTC().UnixMilli()
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO watches(id,slug,matching_policy,valid_until,source,instructions_md,interval_seconds,lookback_seconds,state,revision,next_due_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,1,?,?,?)`, item.ID, item.Slug, item.MatchingPolicy, validUntil, string(source), item.InstructionsMD, item.IntervalSeconds, item.LookbackSeconds, item.State, now, now, now)
		if err != nil {
			return err
		}
		if err = replaceWatchInterests(ctx, tx, item.ID, ids, now); err != nil {
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
			if value.Slug.Set {
				item.Slug = value.Slug.Value
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
		if !validSlug(item.Slug) {
			return Invalid("proposal contains invalid Interest slug")
		}
		_, err = tx.ExecContext(ctx, `UPDATE interests SET slug=?,title=?,instructions_md=?,state=?,revision=revision+1,updated_at=? WHERE id=?`, item.Slug, item.Title, item.InstructionsMD, item.State, now, item.ID)
		if err != nil {
			return err
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
		if value.Slug.Set {
			item.Slug = value.Slug.Value
		}
		if value.MatchingPolicy.Set {
			item.MatchingPolicy = value.MatchingPolicy.Value
		}
		if value.ValidUntil.Set {
			item.ValidUntil = value.ValidUntil.Value
		}
		if value.ClearValidUntil {
			if value.ValidUntil.Set {
				return Invalid("valid_until and clear_valid_until cannot be combined")
			}
			item.ValidUntil = nil
		}
		if value.InterestIDs.Set {
			item.InterestIDs, err = canonicalInterestIDs(ctx, tx, value.InterestIDs.Value)
			if err != nil {
				return err
			}
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
	if err = checkWatchSlug(ctx, tx, item.Slug, item.ID); err != nil {
		return err
	}
	source, _ := json.Marshal(item.Source)
	var cursor any
	if item.Cursor != nil {
		cursor = string(item.Cursor)
	}
	var validUntil any
	if item.ValidUntil != nil {
		validUntil = item.ValidUntil.UTC().UnixMilli()
	}
	previous, err := getWatch(ctx, tx, item.ID)
	if err != nil {
		return err
	}
	if previous.Source != item.Source || previous.InstructionsMD != item.InstructionsMD || previous.IntervalSeconds != item.IntervalSeconds || previous.LookbackSeconds != item.LookbackSeconds || (previous.State != "active" && item.State == "active") {
		item.NextDueAt = time.UnixMilli(now).UTC()
	}
	if previous.Source != item.Source {
		item.SourceGeneration++
	}
	_, err = tx.ExecContext(ctx, `UPDATE watches SET slug=?,matching_policy=?,valid_until=?,source=?,instructions_md=?,interval_seconds=?,lookback_seconds=?,state=?,revision=revision+1,source_generation=?,cursor=?,next_due_at=?,updated_at=? WHERE id=?`, item.Slug, item.MatchingPolicy, validUntil, string(source), item.InstructionsMD, item.IntervalSeconds, item.LookbackSeconds, item.State, item.SourceGeneration, cursor, item.NextDueAt.UnixMilli(), now, item.ID)
	if err != nil {
		return err
	}
	if p.Operation == "update" {
		var value watchUpdatePayload
		if err = strictPayload(p.Payload, &value); err != nil {
			return err
		}
		if value.InterestIDs.Set {
			if err = replaceWatchInterests(ctx, tx, item.ID, item.InterestIDs, now); err != nil {
				return err
			}
		}
	}
	return a.event(ctx, tx, "user", "watch", item.ID, "watch.updated", map[string]any{"proposal_id": p.ID})
}

func (a *App) ResolveProposal(ctx context.Context, id string, input ResolveProposal) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /proposals/"+id+"/resolve", input, func(tx *sql.Tx) (any, error) {
		if input.Resolution != "accepted" && input.Resolution != "rejected" && input.Resolution != "snoozed" && input.Resolution != "merged" {
			return nil, Invalid("resolution must be accepted, rejected, snoozed, or merged")
		}
		if input.Resolution == "snoozed" && (input.SnoozeUntil == nil || !input.SnoozeUntil.After(a.Now().UTC())) {
			return nil, Invalid("snooze_until must be in the future")
		}
		if input.Resolution == "merged" && input.MergeInto == "" {
			return nil, Invalid("merge_into is required")
		}
		item, err := getProposal(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if input.Resolution == "merged" {
			target, resolveErr := getProposal(ctx, tx, input.MergeInto)
			if resolveErr != nil {
				return nil, resolveErr
			}
			if target.ID == item.ID {
				return nil, Invalid("proposal cannot merge into itself")
			}
			input.MergeInto = target.ID
		}
		if item.State != "pending" {
			if item.State == input.Resolution || item.State == "accepted" && input.Resolution == "merged" && item.MergedInto != nil && *item.MergedInto == input.MergeInto {
				return item, nil
			}
			return nil, &Error{Status: 409, Code: "proposal_resolved", Message: "Proposal already has a different resolution"}
		}
		if item.ExpiresAt != nil && !item.ExpiresAt.After(a.Now().UTC()) && input.Resolution != "rejected" {
			return nil, &Error{Status: 409, Code: "proposal_expired", Message: "Proposal expired; create a fresh review proposal"}
		}
		if input.Resolution == "accepted" {
			if err = a.applyProposal(ctx, tx, item); err != nil {
				return nil, err
			}
		}
		state := input.Resolution
		var snoozed, merged any
		if input.Resolution == "snoozed" {
			state = "pending"
			snoozed = input.SnoozeUntil.UTC().UnixMilli()
		}
		if input.Resolution == "merged" {
			state = "accepted"
			merged = input.MergeInto
		}
		_, err = tx.ExecContext(ctx, `UPDATE proposals SET state=?,snoozed_until=?,merged_into=? WHERE id=?`, state, snoozed, merged, id)
		if err != nil {
			return nil, err
		}
		item, err = getProposal(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return item, a.event(ctx, tx, "user", "proposal", id, "proposal."+input.Resolution, map[string]any{"proposal_key": item.ProposalKey, "snoozed_until": item.SnoozedUntil, "merged_into": item.MergedInto})
	})
}
