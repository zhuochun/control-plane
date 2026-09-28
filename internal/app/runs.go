package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const contextPageSize = 50
const contextContinuationPageSize = 150
const contextPageBytes = 64 << 10
const attentionTitleMaxBytes = 256
const attentionSummaryMaxBytes = 512
const attentionReasonMaxBytes = 256

type SelectedWatch struct {
	ID               string          `json:"id"`
	Revision         int64           `json:"revision"`
	SourceGeneration int64           `json:"source_generation"`
	MatchingPolicy   string          `json:"matching_policy"`
	Interests        []InterestMatch `json:"interests"`
	Source           WatchSource     `json:"source"`
	InstructionsMD   string          `json:"instructions_md"`
	Cursor           json.RawMessage `json:"cursor"`
	IntervalSeconds  int64           `json:"interval_seconds"`
	LookbackSeconds  int64           `json:"lookback_seconds"`
	NextDueAt        time.Time       `json:"next_due_at"`
	RelatedItems     []AttentionItem `json:"related_items"`
}

type InterestMatch struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

type AttentionItem struct {
	ID              string         `json:"id"`
	Kind            string         `json:"kind"`
	Interests       []ItemInterest `json:"interests"`
	WatchID         *string        `json:"watch_id,omitempty"`
	ParentID        *string        `json:"parent_id,omitempty"`
	Title           string         `json:"title"`
	Summary         string         `json:"summary"`
	ContentVersion  int64          `json:"content_version"`
	TodoState       string         `json:"todo_state"`
	RemindAt        *time.Time     `json:"remind_at,omitempty"`
	TruncatedFields []string       `json:"truncated_fields,omitempty"`
}

type Run struct {
	ID              string          `json:"id"`
	RunnerLabel     string          `json:"runner_label"`
	Status          string          `json:"status"`
	StartedAt       time.Time       `json:"started_at"`
	EndedAt         *time.Time      `json:"ended_at"`
	SelectedWatches []SelectedWatch `json:"-"`
	AfterSeq        int64           `json:"after_seq"`
	ThroughSeq      int64           `json:"through_seq"`
	Summary         string          `json:"summary"`
}

// RunSummary is the human-facing history shape. The active packet keeps its
// selected Watches under brief; history still exposes them for portal counts
// and run browsing without changing the authoritative run response.
type RunSummary struct {
	Run
	SelectedWatches []SelectedWatch `json:"selected_watches"`
}

type StartRun struct {
	RequestID   string          `json:"request_id"`
	RunnerLabel string          `json:"runner_label"`
	WatchIDs    Field[[]string] `json:"watch_ids"`
	Force       bool            `json:"force,omitempty"`
}
type FinishRun struct {
	RequestID     string `json:"request_id"`
	Summary       string `json:"summary"`
	AckThroughSeq *int64 `json:"ack_through_seq,omitempty"`
}
type AbandonRun struct {
	RequestID string `json:"request_id"`
	Reason    string `json:"reason"`
}

func scanRun(row scanner) (Run, error) {
	var run Run
	var started int64
	var ended sql.NullInt64
	var selected string
	err := row.Scan(&run.ID, &run.RunnerLabel, &run.Status, &started, &ended, &selected, &run.AfterSeq, &run.ThroughSeq, &run.Summary)
	if err != nil {
		return run, missing(err)
	}
	run.StartedAt = time.UnixMilli(started).UTC()
	if ended.Valid {
		value := time.UnixMilli(ended.Int64).UTC()
		run.EndedAt = &value
	}
	if err = json.Unmarshal([]byte(selected), &run.SelectedWatches); err != nil {
		return run, err
	}
	return run, nil
}

const runColumns = `id,runner_label,status,started_at,ended_at,selected_watches,after_seq,through_seq,summary`

