package app

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"
)

const briefPageSize = 20
const eligibleWatcher = `w.state='active' AND (w.valid_until IS NULL OR w.valid_until>?) AND EXISTS (
 SELECT 1 FROM interests i WHERE i.state='active' AND (w.matching_policy='broad' OR EXISTS
 (SELECT 1 FROM watch_interests wi WHERE wi.watch_id=w.id AND wi.interest_id=i.id)))`
const latestCurrentResult = `(SELECT wr.rowid FROM watch_results wr JOIN runs r ON r.id=wr.run_id
 WHERE wr.watch_id=w.id AND EXISTS (SELECT 1 FROM json_each(r.selected_watches) sw
 WHERE json_extract(sw.value,'$.id')=w.id AND json_extract(sw.value,'$.source_generation')=w.source_generation)
 ORDER BY wr.recorded_at DESC,r.started_at DESC,r.rowid DESC LIMIT 1)`

type focusHandle struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Revision int64  `json:"revision"`
}
type watcherHandle struct {
	ID               string     `json:"id"`
	Slug             string     `json:"slug"`
	State            string     `json:"state"`
	SourceGeneration int64      `json:"source_generation"`
	InterestIDs      []string   `json:"interest_ids,omitempty"`
	NextDueAt        time.Time  `json:"next_due_at"`
	DueReason        string     `json:"due_reason,omitempty"`
	LastStatus       string     `json:"last_status,omitempty"`
	LastAttemptAt    *time.Time `json:"last_attempt_at,omitempty"`
	Limitation       string     `json:"limitation,omitempty"`
	TruncatedFields  []string   `json:"truncated_fields,omitempty"`
}

