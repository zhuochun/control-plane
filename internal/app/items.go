package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Source struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Label      string    `json:"label"`
	ObservedAt time.Time `json:"observed_at"`
}

type ReportAction struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Label     string `json:"label"`
	SourceRef string `json:"source_ref,omitempty"`
	State     string `json:"state,omitempty"`
}

type Report struct {
	SchemaVersion int            `json:"schema_version"`
	BodyMD        string         `json:"body_md"`
	Actions       []ReportAction `json:"actions,omitempty"`
}

type ItemContent struct {
	Sources   []Source `json:"sources"`
	ContextMD string   `json:"context_md,omitempty"`
	Report    Report   `json:"report"`
}

type Item struct {
	ID                         string     `json:"id"`
	DedupeKey                  string     `json:"dedupe_key"`
	Kind                       string     `json:"kind"`
	InterestID                 *string    `json:"interest_id"`
	WatchID                    *string    `json:"watch_id"`
	ParentID                   *string    `json:"parent_id"`
	Title                      string     `json:"title"`
	Summary                    string     `json:"summary"`
	Sources                    []Source   `json:"sources"`
	ContextMD                  string     `json:"context_md,omitempty"`
	Report                     Report     `json:"report"`
	ContentVersion             int64      `json:"content_version"`
	TodoState                  string     `json:"todo_state"`
	RemindAt                   *time.Time `json:"remind_at"`
	ReminderTimezone           *string    `json:"reminder_timezone"`
	AcknowledgedContentVersion int64      `json:"acknowledged_content_version"`
	UserNote                   string     `json:"user_note"`
	StateVersion               int64      `json:"state_version"`
	CreatedAt                  time.Time  `json:"created_at"`
	ContentUpdatedAt           time.Time  `json:"content_updated_at"`
	StateUpdatedAt             time.Time  `json:"state_updated_at"`
}

type PutItem struct {
	RequestID              string   `json:"request_id,omitempty"`
	DedupeKey              string   `json:"dedupe_key"`
	ExpectedContentVersion int64    `json:"expected_content_version"`
	Kind                   string   `json:"kind"`
	InterestID             *string  `json:"interest_id,omitempty"`
	WatchID                *string  `json:"watch_id,omitempty"`
	ParentID               *string  `json:"parent_id,omitempty"`
	Title                  string   `json:"title"`
	Summary                string   `json:"summary"`
	Sources                []Source `json:"sources"`
	ContextMD              string   `json:"context_md,omitempty"`
	Report                 Report   `json:"report"`
	InitialTodoState       string   `json:"initial_todo_state,omitempty"`
}

const itemColumns = `id,dedupe_key,kind,interest_id,watch_id,parent_id,title,summary,content,content_version,todo_state,remind_at,reminder_timezone,acknowledged_content_version,user_note,state_version,created_at,content_updated_at,state_updated_at`

func scanItem(row scanner) (Item, error) {
	var item Item
	var interest, watch, parent, timezone sql.NullString
	var reminder sql.NullInt64
	var content string
	var created, contentUpdated, stateUpdated int64
	err := row.Scan(&item.ID, &item.DedupeKey, &item.Kind, &interest, &watch, &parent, &item.Title, &item.Summary, &content, &item.ContentVersion, &item.TodoState, &reminder, &timezone, &item.AcknowledgedContentVersion, &item.UserNote, &item.StateVersion, &created, &contentUpdated, &stateUpdated)
	if err != nil {
		return item, missing(err)
	}
	if interest.Valid {
		item.InterestID = &interest.String
	}
	if watch.Valid {
		item.WatchID = &watch.String
	}
	if parent.Valid {
		item.ParentID = &parent.String
	}
	if reminder.Valid {
		value := time.UnixMilli(reminder.Int64).UTC()
		item.RemindAt = &value
	}
	if timezone.Valid {
		item.ReminderTimezone = &timezone.String
	}
	var body ItemContent
	if err = json.Unmarshal([]byte(content), &body); err != nil {
		return item, err
	}
	item.Sources, item.ContextMD, item.Report = body.Sources, body.ContextMD, body.Report
	item.CreatedAt, item.ContentUpdatedAt, item.StateUpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(contentUpdated).UTC(), time.UnixMilli(stateUpdated).UTC()
	return item, nil
}

