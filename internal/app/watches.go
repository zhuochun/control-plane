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
	ID               string          `json:"id"`
	Slug             string          `json:"slug"`
	InterestIDs      []string        `json:"interest_ids"`
	MatchingPolicy   string          `json:"matching_policy"`
	ValidUntil       *time.Time      `json:"valid_until,omitempty"`
	Source           WatchSource     `json:"source"`
	InstructionsMD   string          `json:"instructions_md"`
	IntervalSeconds  int64           `json:"interval_seconds"`
	LookbackSeconds  int64           `json:"lookback_seconds"`
	State            string          `json:"state"`
	Revision         int64           `json:"revision"`
	SourceGeneration int64           `json:"source_generation"`
	Cursor           json.RawMessage `json:"cursor"`
	NextDueAt        time.Time       `json:"next_due_at"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}
type CreateWatch struct {
	RequestID       string        `json:"request_id"`
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
type UpdateWatch struct {
	RequestID        string             `json:"request_id"`
	ExpectedRevision int64              `json:"expected_revision"`
	Slug             Field[string]      `json:"slug"`
	InterestIDs      Field[[]string]    `json:"interest_ids"`
	MatchingPolicy   Field[string]      `json:"matching_policy"`
	ValidUntil       Field[*time.Time]  `json:"valid_until"`
	ClearValidUntil  bool               `json:"clear_valid_until,omitempty"`
	Source           Field[WatchSource] `json:"source"`
	InstructionsMD   Field[string]      `json:"instructions_md"`
	IntervalSeconds  Field[int64]       `json:"interval_seconds"`
	LookbackSeconds  Field[int64]       `json:"lookback_seconds"`
	State            Field[string]      `json:"state"`
}

const watchColumns = `id,slug,matching_policy,valid_until,source,instructions_md,interval_seconds,lookback_seconds,state,revision,source_generation,cursor,next_due_at,created_at,updated_at,
 (SELECT COALESCE(json_group_array(interest_id),'[]') FROM (SELECT interest_id FROM watch_interests WHERE watch_id=watches.id ORDER BY interest_id))`

func scanWatch(row scanner) (Watch, error) {
	var w Watch
	var source, links string
	var cursor sql.NullString
	var valid sql.NullInt64
	var due, created, updated int64
	err := row.Scan(&w.ID, &w.Slug, &w.MatchingPolicy, &valid, &source, &w.InstructionsMD, &w.IntervalSeconds, &w.LookbackSeconds, &w.State, &w.Revision, &w.SourceGeneration, &cursor, &due, &created, &updated, &links)
	if err != nil {
		return w, missing(err)
	}
	if err = json.Unmarshal([]byte(source), &w.Source); err != nil {
		return w, err
	}
	if err = json.Unmarshal([]byte(links), &w.InterestIDs); err != nil {
		return w, err
	}
	if cursor.Valid {
		w.Cursor = json.RawMessage(cursor.String)
	}
	if valid.Valid {
		v := time.UnixMilli(valid.Int64).UTC()
		w.ValidUntil = &v
	}
	w.NextDueAt, w.CreatedAt, w.UpdatedAt = time.UnixMilli(due).UTC(), time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()
	return w, nil
}
func getWatch(ctx context.Context, db querier, id string) (Watch, error) {
	canonical, err := resolveID(ctx, db, "watches", id)
	if err != nil {
		return Watch{}, err
	}
	return scanWatch(db.QueryRowContext(ctx, "SELECT "+watchColumns+" FROM watches WHERE id=?", canonical))
}
func (a *App) Watch(ctx context.Context, id string) (Watch, error) {
	return getWatch(ctx, a.Store.DB, id)
}
func (a *App) Watches(ctx context.Context, interestID string, states ...string) ([]Watch, error) {
	query := "SELECT " + watchColumns + " FROM watches"
	args := []any{}
	conditions := []string{}
	if interestID != "" {
		id, err := resolveID(ctx, a.Store.DB, "interests", interestID)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, "(matching_policy='broad' OR EXISTS (SELECT 1 FROM watch_interests WHERE watch_id=watches.id AND interest_id=?))")
		args = append(args, id)
	}
	state := "active"
	if len(states) > 0 && states[0] != "" {
		state = states[0]
	}
	if state != "all" {
		if !validState(state) {
			return nil, Invalid("invalid state")
		}
		conditions = append(conditions, "state=?")
		args = append(args, state)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
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
func validateWatch(w Watch) error {
	if !validSlug(w.Slug) {
		return Invalid("invalid slug")
	}
	if w.MatchingPolicy != "broad" && w.MatchingPolicy != "explicit" {
		return Invalid("matching_policy must be broad or explicit")
	}
	if w.MatchingPolicy == "broad" && len(w.InterestIDs) > 0 {
		return Invalid("broad Watcher cannot have explicit Interest links")
	}
	if strings.TrimSpace(w.Source.Kind) == "" || strings.TrimSpace(w.Source.Locator) == "" {
		return Invalid("source.kind and source.locator are required")
	}
	if w.IntervalSeconds < 1 || w.IntervalSeconds > 31536000 {
		return Invalid("interval_seconds must be between 1 and 31536000")
	}
	if w.LookbackSeconds < 1 {
		return Invalid("lookback_seconds must be positive")
	}
	if !validState(w.State) {
		return Invalid("invalid state")
	}
	return nil
}
func canonicalInterestIDs(ctx context.Context, tx *sql.Tx, ids []string) ([]string, error) {
	result := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, ref := range ids {
		item, err := getInterest(ctx, tx, ref)
		if err != nil {
			return nil, err
		}
		if seen[item.ID] {
			return nil, Invalid("duplicate Interest link")
		}
		seen[item.ID] = true
		result = append(result, item.ID)
	}
	return result, nil
}
func replaceWatchInterests(ctx context.Context, tx *sql.Tx, id string, ids []string, now int64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM watch_interests WHERE watch_id=?", id); err != nil {
		return err
	}
	for _, interestID := range ids {
		if _, err := tx.ExecContext(ctx, "INSERT INTO watch_interests(watch_id,interest_id,linked_at) VALUES(?,?,?)", id, interestID, now); err != nil {
			return err
		}
	}
	return nil
}
func checkWatchSlug(ctx context.Context, tx *sql.Tx, slug, id string) error {
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM watches WHERE slug=? AND id<>?", slug, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return &Error{Status: 409, Code: "slug_conflict", Message: "Watcher slug is already in use"}
	}
	return nil
}
func (a *App) CreateWatch(ctx context.Context, input CreateWatch) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /watches", input, func(tx *sql.Tx) (any, error) {
		ids, err := canonicalInterestIDs(ctx, tx, input.InterestIDs)
		if err != nil {
			return nil, err
		}
		w := Watch{ID: uuid.NewString(), Slug: input.Slug, InterestIDs: ids, MatchingPolicy: input.MatchingPolicy, ValidUntil: input.ValidUntil, Source: input.Source, InstructionsMD: input.InstructionsMD, IntervalSeconds: 7200, LookbackSeconds: 604800, State: "active"}
		if w.Slug == "" {
			w.Slug = defaultSlug("watcher", w.ID)
		}
		if input.IntervalSeconds.Set {
			w.IntervalSeconds = input.IntervalSeconds.Value
		}
		if input.LookbackSeconds.Set {
			w.LookbackSeconds = input.LookbackSeconds.Value
		}
		if input.State.Set {
			w.State = input.State.Value
		}
		if err = validateWatch(w); err != nil {
			return nil, err
		}
		if err = checkWatchSlug(ctx, tx, w.Slug, w.ID); err != nil {
			return nil, err
		}
		source, err := json.Marshal(w.Source)
		if err != nil {
			return nil, err
		}
		now := a.Now().UTC().UnixMilli()
		var valid any
		if w.ValidUntil != nil {
			valid = w.ValidUntil.UTC().UnixMilli()
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO watches(id,slug,matching_policy,valid_until,source,instructions_md,interval_seconds,lookback_seconds,state,revision,next_due_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,1,?,?,?)", w.ID, w.Slug, w.MatchingPolicy, valid, string(source), w.InstructionsMD, w.IntervalSeconds, w.LookbackSeconds, w.State, now, now, now)
		if err != nil {
			return nil, err
		}
		if err = replaceWatchInterests(ctx, tx, w.ID, ids, now); err != nil {
			return nil, err
		}
		w, err = getWatch(ctx, tx, w.ID)
		if err != nil {
			return nil, err
		}
		return w, a.event(ctx, tx, "user", "watch", w.ID, "watch.created", w)
	})
}
func (a *App) UpdateWatch(ctx context.Context, id string, input UpdateWatch) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "PATCH /watches/"+id, input, func(tx *sql.Tx) (any, error) {
		w, err := getWatch(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		id = w.ID
		if input.ExpectedRevision != w.Revision {
			return nil, revisionConflict(w.Revision)
		}
		previous := w
		if input.Slug.Set {
			w.Slug = input.Slug.Value
		}
		if input.MatchingPolicy.Set {
			w.MatchingPolicy = input.MatchingPolicy.Value
		}
		if input.ValidUntil.Set {
			w.ValidUntil = input.ValidUntil.Value
		}
		if input.ClearValidUntil {
			if input.ValidUntil.Set {
				return nil, Invalid("valid_until and clear_valid_until cannot be combined")
			}
			w.ValidUntil = nil
		}
		if input.InterestIDs.Set {
			w.InterestIDs, err = canonicalInterestIDs(ctx, tx, input.InterestIDs.Value)
			if err != nil {
				return nil, err
			}
		}
		if input.Source.Set {
			w.Source = input.Source.Value
		}
		if input.InstructionsMD.Set {
			w.InstructionsMD = input.InstructionsMD.Value
		}
		if input.IntervalSeconds.Set {
			w.IntervalSeconds = input.IntervalSeconds.Value
		}
		if input.LookbackSeconds.Set {
			w.LookbackSeconds = input.LookbackSeconds.Value
		}
		if input.State.Set {
			w.State = input.State.Value
		}
		if err = validateWatch(w); err != nil {
			return nil, err
		}
		if err = checkWatchSlug(ctx, tx, w.Slug, id); err != nil {
			return nil, err
		}
		now := a.Now().UTC()
		if previous.Source != w.Source || previous.InstructionsMD != w.InstructionsMD || previous.IntervalSeconds != w.IntervalSeconds || previous.LookbackSeconds != w.LookbackSeconds || (previous.State != "active" && w.State == "active") {
			w.NextDueAt = now
		}
		if previous.Source != w.Source {
			w.Cursor = nil
			w.SourceGeneration++
		}
		source, err := json.Marshal(w.Source)
		if err != nil {
			return nil, err
		}
		var cursor, valid any
		if w.Cursor != nil {
			cursor = string(w.Cursor)
		}
		if w.ValidUntil != nil {
			valid = w.ValidUntil.UTC().UnixMilli()
		}
		_, err = tx.ExecContext(ctx, "UPDATE watches SET slug=?,matching_policy=?,valid_until=?,source=?,instructions_md=?,interval_seconds=?,lookback_seconds=?,state=?,revision=revision+1,source_generation=?,cursor=?,next_due_at=?,updated_at=? WHERE id=?", w.Slug, w.MatchingPolicy, valid, string(source), w.InstructionsMD, w.IntervalSeconds, w.LookbackSeconds, w.State, w.SourceGeneration, cursor, w.NextDueAt.UnixMilli(), now.UnixMilli(), id)
		if err != nil {
			return nil, err
		}
		if input.InterestIDs.Set {
			if err = replaceWatchInterests(ctx, tx, id, w.InterestIDs, now.UnixMilli()); err != nil {
				return nil, err
			}
		}
		w, err = getWatch(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return w, a.event(ctx, tx, "user", "watch", id, "watch.updated", map[string]any{"before": previous, "after": w})
	})
}
