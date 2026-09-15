package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestItemContentAndLocalStateStayIndependent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	a := New(s)
	a.Now = func() time.Time { return now }
	observed := time.Date(2026, 9, 14, 20, 0, 0, 0, time.UTC)
	input := PutItem{RequestID: "create", DedupeKey: "slack:team:channel:thread:1", ExpectedContentVersion: 0, Kind: "report", Title: "Choose rollout order", Summary: "A decision is still needed.", Sources: []Source{{ID: "thread", URL: "https://example.slack.com/thread/1", Label: "Discussion", ObservedAt: observed}}, ContextMD: "The rollout order remains open.", Report: Report{SchemaVersion: 1, BodyMD: "## Decision\nRead the options.", Actions: []ReportAction{{ID: "open", Type: "open_link", Label: "Open thread", SourceRef: "thread"}}}}
	raw, err := a.PutItem(ctx, "", input)
	if err != nil {
		t.Fatal(err)
	}
	var created Item
	if err = json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	if created.ContentVersion != 1 || created.StateVersion != 1 || created.TodoState != "none" {
		t.Fatalf("bad initial state: %+v", created)
	}
	now = now.Add(time.Minute)
	raw, err = a.ApplyItemAction(ctx, created.ID, ApplyItemAction{RequestID: "todo", ExpectedStateVersion: 1, Action: Action{Type: "set_todo", State: "todo"}})
	if err != nil {
		t.Fatal(err)
	}
	var todo Item
	_ = json.Unmarshal(raw, &todo)
	raw, err = a.ApplyItemAction(ctx, created.ID, ApplyItemAction{RequestID: "todo-again", ExpectedStateVersion: 2, Action: Action{Type: "set_todo", State: "todo"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &todo)
	if todo.StateVersion != 2 {
		t.Fatalf("no-op Todo changed state version: %+v", todo)
	}
	raw, err = a.ApplyItemAction(ctx, created.ID, ApplyItemAction{RequestID: "reminder", ExpectedStateVersion: 2, Action: Action{Type: "set_reminder", Date: "2026-09-16", Timezone: "Asia/Singapore"}})
	if err != nil {
		t.Fatal(err)
	}
	var reminded Item
	_ = json.Unmarshal(raw, &reminded)
	if reminded.TodoState != "todo" || reminded.RemindAt == nil || reminded.RemindAt.Format(time.RFC3339) != "2026-09-16T01:00:00Z" {
		t.Fatalf("bad reminder: %+v", reminded)
	}
	input.RequestID = "freshness"
	input.ExpectedContentVersion = 1
	input.Sources[0].ObservedAt = observed.Add(time.Hour)
	raw, err = a.PutItem(ctx, created.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	var fresh Item
	_ = json.Unmarshal(raw, &fresh)
	if fresh.ContentVersion != 1 || fresh.StateVersion != 3 || fresh.TodoState != "todo" || fresh.RemindAt == nil || !fresh.Sources[0].ObservedAt.Equal(observed.Add(time.Hour)) {
		t.Fatalf("freshness update damaged state: %+v", fresh)
	}
	input.RequestID = "update"
	input.Title = "Choose the safe rollout order"
	raw, err = a.PutItem(ctx, created.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	var updated Item
	_ = json.Unmarshal(raw, &updated)
	if updated.ContentVersion != 2 || updated.StateVersion != 3 || updated.TodoState != "todo" || updated.RemindAt == nil {
		t.Fatalf("content update damaged state: %+v", updated)
	}
	if _, err = a.ApplyItemAction(ctx, created.ID, ApplyItemAction{RequestID: "stale", ExpectedStateVersion: 2, Action: Action{Type: "set_todo", State: "done"}}); err == nil {
		t.Fatal("accepted stale local-state edit")
	}
	raw, err = a.ApplyItemAction(ctx, created.ID, ApplyItemAction{RequestID: "done", ExpectedStateVersion: 3, Action: Action{Type: "set_todo", State: "done"}})
	if err != nil {
		t.Fatal(err)
	}
	var done Item
	_ = json.Unmarshal(raw, &done)
	if done.RemindAt != nil || done.TodoState != "done" {
		t.Fatalf("done did not clear reminder: %+v", done)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := New(s).Item(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.TodoState != "done" || persisted.ContentVersion != 2 || persisted.StateVersion != 4 {
		t.Fatalf("restart lost state: %+v", persisted)
	}
}

func TestReminderRejectsImpossibleAndDisambiguatesRepeatedTime(t *testing.T) {
	_, _, err := reminderInstant(Action{Date: "2026-03-08", Time: "02:30", Timezone: "America/New_York"})
	problem, ok := err.(*Error)
	if !ok || problem.Code != "nonexistent_local_time" {
		t.Fatalf("expected nonexistent time, got %v", err)
	}
	_, offsets, err := reminderInstant(Action{Date: "2026-11-01", Time: "01:30", Timezone: "America/New_York"})
	if err != nil || len(offsets) != 2 {
		t.Fatalf("expected two occurrences, got offsets=%v err=%v", offsets, err)
	}
	instant, candidates, err := reminderInstant(Action{Date: "2026-11-01", Time: "01:30", Timezone: "America/New_York", UTCOffset: "-05:00"})
	if err != nil || len(candidates) != 2 || instant.Format(time.RFC3339) != "2026-11-01T06:30:00Z" {
		t.Fatalf("bad chosen occurrence: %s %v %v", instant, candidates, err)
	}
}

func TestAllItemKindsShareContentAndOnlyTaskDefaultsTodo(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	if _, err = a.PutItem(ctx, "", PutItem{RequestID: "too-large", DedupeKey: "large", Kind: "report", Title: "Large", Summary: "Too large.", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: strings.Repeat("x", 513<<10)}}); err == nil {
		t.Fatal("accepted an item larger than 512 KiB")
	}
	for _, kind := range []string{"note", "report", "task", "outcome"} {
		raw, putErr := a.PutItem(ctx, "", PutItem{RequestID: "kind-" + kind, DedupeKey: "kind:" + kind, Kind: kind, Title: kind + " title", Summary: "Same content shape.", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "A readable body."}})
		if putErr != nil {
			t.Fatal(putErr)
		}
		var item Item
		_ = json.Unmarshal(raw, &item)
		want := "none"
		if kind == "task" {
			want = "todo"
		}
		if item.TodoState != want || item.Report.BodyMD != "A readable body." {
			t.Fatalf("kind %s has unexpected behavior: %+v", kind, item)
		}
	}
}