func getItem(ctx context.Context, db querier, id string) (Item, error) {
	canonical, err := resolveID(ctx, db, "items", id)
	if err != nil {
		return Item{}, err
	}
	return scanItem(db.QueryRowContext(ctx, "SELECT "+itemColumns+" FROM items WHERE id=?", canonical))
}
func (a *App) Item(ctx context.Context, id string) (Item, error) { return getItem(ctx, a.Store.DB, id) }

func validateItem(input PutItem) error {
	if strings.TrimSpace(input.DedupeKey) == "" || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Summary) == "" {
		return Invalid("dedupe_key, title, and summary are required")
	}
	if input.ExpectedContentVersion < 0 {
		return Invalid("expected_content_version cannot be negative")
	}
	if input.Kind != "note" && input.Kind != "report" && input.Kind != "task" && input.Kind != "outcome" {
		return Invalid("kind must be note, report, task, or outcome")
	}
	if input.Report.SchemaVersion != 1 {
		return Invalid("report.schema_version must be 1")
	}
	content, err := json.Marshal(contentFor(input))
	if err != nil {
		return Invalid("item content must be valid JSON")
	}
	if len(content) > 512<<10 {
		return Invalid("item content exceeds 512 KiB")
	}
	seenSources := map[string]bool{}
	for _, source := range input.Sources {
		parsed, err := url.Parse(source.URL)
		if source.ID == "" || source.Label == "" || err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return Invalid("each source needs a unique id, label, and HTTP(S) URL")
		}
		if source.ObservedAt.IsZero() {
			return Invalid("each source needs observed_at")
		}
		if seenSources[source.ID] {
			return Invalid("source ids must be unique within an item")
		}
		seenSources[source.ID] = true
	}
	seenActions := map[string]bool{}
	for _, action := range input.Report.Actions {
		if action.ID == "" || action.Label == "" || seenActions[action.ID] {
			return Invalid("report actions need unique ids and labels")
		}
		seenActions[action.ID] = true
		switch action.Type {
		case "open_link":
			if !seenSources[action.SourceRef] || action.State != "" {
				return Invalid("open_link must reference a source")
			}
		case "set_todo":
			if action.State != "none" && action.State != "todo" && action.State != "done" {
				return Invalid("set_todo needs state none, todo, or done")
			}
		case "set_reminder", "clear_reminder", "acknowledge":
			if action.SourceRef != "" || action.State != "" {
				return Invalid("invalid report action fields")
			}
		default:
			return Invalid("unknown report action type")
		}
	}
	if input.InitialTodoState != "" && input.InitialTodoState != "none" && input.InitialTodoState != "todo" && input.InitialTodoState != "done" {
		return Invalid("initial_todo_state must be none, todo, or done")
	}
	return nil
}

func contentFor(input PutItem) ItemContent {
	return ItemContent{Sources: input.Sources, ContextMD: input.ContextMD, Report: input.Report}
}

