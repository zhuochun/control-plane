package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// ConfigPlan is an ordered, bounded change set. Each operation uses the same
// payload shape and revision fence as a direct configuration command.
type ConfigPlan struct {
	Operations []ConfigOperation `json:"operations"`
}
type ConfigOperation struct {
	TargetType       string          `json:"target_type"`
	Operation        string          `json:"operation"`
	Target           string          `json:"target,omitempty"`
	ExpectedRevision *int64          `json:"expected_revision,omitempty"`
	Payload          json.RawMessage `json:"payload,omitempty"`
}
type ApplyConfigPlan struct {
	RequestID    string     `json:"request_id,omitempty"`
	PreviewToken string     `json:"preview_token"`
	Plan         ConfigPlan `json:"plan"`
}
type ConfigEffect struct {
	TargetType          string   `json:"target_type"`
	Operation           string   `json:"operation"`
	Target              string   `json:"target"`
	Before              any      `json:"before,omitempty"`
	After               any      `json:"after,omitempty"`
	AffectedItems       int      `json:"affected_items"`
	AffectedWatchers    []string `json:"affected_watchers,omitempty"`
	CursorReset         bool     `json:"cursor_reset"`
	Consequence         string   `json:"consequence,omitempty"`
	OverlappingWatchers []string `json:"overlapping_watchers,omitempty"`
}
type ConfigPreview struct {
	PreviewToken          string         `json:"preview_token"`
	Changes               []ConfigEffect `json:"changes"`
	DueBefore             int            `json:"due_before"`
	DueAfter              int            `json:"due_after"`
	ForwardOnlyAssessment bool           `json:"forward_only_assessment"`
}

