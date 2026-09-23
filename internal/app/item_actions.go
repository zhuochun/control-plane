package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type Action struct {
	Type           string `json:"type"`
	State          string `json:"state,omitempty"`
	ContentVersion int64  `json:"content_version,omitempty"`
	Date           string `json:"date,omitempty"`
	Time           string `json:"time,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	UTCOffset      string `json:"utc_offset,omitempty"`
}
type ApplyItemAction struct {
	RequestID            string `json:"request_id"`
	ExpectedStateVersion int64  `json:"expected_state_version"`
	Action               Action `json:"action"`
}
type SetUserNote struct {
	RequestID            string `json:"request_id"`
	ExpectedStateVersion int64  `json:"expected_state_version"`
	UserNote             string `json:"user_note"`
}

func stateConflict(item Item) error {
	return &Error{Status: 409, Code: "state_conflict", Message: "The item state changed. Read it again before applying this action.", Details: map[string]any{"current_state_version": item.StateVersion}}
}

func (a *App) ApplyItemAction(ctx context.Context, id string, input ApplyItemAction) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "POST /items/"+id+"/actions", input, func(tx *sql.Tx) (any, error) {
		item, err := getItem(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		id = item.ID
		if input.ExpectedStateVersion != item.StateVersion {
			return nil, stateConflict(item)
		}
		var remindAt any
		if item.RemindAt != nil {
			remindAt = item.RemindAt.UnixMilli()
		}
		var timezone any
		if item.ReminderTimezone != nil {
			timezone = *item.ReminderTimezone
		}
		changed := true
		switch input.Action.Type {
		case "set_todo":
			if input.Action.State != "none" && input.Action.State != "todo" && input.Action.State != "done" {
				return nil, Invalid("set_todo needs state none, todo, or done")
			}
			changed = item.TodoState != input.Action.State
			item.TodoState = input.Action.State
			if item.TodoState == "done" {
				changed = changed || item.RemindAt != nil
				remindAt = nil
				timezone = nil
			}
		case "clear_reminder":
			changed = item.RemindAt != nil
			remindAt = nil
			timezone = nil
		case "acknowledge":
			if input.Action.ContentVersion < 1 || input.Action.ContentVersion > item.ContentVersion {
				return nil, Invalid("acknowledgement must name a current or earlier content version")
			}
			changed = input.Action.ContentVersion > item.AcknowledgedContentVersion
			if changed {
				item.AcknowledgedContentVersion = input.Action.ContentVersion
			}
		case "set_reminder":
			instant, candidates, err := reminderInstant(input.Action)
			if err != nil {
				return nil, err
			}
			if len(candidates) > 1 && input.Action.UTCOffset == "" {
				return nil, &Error{Status: 422, Code: "ambiguous_local_time", Message: "Choose which occurrence of this local time to use.", Details: map[string]any{"utc_offsets": candidates}}
			}
			changed = item.RemindAt == nil || !item.RemindAt.Equal(instant) || item.ReminderTimezone == nil || *item.ReminderTimezone != input.Action.Timezone
			remindAt = instant.UnixMilli()
			timezone = input.Action.Timezone
		default:
			return nil, Invalid("unknown item action")
		}
		if !changed {
			return item, nil
		}
		now := a.Now().UTC().UnixMilli()
		_, err = tx.ExecContext(ctx, `UPDATE items SET todo_state=?,remind_at=?,reminder_timezone=?,acknowledged_content_version=?,state_version=state_version+1,state_updated_at=? WHERE id=?`, item.TodoState, remindAt, timezone, item.AcknowledgedContentVersion, now, id)
		if err != nil {
			return nil, err
		}
		item, err = getItem(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return item, a.event(ctx, tx, "user", "item", id, "item.state_updated", map[string]any{"action": input.Action, "state_version": item.StateVersion})
	})
}

func reminderInstant(action Action) (time.Time, []string, error) {
	if action.Date == "" || action.Timezone == "" {
		return time.Time{}, nil, Invalid("set_reminder needs date and timezone")
	}
	clock := action.Time
	if clock == "" {
		clock = "09:00"
	}
	local, err := time.Parse("2006-01-02 15:04", action.Date+" "+clock)
	if err != nil {
		return time.Time{}, nil, Invalid("date and time must use YYYY-MM-DD and HH:MM")
	}
	location, err := time.LoadLocation(action.Timezone)
	if err != nil {
		return time.Time{}, nil, Invalid("timezone must be a valid IANA name")
	}
	type candidate struct {
		instant time.Time
		offset  string
	}
	found := []candidate{}
	for seconds := -14 * 3600; seconds <= 14*3600; seconds += 15 * 60 {
		instant := local.Add(-time.Duration(seconds) * time.Second).UTC()
		shown := instant.In(location)
		_, actual := shown.Zone()
		if actual == seconds && shown.Year() == local.Year() && shown.Month() == local.Month() && shown.Day() == local.Day() && shown.Hour() == local.Hour() && shown.Minute() == local.Minute() {
			found = append(found, candidate{instant, formatOffset(seconds)})
		}
	}
	if len(found) == 0 {
		return time.Time{}, nil, &Error{Status: 422, Code: "nonexistent_local_time", Message: "That local time does not exist in the selected timezone."}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].instant.Before(found[j].instant) })
	offsets := make([]string, len(found))
	for i, c := range found {
		offsets[i] = c.offset
	}
	if action.UTCOffset == "" {
		return found[0].instant, offsets, nil
	}
	for _, c := range found {
		if c.offset == action.UTCOffset {
			return c.instant, offsets, nil
		}
	}
	return time.Time{}, nil, Invalid("utc_offset is not valid for that timezone and local time")
}
func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	return fmt.Sprintf("%s%02d:%02d", sign, seconds/3600, (seconds%3600)/60)
}

func (a *App) SetUserNote(ctx context.Context, id string, input SetUserNote) (json.RawMessage, error) {
	return a.mutate(ctx, input.RequestID, "PUT /items/"+id+"/note", input, func(tx *sql.Tx) (any, error) {
		item, err := getItem(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		id = item.ID
		if input.ExpectedStateVersion != item.StateVersion {
			return nil, stateConflict(item)
		}
		now := a.Now().UTC().UnixMilli()
		_, err = tx.ExecContext(ctx, `UPDATE items SET user_note=?,state_version=state_version+1,state_updated_at=? WHERE id=?`, input.UserNote, now, id)
		if err != nil {
			return nil, err
		}
		item, err = getItem(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return item, a.event(ctx, tx, "user", "item", id, "item.note_updated", map[string]any{"state_version": item.StateVersion})
	})
}

type ItemVersion struct {
	ContentVersion int64           `json:"content_version"`
	Snapshot       json.RawMessage `json:"snapshot"`
}

func (a *App) ItemHistory(ctx context.Context, id string) ([]ItemVersion, error) {
	item, err := a.Item(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := a.Store.DB.QueryContext(ctx, `SELECT content_version,snapshot FROM item_versions WHERE item_id=? ORDER BY content_version DESC LIMIT 20`, item.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ItemVersion{}
	for rows.Next() {
		var item ItemVersion
		var raw string
		if err = rows.Scan(&item.ContentVersion, &raw); err != nil {
			return nil, err
		}
		item.Snapshot = json.RawMessage(raw)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) ItemContext(ctx context.Context, id string) (map[string]any, error) {
	item, err := a.Item(ctx, id)
	if err != nil {
		return nil, err
	}
	history, err := a.ItemHistory(ctx, id)
	if err != nil {
		return nil, err
	}
	var count int
	if err = a.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM item_versions WHERE item_id=?`, item.ID).Scan(&count); err != nil {
		return nil, err
	}
	return map[string]any{"item": item, "history": history, "truncated": count > len(history), "limits": map[string]int{"history_items": 20}, "links": map[string]string{"item": "/api/v1/items/" + item.ID, "history": "/api/v1/items/" + item.ID + "/history"}}, nil
}