func contentHash(input PutItem) (string, error) {
	copyInput := input
	copyInput.RequestID = ""
	copyInput.ExpectedContentVersion = 0
	copyInput.InitialTodoState = ""
	copyInput.Sources = append([]Source(nil), input.Sources...)
	for i := range copyInput.Sources {
		copyInput.Sources[i].ObservedAt = time.Time{}
	}
	sort.Slice(copyInput.Sources, func(i, j int) bool { return copyInput.Sources[i].ID < copyInput.Sources[j].ID })
	if copyInput.Report.Actions == nil {
		copyInput.Report.Actions = []ReportAction{}
	}
	data, err := json.Marshal(copyInput)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func mergeFreshness(old []Source, next []Source) []Source {
	observed := map[string]time.Time{}
	for _, source := range old {
		observed[source.ID] = source.ObservedAt
	}
	result := append([]Source(nil), next...)
	for i := range result {
		if observed[result[i].ID].After(result[i].ObservedAt) {
			result[i].ObservedAt = observed[result[i].ID]
		}
	}
	return result
}

func (a *App) PutItem(ctx context.Context, id string, input PutItem) (json.RawMessage, error) {
	operation := "POST /items"
	if id != "" {
		operation = "PUT /items/" + id
	}
	return a.mutate(ctx, input.RequestID, operation, input, func(tx *sql.Tx) (any, error) {
		return a.putItemTx(ctx, tx, id, input)
	})
}

// UpsertInterestItem is the agent-facing path for a useful finding that is
// related to an Interest but not to a selected Watch. It intentionally cannot
// advance Watch coverage.
func (a *App) UpsertInterestItem(ctx context.Context, input PutItem) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /items/interest", input, func(tx *sql.Tx) (any, error) {
		if input.InterestID == nil || strings.TrimSpace(*input.InterestID) == "" {
			return nil, Invalid("interest_id is required for an Interest-level Item")
		}
		if input.WatchID != nil {
			return nil, Invalid("Interest-level Items cannot contain watch_id")
		}
		interest, err := getInterest(ctx, tx, *input.InterestID)
		if err != nil {
			return nil, err
		}
		input.InterestID = &interest.ID
		current, lookupErr := scanItem(tx.QueryRowContext(ctx, "SELECT "+itemColumns+" FROM items WHERE dedupe_key=?", input.DedupeKey))
		if lookupErr == nil {
			if current.WatchID != nil || current.InterestID == nil || *current.InterestID != interest.ID {
				return nil, &Error{Status: 409, Code: "item_scope_conflict", Message: "The dedupe key belongs to an Item with a different Interest or Watch scope", Details: map[string]any{"id": current.ID, "interest_id": current.InterestID, "watch_id": current.WatchID}}
			}
		} else {
			var problem *Error
			if !errors.As(lookupErr, &problem) || problem.Status != 404 {
				return nil, lookupErr
			}
		}
		return a.putItemTx(ctx, tx, "", input)
	})
}