func configSequence(ctx context.Context, tx *sql.Tx) (int64, error) {
	var seq int64
	err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM events").Scan(&seq)
	return seq, err
}
func configToken(plan ConfigPlan, seq int64) (string, error) {
	body, err := json.Marshal(struct {
		Plan     ConfigPlan `json:"plan"`
		Sequence int64      `json:"sequence"`
	}{plan, seq})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
func dueCountTx(ctx context.Context, tx *sql.Tx, now int64) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM watches w WHERE w.state='active' AND w.next_due_at<=?
 AND (w.valid_until IS NULL OR w.valid_until>?) AND EXISTS (
 SELECT 1 FROM interests i WHERE i.state='active' AND
 (w.matching_policy='broad' OR EXISTS (SELECT 1 FROM watch_interests wi WHERE wi.watch_id=w.id AND wi.interest_id=i.id)))`, now, now).Scan(&count)
	return count, err
}
func (a *App) executeConfigPlan(ctx context.Context, tx *sql.Tx, plan ConfigPlan, token string) ([]ConfigEffect, error) {
	if len(plan.Operations) == 0 || len(plan.Operations) > 50 {
		return nil, Invalid("plan must contain 1-50 operations")
	}
	effects := make([]ConfigEffect, 0, len(plan.Operations))
	for index, operation := range plan.Operations {
		if operation.TargetType != "interest" && operation.TargetType != "watch" {
			return nil, Invalid("target_type must be interest or watch")
		}
		if operation.Operation != "create" && operation.Operation != "update" && operation.Operation != "deprecate" {
			return nil, Invalid("operation must be create, update, or deprecate")
		}
		if operation.Operation == "create" && operation.Target != "" {
			return nil, Invalid("create operation cannot name a target")
		}
		if operation.Operation != "create" && (operation.Target == "" || operation.ExpectedRevision == nil) {
			return nil, Invalid("updates and deprecations need target and expected_revision")
		}
		effect := ConfigEffect{TargetType: operation.TargetType, Operation: operation.Operation}
		var beforeWatch Watch
		var targetID string
		if operation.Operation != "create" {
			if operation.TargetType == "interest" {
				before, err := getInterest(ctx, tx, operation.Target)
				if err != nil {
					return nil, err
				}
				effect.Before = before
				targetID = before.ID
				if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM item_interests WHERE interest_id=?", targetID).Scan(&effect.AffectedItems); err != nil {
					return nil, err
				}
			} else {
				before, err := getWatch(ctx, tx, operation.Target)
				if err != nil {
					return nil, err
				}
				effect.Before = before
				beforeWatch = before
				targetID = before.ID
				if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM items WHERE watch_id=?", targetID).Scan(&effect.AffectedItems); err != nil {
					return nil, err
				}
			}
		}
		proposal := Proposal{TargetType: operation.TargetType, Operation: operation.Operation, Payload: operation.Payload, ExpectedRevision: operation.ExpectedRevision}
		if operation.Operation == "create" {
			proposal.CreatedID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:%d", token, index))).String()
		}
		if targetID != "" {
			proposal.TargetID = &targetID
		}
		if err := a.applyProposal(ctx, tx, proposal); err != nil {
			return nil, err
		}
		if operation.Operation == "create" {
			var named struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(operation.Payload, &named); err != nil {
				return nil, Invalid("create plan payload must have a slug")
			}
			if !validSlug(named.Slug) {
				return nil, Invalid("create plan payload must have a valid slug")
			}
			targetID = proposal.CreatedID
		}
		if operation.TargetType == "interest" {
			after, err := getInterest(ctx, tx, targetID)
			if err != nil {
				return nil, err
			}
			effect.Target = after.Slug
			effect.After = after
			rows, err := tx.QueryContext(ctx, `SELECT slug FROM watches w WHERE w.matching_policy='broad'
OR EXISTS (SELECT 1 FROM watch_interests wi WHERE wi.watch_id=w.id AND wi.interest_id=?) ORDER BY slug`, after.ID)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var slug string
				if err = rows.Scan(&slug); err != nil {
					rows.Close()
					return nil, err
				}
				effect.AffectedWatchers = append(effect.AffectedWatchers, slug)
			}
			if err = rows.Err(); err != nil {
				rows.Close()
				return nil, err
			}
			if err = rows.Close(); err != nil {
				return nil, err
			}
			effect.Consequence = "New Interest assessments start with the next Run; older source input is not reassessed."
		} else {
			after, err := getWatch(ctx, tx, targetID)
			if err != nil {
				return nil, err
			}
			effect.Target = after.Slug
			effect.After = after
			effect.CursorReset = operation.Operation != "create" && beforeWatch.Source != after.Source
			if effect.CursorReset {
				effect.Consequence = "The old source checkpoint remains historical; the new source scope starts without a cursor."
			}
			effect.AffectedWatchers = []string{after.Slug}
			rows, err := tx.QueryContext(ctx, "SELECT slug FROM watches WHERE source=? AND id<>? ORDER BY slug", mustJSON(after.Source), after.ID)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var slug string
				if err = rows.Scan(&slug); err != nil {
					rows.Close()
					return nil, err
				}
				effect.OverlappingWatchers = append(effect.OverlappingWatchers, slug)
			}
			if err = rows.Err(); err != nil {
				rows.Close()
				return nil, err
			}
			if err = rows.Close(); err != nil {
				return nil, err
			}
		}
		effects = append(effects, effect)
	}
	return effects, nil
}
func mustJSON(value any) string { body, _ := json.Marshal(value); return string(body) }
func (a *App) PreviewConfigPlan(ctx context.Context, plan ConfigPlan) (ConfigPreview, error) {
	tx, err := a.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return ConfigPreview{}, err
	}
	defer tx.Rollback()
	seq, err := configSequence(ctx, tx)
	if err != nil {
		return ConfigPreview{}, err
	}
	token, err := configToken(plan, seq)
	if err != nil {
		return ConfigPreview{}, err
	}
	now := a.Now().UTC().UnixMilli()
	before, err := dueCountTx(ctx, tx, now)
	if err != nil {
		return ConfigPreview{}, err
	}
	changes, err := a.executeConfigPlan(ctx, tx, plan, token)
	if err != nil {
		return ConfigPreview{}, err
	}
	after, err := dueCountTx(ctx, tx, a.Now().UTC().UnixMilli())
	if err != nil {
		return ConfigPreview{}, err
	}
	return ConfigPreview{PreviewToken: token, Changes: changes, DueBefore: before, DueAfter: after, ForwardOnlyAssessment: true}, nil
}
func (a *App) CommitConfigPlan(ctx context.Context, input ApplyConfigPlan) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /config/plans/apply", input, func(tx *sql.Tx) (any, error) {
		seq, err := configSequence(ctx, tx)
		if err != nil {
			return nil, err
		}
		token, err := configToken(input.Plan, seq)
		if err != nil {
			return nil, err
		}
		if token != input.PreviewToken {
			return nil, &Error{Status: 409, Code: "stale_config_preview", Message: "Configuration changed after preview; preview this plan again"}
		}
		now := a.Now().UTC().UnixMilli()
		before, err := dueCountTx(ctx, tx, now)
		if err != nil {
			return nil, err
		}
		changes, err := a.executeConfigPlan(ctx, tx, input.Plan, token)
		if err != nil {
			return nil, err
		}
		after, err := dueCountTx(ctx, tx, a.Now().UTC().UnixMilli())
		if err != nil {
			return nil, err
		}
		result := ConfigPreview{PreviewToken: token, Changes: changes, DueBefore: before, DueAfter: after, ForwardOnlyAssessment: true}
		if err = a.event(ctx, tx, "user", "config_plan", fmt.Sprintf("%x", sha256.Sum256([]byte(token))), "config_plan.applied", map[string]any{"count": len(changes)}); err != nil {
			return nil, err
		}
		return result, nil
	})
}