func liveBriefCollection(ctx context.Context, tx *sql.Tx, collection string, offset int, now int64) (map[string]any, error) {
	result := map[string]any{"collection": collection, "consistency": "live"}
	var count int
	switch collection {
	case "focus":
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM interests WHERE state='active'").Scan(&count); err != nil {
			return nil, err
		}
		rows, err := tx.QueryContext(ctx, "SELECT id,slug,title,revision FROM interests WHERE state='active' ORDER BY created_at,id LIMIT ? OFFSET ?", briefPageSize, offset)
		if err != nil {
			return nil, err
		}
		items := []focusHandle{}
		for rows.Next() {
			var item focusHandle
			if err = rows.Scan(&item.ID, &item.Slug, &item.Title, &item.Revision); err != nil {
				_ = rows.Close()
				return nil, err
			}
			items = append(items, item)
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		result["items"] = items
	case "due_watches", "unresolved_failures":
		where := "wr.status IN ('partial','failed')"
		args := []any{}
		order := "wr.recorded_at DESC,w.id"
		if collection == "due_watches" {
			where = eligibleWatcher + " AND w.next_due_at<=?"
			args = []any{now, now}
			order = "w.next_due_at,w.id"
		}
		from := " FROM watches w LEFT JOIN watch_results wr ON wr.rowid=" + latestCurrentResult + " WHERE " + where
		if err := tx.QueryRowContext(ctx, "SELECT count(*)"+from, args...).Scan(&count); err != nil {
			return nil, err
		}
		rows, err := tx.QueryContext(ctx, "SELECT w.id,w.slug,w.state,w.source_generation,w.next_due_at,wr.status,wr.recorded_at,json_extract(wr.result,'$.error')"+from+" ORDER BY "+order+" LIMIT ? OFFSET ?", append(args, briefPageSize, offset)...)
		if err != nil {
			return nil, err
		}
		items := []watcherHandle{}
		for rows.Next() {
			var item watcherHandle
			var due int64
			var status, message sql.NullString
			var attempt sql.NullInt64
			if err = rows.Scan(&item.ID, &item.Slug, &item.State, &item.SourceGeneration, &due, &status, &attempt, &message); err != nil {
				_ = rows.Close()
				return nil, err
			}
			item.NextDueAt = time.UnixMilli(due).UTC()
			item.LastStatus = status.String
			if attempt.Valid {
				value := time.UnixMilli(attempt.Int64).UTC()
				item.LastAttemptAt = &value
			}
			var clipped bool
			item.Limitation, clipped = compactContextText(message.String, attentionSummaryMaxBytes)
			if clipped {
				item.TruncatedFields = []string{"limitation"}
			}
			if collection == "due_watches" {
				item.DueReason = "scheduled"
				if status.String == "partial" || status.String == "failed" {
					item.DueReason = "retry_after_" + status.String
				}
			}
			items = append(items, item)
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if collection == "due_watches" {
			for i := range items {
				interestRows, err := tx.QueryContext(ctx, `SELECT i.id FROM interests i JOIN watches w ON w.id=? WHERE i.state='active'
     AND (w.matching_policy='broad' OR EXISTS (SELECT 1 FROM watch_interests wi WHERE wi.watch_id=w.id AND wi.interest_id=i.id)) ORDER BY i.id`, items[i].ID)
				if err != nil {
					return nil, err
				}
				items[i].InterestIDs = []string{}
				for interestRows.Next() {
					var id string
					if err = interestRows.Scan(&id); err != nil {
						_ = interestRows.Close()
						return nil, err
					}
					items[i].InterestIDs = append(items[i].InterestIDs, id)
				}
				err = interestRows.Err()
				closeErr := interestRows.Close()
				if err != nil {
					return nil, err
				}
				if closeErr != nil {
					return nil, closeErr
				}
			}
		}
		result["items"] = items
	default:
		return nil, Invalid("Invalid live brief collection")
	}
	result["count"] = count
	if offset > count {
		return nil, Invalid("Continuation cursor is not in this collection; refresh the live brief")
	}
	if offset+briefPageSize < count {
		result["next_cursor"] = encodeContextCursor(contextCursor{Collection: collection, Offset: offset + briefPageSize})
	}
	return result, nil
}

func (a *App) Brief(ctx context.Context) (map[string]any, error) { return a.BriefPage(ctx, "") }
func (a *App) BriefPage(ctx context.Context, cursor string) (map[string]any, error) {
	if cursor != "" {
		decoded, err := decodeContextCursor(cursor)
		if err != nil {
			return nil, err
		}
		if decoded.RunID != "" {
			snapshot, err := loadRunContext(ctx, a.Store.DB, decoded.RunID)
			if err != nil {
				return nil, err
			}
			var page map[string]any
			switch decoded.Collection {
			case "interests":
				run, readErr := a.Run(ctx, decoded.RunID)
				if readErr != nil {
					return nil, readErr
				}
				page, err = contextPage(decoded.RunID, decoded.Collection, requiredInterests(snapshot, run.SelectedWatches), decoded.Offset, contextContinuationPageSize)
			case "attention_items":
				page, err = contextPageProjected(decoded.RunID, decoded.Collection, snapshot.Attention, decoded.Offset, contextContinuationPageSize, compactAttentionItem)
			default:
				return nil, Invalid("Invalid captured context collection")
			}
			if err != nil {
				return nil, err
			}
			page["run_id"] = decoded.RunID
			page["consistency"] = "captured"
			return page, nil
		}
		tx, err := a.Store.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback() }()
		return liveBriefCollection(ctx, tx, decoded.Collection, decoded.Offset, a.Now().UTC().UnixMilli())
	}
	tx, err := a.Store.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	now := a.Now().UTC()
	millis := now.UnixMilli()
	result := map[string]any{"now": now, "consistency": "live", "active_run": nil, "last_run": nil}
	for _, collection := range []string{"focus", "due_watches", "unresolved_failures"} {
		page, err := liveBriefCollection(ctx, tx, collection, 0, millis)
		if err != nil {
			return nil, err
		}
		result[collection] = page
	}
	var attentionCount, reminders int
	if err = tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER (WHERE remind_at IS NOT NULL AND remind_at<=?) FROM items WHERE `+attentionPredicate, millis, millis).Scan(&attentionCount, &reminders); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+itemColumns+" FROM items WHERE "+attentionPredicate+itemOrder+" LIMIT 5", millis, millis)
	if err != nil {
		return nil, err
	}
	sample := []AttentionItem{}
	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			_ = rows.Close()
			return nil, scanErr
		}
		sample = append(sample, compactAttentionItem(attentionItem(item)))
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	result["attention"] = map[string]any{"count": attentionCount, "due_reminder_count": reminders, "sample": sample, "sample_only": true}
	var paused, expired int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FILTER (WHERE state='paused'),count(*) FILTER (WHERE valid_until<=?) FROM watches", millis).Scan(&paused, &expired); err != nil {
		return nil, err
	}
	result["watcher_counts"] = map[string]int{"paused": paused, "expired": expired}
	settings, err := readSettings(ctx, tx)
	if err != nil {
		return nil, err
	}
	var eligible int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM watches w WHERE "+eligibleWatcher, millis).Scan(&eligible); err != nil {
		return nil, err
	}
	setup := SetupStatus{UserContextSet: strings.TrimSpace(settings.userMD) != "" && settings.userMD != settings.defaultUserMD, HasInterest: result["focus"].(map[string]any)["count"].(int) > 0, HasWatcher: eligible > 0}
	setup.Done = setup.UserContextSet && setup.HasInterest && setup.HasWatcher
	result["setup"] = setup
	var afterText string
	var through int64
	if err = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='acknowledged_event_seq'").Scan(&afterText); err != nil {
		return nil, err
	}
	after, err := strconv.ParseInt(afterText, 10, 64)
	if err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM events").Scan(&through); err != nil {
		return nil, err
	}
	var pending int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE seq>? AND seq<=? AND (actor='user' OR entity_type='proposal')", after, through).Scan(&pending); err != nil {
		return nil, err
	}
	result["pending_changes"] = map[string]any{"count": pending, "after_seq": after, "through_seq": through}
	for _, active := range []bool{false, true} {
		query := "SELECT id,runner_label,status,started_at,ended_at,after_seq,through_seq,summary,json_array_length(selected_watches),(SELECT count(*) FROM watch_results wr WHERE wr.run_id=runs.id) FROM runs"
		if active {
			query += " WHERE status='running'"
		}
		query += " ORDER BY started_at DESC,rowid DESC LIMIT 1"
		item, readErr := scanRunSummary(tx.QueryRowContext(ctx, query))
		if readErr != nil {
			if problem, ok := readErr.(*Error); ok && problem.Status == 404 {
				continue
			}
			return nil, readErr
		}
		if active {
			result["active_run"] = item
		} else {
			result["last_run"] = item
		}
	}
	return result, nil
}