func getRun(ctx context.Context, db querier, id string) (Run, error) {
	canonical, err := resolveID(ctx, db, "runs", id)
	if err != nil {
		return Run{}, err
	}
	return scanRun(db.QueryRowContext(ctx, "SELECT "+runColumns+" FROM runs WHERE id=?", canonical))
}
func (a *App) Run(ctx context.Context, id string) (Run, error) { return getRun(ctx, a.Store.DB, id) }
func (a *App) ActiveRun(ctx context.Context) (Run, error) {
	var id string
	if err := a.Store.DB.QueryRowContext(ctx, "SELECT id FROM runs WHERE status='running'").Scan(&id); err != nil {
		return Run{}, missing(err)
	}
	return getRun(ctx, a.Store.DB, id)
}
func (a *App) FinishActiveRun(ctx context.Context, input FinishRun) (json.RawMessage, error) {
	run, err := a.ActiveRun(ctx)
	if err != nil {
		return nil, err
	}
	return a.FinishRun(ctx, run.ID, input)
}
func (a *App) RunDetail(ctx context.Context, id string) (map[string]any, error) {
	run, err := a.Run(ctx, id)
	if err != nil {
		return nil, err
	}
	results, err := a.WatchResults(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"run": run, "selected_watches": compactSelectedWatches(run.SelectedWatches), "results": results}, nil
}
func (a *App) Runs(ctx context.Context) ([]RunSummary, error) {
	rows, err := a.Store.DB.QueryContext(ctx, "SELECT "+runColumns+" FROM runs ORDER BY started_at DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RunSummary{}
	for rows.Next() {
		item, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, RunSummary{Run: item, SelectedWatches: compactSelectedWatches(item.SelectedWatches)})
	}
	return items, rows.Err()
}

func (a *App) RunsPage(ctx context.Context, afterID string, limit int) ([]RunSummary, bool, error) {
	if limit < 1 || limit > 100 {
		return nil, false, Invalid("limit must be between 1 and 100")
	}
	query := "SELECT " + runColumns + " FROM runs"
	args := []any{}
	if afterID != "" {
		var started int64
		err := a.Store.DB.QueryRowContext(ctx, "SELECT started_at FROM runs WHERE id=?", afterID).Scan(&started)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, Invalid("Continuation cursor is not in this collection")
		}
		if err != nil {
			return nil, false, err
		}
		query += " WHERE started_at<? OR (started_at=? AND id<?)"
		args = append(args, started, started, afterID)
	}
	query += " ORDER BY started_at DESC,id DESC LIMIT ?"
	args = append(args, limit+1)
	rows, err := a.Store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]RunSummary, 0, limit+1)
	for rows.Next() {
		run, scanErr := scanRun(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, RunSummary{Run: run, SelectedWatches: compactSelectedWatches(run.SelectedWatches)})
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	return items, more, nil
}