// putItemTx is shared by direct item commands and atomic Watch publication.
func (a *App) putItemTx(ctx context.Context, tx *sql.Tx, id string, input PutItem) (any, error) {
	if id != "" {
		canonical, err := resolveID(ctx, tx, "items", id)
		if err != nil {
			return nil, err
		}
		id = canonical
	}
	if err := validateItem(input); err != nil {
		return nil, err
	}
	hash, err := contentHash(input)
	if err != nil {
		return nil, err
	}
	var current Item
	current, err = scanItem(tx.QueryRowContext(ctx, "SELECT "+itemColumns+" FROM items WHERE dedupe_key=?", input.DedupeKey))
	exists := err == nil
	if err != nil {
		var problem *Error
		if !errors.As(err, &problem) || problem.Status != 404 {
			return nil, err
		}
	}
	if !exists {
		if id != "" || input.ExpectedContentVersion != 0 {
			return nil, &Error{Status: 409, Code: "content_conflict", Message: "Item does not exist at the expected version", Details: map[string]any{"current_content_version": 0}}
		}
		if input.InitialTodoState == "" {
			if input.Kind == "task" {
				input.InitialTodoState = "todo"
			} else {
				input.InitialTodoState = "none"
			}
		}
		id = uuid.NewString()
		now := a.Now().UTC().UnixMilli()
		content, _ := json.Marshal(contentFor(input))
		_, err = tx.ExecContext(ctx, `INSERT INTO items(id,dedupe_key,kind,interest_id,watch_id,parent_id,title,summary,content,content_version,content_hash,todo_state,state_version,created_at,content_updated_at,state_updated_at) VALUES(?,?,?,?,?,?,?,?,?,1,?,?,1,?,?,?)`, id, input.DedupeKey, input.Kind, input.InterestID, input.WatchID, input.ParentID, input.Title, input.Summary, string(content), hash, input.InitialTodoState, now, now, now)
		if err != nil {
			return nil, err
		}
		snapshot := map[string]any{"title": input.Title, "summary": input.Summary, "kind": input.Kind, "content": contentFor(input)}
		snap, _ := json.Marshal(snapshot)
		if _, err = tx.ExecContext(ctx, `INSERT INTO item_versions(item_id,content_version,snapshot) VALUES(?,1,?)`, id, string(snap)); err != nil {
			return nil, err
		}
		current, err = getItem(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return current, a.event(ctx, tx, "agent", "item", id, "item.created", map[string]any{"content_version": 1})
	}
	if id != "" && id != current.ID {
		return nil, &Error{Status: 409, Code: "content_conflict", Message: "dedupe_key belongs to another item", Details: map[string]any{"id": current.ID, "current_content_version": current.ContentVersion}}
	}
	if input.ExpectedContentVersion != current.ContentVersion {
		return nil, &Error{Status: 409, Code: "content_conflict", Message: "Item content changed. Read it again before saving.", Details: map[string]any{"id": current.ID, "current_content_version": current.ContentVersion}}
	}
	if input.InitialTodoState != "" {
		return nil, Invalid("initial_todo_state is accepted only when creating an item")
	}
	var currentHash string
	if err = tx.QueryRowContext(ctx, "SELECT content_hash FROM items WHERE id=?", current.ID).Scan(&currentHash); err != nil {
		return nil, err
	}
	if hash == currentHash {
		input.Sources = mergeFreshness(current.Sources, input.Sources)
		content, _ := json.Marshal(contentFor(input))
		if _, err = tx.ExecContext(ctx, "UPDATE items SET content=? WHERE id=?", string(content), current.ID); err != nil {
			return nil, err
		}
		return getItem(ctx, tx, current.ID)
	}
	now := a.Now().UTC().UnixMilli()
	version := current.ContentVersion + 1
	content, _ := json.Marshal(contentFor(input))
	snapshot := map[string]any{"title": input.Title, "summary": input.Summary, "kind": input.Kind, "content": contentFor(input)}
	snap, _ := json.Marshal(snapshot)
	_, err = tx.ExecContext(ctx, `UPDATE items SET kind=?,interest_id=?,watch_id=?,parent_id=?,title=?,summary=?,content=?,content_version=?,content_hash=?,content_updated_at=? WHERE id=?`, input.Kind, input.InterestID, input.WatchID, input.ParentID, input.Title, input.Summary, string(content), version, hash, now, current.ID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO item_versions(item_id,content_version,snapshot) VALUES(?,?,?)`, current.ID, version, string(snap)); err != nil {
		return nil, err
	}
	current, err = getItem(ctx, tx, current.ID)
	if err != nil {
		return nil, err
	}
	return current, a.event(ctx, tx, "agent", "item", current.ID, "item.content_updated", map[string]any{"content_version": version})
}

type ItemFilters struct{ View, Kind, InterestID, WatchID, Query, DedupeKey string }

func (a *App) Items(ctx context.Context, filter ItemFilters) ([]Item, error) {
	query := "SELECT " + itemColumns + " FROM items WHERE 1=1"
	args := []any{}
	if filter.InterestID != "" {
		canonical, err := resolveID(ctx, a.Store.DB, "interests", filter.InterestID)
		if err != nil {
			return nil, err
		}
		filter.InterestID = canonical
	}
	if filter.WatchID != "" {
		canonical, err := resolveID(ctx, a.Store.DB, "watches", filter.WatchID)
		if err != nil {
			return nil, err
		}
		filter.WatchID = canonical
	}
	if filter.Kind != "" {
		query += " AND kind=?"
		args = append(args, filter.Kind)
	}
	if filter.InterestID != "" {
		query += " AND interest_id=?"
		args = append(args, filter.InterestID)
	}
	if filter.WatchID != "" {
		query += " AND watch_id=?"
		args = append(args, filter.WatchID)
	}
	if filter.DedupeKey != "" {
		query += " AND dedupe_key=?"
		args = append(args, filter.DedupeKey)
	}
	if filter.Query != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(filter.Query)
		query += ` AND (title LIKE ? ESCAPE '\' OR summary LIKE ? ESCAPE '\' OR json_extract(content,'$.context_md') LIKE ? ESCAPE '\')`
		pattern := "%" + escaped + "%"
		args = append(args, pattern, pattern, pattern)
	}
	switch filter.View {
	case "attention":
		query += " AND (todo_state='todo' OR acknowledged_content_version<content_version OR (remind_at IS NOT NULL AND remind_at<=?))"
		args = append(args, a.Now().UTC().UnixMilli())
	case "todo":
		query += " AND todo_state='todo'"
	case "done":
		query += " AND todo_state='done'"
	case "", "all":
	default:
		return nil, Invalid("view must be attention, todo, done, or all")
	}
	query += ` ORDER BY CASE WHEN remind_at IS NOT NULL AND remind_at<=? THEN 0 WHEN todo_state='todo' THEN 1 ELSE 2 END, COALESCE(remind_at,content_updated_at),content_updated_at DESC,id`
	args = append(args, a.Now().UTC().UnixMilli())
	rows, err := a.Store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