func selectWatches(ctx context.Context, tx *sql.Tx, input StartRun, now int64) ([]SelectedWatch, int, error) {
	if input.Force && !input.WatchIDs.Set {
		return nil, 0, Invalid("force requires explicit watch_ids")
	}
	watchIDs := append([]string(nil), input.WatchIDs.Value...)
	if input.WatchIDs.Set {
		for index, id := range watchIDs {
			canonical, err := resolveID(ctx, tx, "watches", id)
			if err != nil {
				return nil, 0, err
			}
			watchIDs[index] = canonical
		}
	}
	query := `SELECT w.id,w.revision,w.source_generation,w.matching_policy,w.source,w.instructions_md,w.cursor,w.interval_seconds,w.lookback_seconds,w.next_due_at
FROM watches w WHERE w.state='active' AND (w.valid_until IS NULL OR w.valid_until>?) AND EXISTS (
 SELECT 1 FROM interests i WHERE i.state='active' AND
 (w.matching_policy='broad' OR EXISTS (SELECT 1 FROM watch_interests wi WHERE wi.watch_id=w.id AND wi.interest_id=i.id))
)`
	args := []any{now}
	if input.WatchIDs.Set {
		if len(watchIDs) == 0 {
			return []SelectedWatch{}, 0, nil
		}
		query += " AND w.id IN ("
		for index, id := range watchIDs {
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
	if input.WatchIDs.Set && eligible != len(watchIDs) {
		return nil, 0, Invalid("Every selected Watcher must be active, unexpired, have an applicable active Interest, and be due unless forced")
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
		if err = rows.Scan(&item.ID, &item.Revision, &item.SourceGeneration, &item.MatchingPolicy, &source, &item.InstructionsMD, &nullable, &item.IntervalSeconds, &item.LookbackSeconds, &due); err != nil {
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
		interestRows, interestErr := tx.QueryContext(ctx, `SELECT i.id,i.revision FROM interests i
WHERE i.state='active' AND (?='broad' OR EXISTS (SELECT 1 FROM watch_interests wi WHERE wi.watch_id=? AND wi.interest_id=i.id))
ORDER BY i.id`, selected[index].MatchingPolicy, selected[index].ID)
		if interestErr != nil {
			return nil, 0, interestErr
		}
		selected[index].Interests = []InterestMatch{}
		for interestRows.Next() {
			var match InterestMatch
			if err = interestRows.Scan(&match.ID, &match.Revision); err != nil {
				interestRows.Close()
				return nil, 0, err
			}
			selected[index].Interests = append(selected[index].Interests, match)
		}
		if err = interestRows.Err(); err != nil {
			interestRows.Close()
			return nil, 0, err
		}
		if err = interestRows.Close(); err != nil {
			return nil, 0, err
		}
		selected[index].RelatedItems = []AttentionItem{}
		itemRows, itemErr := tx.QueryContext(ctx, `SELECT `+itemColumns+` FROM items WHERE watch_id=? ORDER BY content_updated_at DESC,id LIMIT 20`, selected[index].ID)
		if itemErr != nil {
			return nil, 0, itemErr
		}
		for itemRows.Next() {
			item, scanErr := scanItem(itemRows)
			if scanErr != nil {
				itemRows.Close()
				return nil, 0, scanErr
			}
			selected[index].RelatedItems = append(selected[index].RelatedItems, attentionItem(item))
		}
		if itemErr = itemRows.Close(); itemErr != nil {
			return nil, 0, itemErr
		}
	}
	more := eligible - len(selected)
	return selected, more, nil
}

type runContextSnapshot struct {
	Interests []Interest        `json:"interests"`
	Attention []AttentionItem   `json:"attention_items"`
	Contexts  map[string]string `json:"contexts"`
}

type contextCursor struct {
	RunID      string `json:"run_id,omitempty"`
	Collection string `json:"collection"`
	Offset     int    `json:"offset"`
}

func encodeContextCursor(cursor contextCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeContextCursor(value string) (contextCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return contextCursor{}, Invalid("Invalid continuation cursor")
	}
	var cursor contextCursor
	if err = json.Unmarshal(data, &cursor); err != nil || cursor.Offset < 0 || (cursor.Collection != "interests" && cursor.Collection != "attention_items") {
		return contextCursor{}, Invalid("Invalid continuation cursor")
	}
	return cursor, nil
}

func activeInterests(ctx context.Context, db rowsQuerier) ([]Interest, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+interestColumns+" FROM interests WHERE state='active' ORDER BY created_at,id")
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

func attentionItems(ctx context.Context, db rowsQuerier, now int64) ([]AttentionItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+itemColumns+` FROM items
WHERE (origin<>'agent' OR watch_id IS NOT NULL) AND (todo_state='todo' OR acknowledged_content_version<content_version OR (remind_at IS NOT NULL AND remind_at<=?))
ORDER BY CASE WHEN remind_at IS NOT NULL AND remind_at<=? THEN 0 WHEN todo_state='todo' THEN 1 ELSE 2 END, COALESCE(remind_at,content_updated_at),content_updated_at DESC,id`, now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AttentionItem{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, attentionItem(item))
	}
	return items, rows.Err()
}

func attentionItem(item Item) AttentionItem {
	return AttentionItem{ID: item.ID, Kind: item.Kind, Interests: item.Interests, WatchID: item.WatchID, ParentID: item.ParentID, Title: item.Title, Summary: item.Summary, ContentVersion: item.ContentVersion, TodoState: item.TodoState, RemindAt: item.RemindAt}
}

func compactAttentionItem(item AttentionItem) AttentionItem {
	var titleTruncated, summaryTruncated bool
	item.Title, titleTruncated = compactContextText(item.Title, attentionTitleMaxBytes)
	item.Summary, summaryTruncated = compactContextText(item.Summary, attentionSummaryMaxBytes)
	interests := append([]ItemInterest(nil), item.Interests...)
	reasonTruncated := false
	for index := range interests {
		var clipped bool
		interests[index].Reason, clipped = compactContextText(interests[index].Reason, attentionReasonMaxBytes)
		reasonTruncated = reasonTruncated || clipped
	}
	var truncatedFields []string
	if titleTruncated {
		truncatedFields = append(truncatedFields, "title")
	}
	if summaryTruncated {
		truncatedFields = append(truncatedFields, "summary")
	}
	if reasonTruncated {
		truncatedFields = append(truncatedFields, "interest_reason")
	}
	item.Interests = interests
	item.TruncatedFields = truncatedFields
	return item
}

func compactContextText(value string, maxBytes int) (string, bool) {
	if len(value) <= maxBytes {
		return value, false
	}
	limit := maxBytes - len("…")
	cut := 0
	for index := range value {
		if index > limit {
			break
		}
		cut = index
	}
	return value[:cut] + "…", true
}

func contextSnapshot(ctx context.Context, db *sql.Tx, settings settingsRow, now int64) (runContextSnapshot, error) {
	interests, err := activeInterests(ctx, db)
	if err != nil {
		return runContextSnapshot{}, err
	}
	attention, err := attentionItems(ctx, db, now)
	if err != nil {
		return runContextSnapshot{}, err
	}
	return runContextSnapshot{Interests: interests, Attention: attention, Contexts: map[string]string{"AGENTS.md": settings.agentsMD, "USER.md": settings.userMD}}, nil
}

func contextPage[T any](runID, collection string, items []T, offset, pageSize int) (map[string]any, error) {
	return contextPageProjected(runID, collection, items, offset, pageSize, nil)
}

func contextPageProjected[T any](runID, collection string, items []T, offset, pageSize int, project func(T) T) (map[string]any, error) {
	if offset < 0 || offset > len(items) {
		return nil, Invalid("Continuation cursor is not in this collection")
	}
	end := min(offset+pageSize, len(items))
	pageItems := items[offset:end]
	if project != nil {
		pageItems = make([]T, end-offset)
		for index := range pageItems {
			pageItems[index] = project(items[offset+index])
		}
	}
	if end > offset+1 {
		encoded, err := json.Marshal(pageItems)
		if err != nil {
			return nil, err
		}
		if len(encoded) > contextPageBytes {
			low, high := offset+1, end-1
			for low < high {
				middle := low + (high-low+1)/2
				encoded, err = json.Marshal(pageItems[:middle-offset])
				if err != nil {
					return nil, err
				}
				if len(encoded) <= contextPageBytes {
					low = middle
				} else {
					high = middle - 1
				}
			}
			end = low
		}
	}
	result := map[string]any{"collection": collection, "items": pageItems[:end-offset]}
	if end < len(items) {
		result["next_cursor"] = encodeContextCursor(contextCursor{RunID: runID, Collection: collection, Offset: end})
	}
	return result, nil
}

func firstContextPages(snapshot runContextSnapshot, runID string) (map[string]any, error) {
	interests, err := contextPage(runID, "interests", snapshot.Interests, 0, contextPageSize)
	if err != nil {
		return nil, err
	}
	attention, err := contextPageProjected(runID, "attention_items", snapshot.Attention, 0, contextPageSize, compactAttentionItem)
	if err != nil {
		return nil, err
	}
	brief := map[string]any{
		"interests":       interests["items"],
		"attention_items": attention["items"],
		"contexts":        snapshot.Contexts,
		"continuations":   map[string]any{},
	}
	continuations := brief["continuations"].(map[string]any)
	if cursor, ok := interests["next_cursor"]; ok {
		continuations["interests"] = cursor
	}
	if cursor, ok := attention["next_cursor"]; ok {
		continuations["attention_items"] = cursor
	}
	return brief, nil
}

// The receipt keeps the full captured text. Its cursor still follows the
// bounded output page, so a retry has the same continuation boundary.
func firstStoredContextPages(snapshot runContextSnapshot, runID string) (map[string]any, error) {
	brief, err := firstContextPages(snapshot, runID)
	if err != nil {
		return nil, err
	}
	count := len(brief["attention_items"].([]AttentionItem))
	brief["attention_items"] = snapshot.Attention[:count]
	return brief, nil
}

func compactSelectedWatches(watches []SelectedWatch) []SelectedWatch {
	output := make([]SelectedWatch, len(watches))
	for index, watch := range watches {
		output[index] = watch
		if watch.RelatedItems == nil {
			continue
		}
		output[index].RelatedItems = make([]AttentionItem, len(watch.RelatedItems))
		for itemIndex, item := range watch.RelatedItems {
			output[index].RelatedItems[itemIndex] = compactAttentionItem(item)
		}
	}
	return output
}

func compactStartRunResponse(raw json.RawMessage) (json.RawMessage, error) {
	var response map[string]json.RawMessage
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	var brief map[string]json.RawMessage
	if err := json.Unmarshal(response["brief"], &brief); err != nil {
		return nil, err
	}
	var attention []AttentionItem
	if err := json.Unmarshal(brief["attention_items"], &attention); err != nil {
		return nil, err
	}
	for index := range attention {
		attention[index] = compactAttentionItem(attention[index])
	}
	encoded, err := json.Marshal(attention)
	if err != nil {
		return nil, err
	}
	brief["attention_items"] = encoded
	var watches []SelectedWatch
	if err := json.Unmarshal(brief["watches"], &watches); err != nil {
		return nil, err
	}
	encoded, err = json.Marshal(compactSelectedWatches(watches))
	if err != nil {
		return nil, err
	}
	brief["watches"] = encoded
	encoded, err = json.Marshal(brief)
	if err != nil {
		return nil, err
	}
	response["brief"] = encoded
	return json.Marshal(response)
}

func loadRunContext(ctx context.Context, db querier, runID string) (runContextSnapshot, error) {
	canonical, err := resolveID(ctx, db, "runs", runID)
	if err != nil {
		return runContextSnapshot{}, err
	}
	var raw string
	if err = db.QueryRowContext(ctx, "SELECT context_snapshot FROM runs WHERE id=?", canonical).Scan(&raw); err != nil {
		return runContextSnapshot{}, missing(err)
	}
	var snapshot runContextSnapshot
	if err = json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return runContextSnapshot{}, err
	}
	return snapshot, nil
}

func (a *App) StartRun(ctx context.Context, input StartRun) (json.RawMessage, error) {
	raw, err := a.mutate(ctx, input.RequestID, "POST /runs", input, func(tx *sql.Tx) (any, error) {
		if input.RunnerLabel == "" {
			input.RunnerLabel = "agent"
		}
		now := a.Now().UTC()
		nowMillis := now.UnixMilli()
		var active string
		err := tx.QueryRowContext(ctx, `SELECT id FROM runs WHERE status='running'`).Scan(&active)
		if err == nil {
			return nil, &Error{Status: 409, Code: "run_in_progress", Message: "Another run is already active.", Retryable: true, Details: map[string]any{"run_id": active}}
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
		settings, err := readSettings(ctx, tx)
		if err != nil {
			return nil, err
		}
		snapshot, err := contextSnapshot(ctx, tx, settings, nowMillis)
		if err != nil {
			return nil, err
		}
		snapshotJSON, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		id := uuid.NewString()
		selectedJSON, _ := json.Marshal(selected)
		// The legacy schema still requires lease_expires_at. Runs no longer expire
		// automatically because the heartbeat contract has no renewal step.
		_, err = tx.ExecContext(ctx, `INSERT INTO runs(id,runner_label,status,started_at,lease_expires_at,selected_watches,after_seq,through_seq,context_snapshot) VALUES(?,?,'running',?,?,?,?,?,?)`, id, input.RunnerLabel, nowMillis, 0, string(selectedJSON), after, through, string(snapshotJSON))
		if err != nil {
			return nil, err
		}
		run, err := getRun(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		brief, err := firstStoredContextPages(snapshot, id)
		if err != nil {
			return nil, err
		}
		brief["watches"] = selected
		brief["after_seq"] = after
		brief["through_seq"] = through
		brief["more_due_count"] = more
		changes, nextChanges, err := eventsPage(ctx, tx, after, through, "", contextPageSize)
		if err != nil {
			return nil, err
		}
		brief["changes"] = changes
		if nextChanges != "" {
			brief["changes_next_cursor"] = nextChanges
		}
		return map[string]any{"run": run, "brief": brief}, nil
	})
	if err != nil {
		return nil, err
	}
	return compactStartRunResponse(raw)
}

func liveRun(run Run) error {
	if run.Status != "running" {
		return &Error{Status: 409, Code: "run_finished", Message: "Run is no longer active"}
	}
	return nil
}

func (a *App) FinishRun(ctx context.Context, id string, input FinishRun) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /runs/"+id+"/finish", input, func(tx *sql.Tx) (any, error) {
		run, err := getRun(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		id = run.ID
		now := a.Now().UTC()
		if err = liveRun(run); err != nil {
			return nil, err
		}
		var success, partial int
		covered := map[string]bool{}
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
		resultRows, err := tx.QueryContext(ctx, `SELECT watch_id FROM watch_results WHERE run_id=?`, id)
		if err != nil {
			return nil, err
		}
		for resultRows.Next() {
			var watchID string
			if err = resultRows.Scan(&watchID); err != nil {
				resultRows.Close()
				return nil, err
			}
			covered[watchID] = true
		}
		if err = resultRows.Err(); err != nil {
			resultRows.Close()
			return nil, err
		}
		resultRows.Close()
		missingWatches := []string{}
		for _, selected := range run.SelectedWatches {
			if !covered[selected.ID] {
				missingWatches = append(missingWatches, selected.ID)
			}
		}
		if len(missingWatches) > 0 {
			return nil, &Error{Status: 409, Code: "watch_coverage_missing", Message: "Every selected Watch needs a terminal result before the run can finish", Retryable: true, Details: map[string]any{"watch_ids": missingWatches}}
		}
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

// AbandonRun releases a run that its external agent can no longer finish.
// Submitted results and successful Watch checkpoints remain durable; the
// captured event range is deliberately left unacknowledged for the next run.
func (a *App) AbandonRun(ctx context.Context, id string, input AbandonRun) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /runs/"+id+"/abandon", input, func(tx *sql.Tx) (any, error) {
		reason := strings.TrimSpace(input.Reason)
		if len(reason) == 0 || len(reason) > 500 {
			return nil, Invalid("reason must contain 1 to 500 characters")
		}
		run, err := getRun(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if err = liveRun(run); err != nil {
			return nil, err
		}
		summary := "Abandoned by owner: " + reason
		_, err = tx.ExecContext(ctx, `UPDATE runs SET status='failed',ended_at=?,summary=? WHERE id=?`, a.Now().UTC().UnixMilli(), summary, run.ID)
		if err != nil {
			return nil, err
		}
		if err = a.event(ctx, tx, "user", "run", run.ID, "run.abandoned", map[string]any{"reason": reason}); err != nil {
			return nil, err
		}
		return getRun(ctx, tx, run.ID)
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
	return eventsBetween(ctx, a.Store.DB, after, through)
}

func encodeEventCursor(seq int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(seq, 10)))
}

func decodeEventCursor(cursor string) (int64, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, Invalid("Invalid continuation cursor")
	}
	seq, err := strconv.ParseInt(string(decoded), 10, 64)
	if err != nil || seq < 0 {
		return 0, Invalid("Invalid continuation cursor")
	}
	return seq, nil
}

func eventsPage(ctx context.Context, db rowsQuerier, after, through int64, cursor string, limit int) ([]Event, string, error) {
	if after < 0 || through < after {
		return nil, "", Invalid("expected 0 <= after_seq <= through_seq")
	}
	if limit < 1 || limit > 100 {
		return nil, "", Invalid("limit must be between 1 and 100")
	}
	start := after
	if cursor != "" {
		decoded, err := decodeEventCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		if decoded <= after || decoded > through {
			return nil, "", Invalid("Continuation cursor is not in this change range")
		}
		start = decoded
	}
	rows, err := db.QueryContext(ctx, `SELECT seq,occurred_at,actor,entity_type,entity_id,change_type,payload FROM events WHERE seq>? AND seq<=? AND (actor='user' OR entity_type='proposal') ORDER BY seq LIMIT ?`, start, through, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []Event{}
	for rows.Next() {
		var item Event
		var occurred int64
		var raw string
		if err = rows.Scan(&item.Seq, &occurred, &item.Actor, &item.EntityType, &item.EntityID, &item.ChangeType, &raw); err != nil {
			return nil, "", err
		}
		item.OccurredAt = time.UnixMilli(occurred).UTC()
		item.Payload = json.RawMessage(raw)
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, "", err
	}
	if len(items) <= limit {
		return items, "", nil
	}
	items = items[:limit]
	return items, encodeEventCursor(items[len(items)-1].Seq), nil
}

func (a *App) ChangesPage(ctx context.Context, after, through int64, cursor string, limit int) (map[string]any, error) {
	items, next, err := eventsPage(ctx, a.Store.DB, after, through, cursor, limit)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"items": items}
	if next != "" {
		result["next_cursor"] = next
	}
	return result, nil
}

func eventsBetween(ctx context.Context, db rowsQuerier, after, through int64) ([]Event, error) {
	rows, err := db.QueryContext(ctx, `SELECT seq,occurred_at,actor,entity_type,entity_id,change_type,payload FROM events WHERE seq>? AND seq<=? AND (actor='user' OR entity_type='proposal') ORDER BY seq`, after, through)
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
	return a.BriefPage(ctx, "")
}

func (a *App) BriefPage(ctx context.Context, cursor string) (map[string]any, error) {
	if cursor != "" {
		decoded, err := decodeContextCursor(cursor)
		if err != nil {
			return nil, err
		}
		var snapshot runContextSnapshot
		if decoded.RunID != "" {
			snapshot, err = loadRunContext(ctx, a.Store.DB, decoded.RunID)
		} else {
			var tx *sql.Tx
			tx, err = a.Store.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
			if err == nil {
				settings, settingsErr := readSettings(ctx, tx)
				if settingsErr == nil {
					snapshot, err = contextSnapshot(ctx, tx, settings, a.Now().UTC().UnixMilli())
				} else {
					err = settingsErr
				}
				_ = tx.Rollback()
			}
		}
		if err != nil {
			return nil, err
		}
		var page map[string]any
		switch decoded.Collection {
		case "interests":
			page, err = contextPage(decoded.RunID, decoded.Collection, snapshot.Interests, decoded.Offset, contextContinuationPageSize)
		case "attention_items":
			page, err = contextPageProjected(decoded.RunID, decoded.Collection, snapshot.Attention, decoded.Offset, contextContinuationPageSize, compactAttentionItem)
		}
		if err != nil {
			return nil, err
		}
		if decoded.RunID != "" {
			page["run_id"] = decoded.RunID
		}
		return page, nil
	}

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
	settings, err := readSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	snapshot, err := contextSnapshot(ctx, tx, settings, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	changes, nextChanges, err := eventsPage(ctx, tx, after, through, "", contextPageSize)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	brief, err := firstContextPages(snapshot, "")
	if err != nil {
		return nil, err
	}
	brief["watches"] = compactSelectedWatches(selected)
	brief["more_due_count"] = more
	brief["changes"] = map[string]int64{"after_seq": after, "through_seq": through}
	brief["changes_items"] = changes
	if nextChanges != "" {
		brief["changes_next_cursor"] = nextChanges
	}
	health, err := a.OperationalHealth(ctx)
	if err != nil {
		return nil, err
	}
	brief["now"] = now
	brief["health"] = health
	return brief, nil
}

type WatchHealth struct {
	WatchID       string     `json:"watch_id"`
	NextDueAt     time.Time  `json:"next_due_at"`
	DueReason     string     `json:"due_reason"`
	LastStatus    string     `json:"last_status,omitempty"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}
type ActiveRunHealth struct {
	ID             string    `json:"id"`
	StartedAt      time.Time `json:"started_at"`
	SelectedCount  int       `json:"selected_count"`
	SubmittedCount int       `json:"submitted_count"`
}
type LastRunHealth struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Summary   string     `json:"summary,omitempty"`
}

type ItemCounts struct {
	All       int `json:"all"`
	Attention int `json:"attention"`
	Todo      int `json:"todo"`
}

type SetupStatus struct {
	UserContextSet bool `json:"user_context_set"`
	HasInterest    bool `json:"has_interest"`
	HasWatcher     bool `json:"has_watcher"`
	Done           bool `json:"done"`
}

func (a *App) OperationalHealth(ctx context.Context) (map[string]any, error) {
	var lastRun any
	var activeRun any
	run, err := scanRun(a.Store.DB.QueryRowContext(ctx, "SELECT "+runColumns+" FROM runs ORDER BY started_at DESC,rowid DESC LIMIT 1"))
	if err == nil {
		lastRun = LastRunHealth{ID: run.ID, Status: run.Status, StartedAt: run.StartedAt, EndedAt: run.EndedAt, Summary: run.Summary}
	} else {
		var problem *Error
		if !errors.As(err, &problem) || problem.Status != 404 {
			return nil, err
		}
	}
	run, err = scanRun(a.Store.DB.QueryRowContext(ctx, "SELECT "+runColumns+" FROM runs WHERE status='running'"))
	if err == nil {
		var submitted int
		if err = a.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM watch_results WHERE run_id=?`, run.ID).Scan(&submitted); err != nil {
			return nil, err
		}
		activeRun = ActiveRunHealth{ID: run.ID, StartedAt: run.StartedAt, SelectedCount: len(run.SelectedWatches), SubmittedCount: submitted}
	} else {
		var problem *Error
		if !errors.As(err, &problem) || problem.Status != 404 {
			return nil, err
		}
	}
	rows, err := a.Store.DB.QueryContext(ctx, `SELECT w.id,w.next_due_at,w.state,w.valid_until,
  (SELECT count(*) FROM interests i WHERE i.state='active' AND
   (w.matching_policy='broad' OR EXISTS (SELECT 1 FROM watch_interests wi WHERE wi.watch_id=w.id AND wi.interest_id=i.id))),
  (SELECT status FROM watch_results wr WHERE wr.watch_id=w.id ORDER BY recorded_at DESC,run_id DESC LIMIT 1),
  (SELECT recorded_at FROM watch_results wr WHERE wr.watch_id=w.id ORDER BY recorded_at DESC,run_id DESC LIMIT 1),
  (SELECT MAX(recorded_at) FROM watch_results wr WHERE wr.watch_id=w.id AND status='success'),
  (SELECT json_extract(result,'$.error') FROM watch_results wr WHERE wr.watch_id=w.id ORDER BY recorded_at DESC,run_id DESC LIMIT 1)
	 FROM watches w ORDER BY w.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	watches := []WatchHealth{}
	dueCount := 0
	now := a.Now().UTC()
	hasWatcher := false
	for rows.Next() {
		var item WatchHealth
		var nextDue int64
		var state string
		var validUntil sql.NullInt64
		var interestCount int
		var status, message sql.NullString
		var attempt, success sql.NullInt64
		if err = rows.Scan(&item.WatchID, &nextDue, &state, &validUntil, &interestCount, &status, &attempt, &success, &message); err != nil {
			return nil, err
		}
		item.NextDueAt = time.UnixMilli(nextDue).UTC()
		if state == "active" && (!validUntil.Valid || validUntil.Int64 > now.UnixMilli()) && interestCount > 0 {
			hasWatcher = true
		}
		switch {
		case state != "active":
			item.DueReason = "watcher_" + state
		case validUntil.Valid && validUntil.Int64 <= now.UnixMilli():
			item.DueReason = "expired"
		case interestCount == 0:
			item.DueReason = "no_active_interest"
		case item.NextDueAt.After(now):
			item.DueReason = "scheduled_later"
		case status.String == "partial" || status.String == "failed":
			item.DueReason = "retry_after_" + status.String
		default:
			item.DueReason = "scheduled"
		}
		if item.DueReason == "scheduled" || strings.HasPrefix(item.DueReason, "retry_after_") {
			dueCount++
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
	if err = rows.Close(); err != nil {
		return nil, err
	}
	settings, err := readSettings(ctx, a.Store.DB)
	if err != nil {
		return nil, err
	}
	var activeInterests int
	if err = a.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM interests WHERE state='active'").Scan(&activeInterests); err != nil {
		return nil, err
	}
	setup := SetupStatus{UserContextSet: strings.TrimSpace(settings.userMD) != "" && settings.userMD != settings.defaultUserMD, HasInterest: activeInterests > 0, HasWatcher: hasWatcher}
	setup.Done = setup.UserContextSet && setup.HasInterest && setup.HasWatcher
	var counts ItemCounts
	query := `SELECT count(*),
count(*) FILTER (WHERE ` + attentionPredicate + `),
count(*) FILTER (WHERE todo_state='todo') FROM items`
	if err = a.Store.DB.QueryRowContext(ctx, query, now.UnixMilli()).Scan(&counts.All, &counts.Attention, &counts.Todo); err != nil {
		return nil, err
	}
	return map[string]any{"last_run": lastRun, "active_run": activeRun, "due_count": dueCount, "watches": watches, "item_counts": counts, "setup": setup}, nil
}
