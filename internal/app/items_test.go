package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestItemsPageMatchesFullCollectionOrderAndFilters(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	a := New(s)
	a.Now = func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) }
	raw, err := a.CreateInterest(ctx, CreateInterest{RequestID: "page-interest", Title: "Page search interest", InstructionsMD: "Fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	if err = json.Unmarshal(raw, &interest); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		// Duplicate timestamps exercise the ID tie-breaker as well as direction.
		a.Now = func() time.Time { return time.Date(2026, 9, 28, 12+i/2, 0, 0, 0, time.UTC) }
		kind := "note"
		if i%3 == 0 {
			kind = "task"
		}
		input := PutItem{RequestID: fmt.Sprintf("page-%d", i), DedupeKey: fmt.Sprintf("page:%d", i), Kind: kind, Title: fmt.Sprintf("Item %02d", i), Summary: "Page fixture", Report: Report{SchemaVersion: 1, BodyMD: "Test content"}}
		if i == 11 {
			input.Interests = []ItemInterest{{ID: interest.ID, Reason: "Fixture"}}
		}
		_, err = a.PutItem(ctx, "", input)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, filter := range []ItemFilters{{View: "all"}, {View: "todo"}, {View: "attention"}, {View: "all", Kind: "task"}, {View: "all", Query: "Item 09"}, {View: "all", Query: "Page search interest"}, {View: "all", InterestID: interest.ID}, {View: "all", Sort: "newest"}, {View: "all", Sort: "oldest"}, {View: "all", Sort: "title"}, {View: "todo", Sort: "newest"}, {View: "all", Query: "Item 0", Sort: "title"}} {
		all, err := a.Items(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		for index := 1; index < len(all); index++ {
			before, after := all[index-1], all[index]
			if filter.Sort == "newest" && before.ContentUpdatedAt.Before(after.ContentUpdatedAt) || filter.Sort == "oldest" && before.ContentUpdatedAt.After(after.ContentUpdatedAt) || filter.Sort == "title" && strings.Compare(strings.ToLower(before.Title), strings.ToLower(after.Title)) > 0 {
				t.Fatalf("incorrect %s order: %s before %s", filter.Sort, before.Title, after.Title)
			}
		}
		var got []string
		cursor := ""
		for {
			page, more, pageErr := a.ItemsPage(ctx, filter, cursor, 2)
			if pageErr != nil {
				t.Fatal(pageErr)
			}
			for _, item := range page {
				got = append(got, item.ID)
			}
			if !more {
				break
			}
			if len(page) != 2 {
				t.Fatalf("short nonfinal page: %+v", filter)
			}
			cursor = page[len(page)-1].ID
		}
		want := make([]string, 0, len(all))
		for _, item := range all {
			want = append(want, item.ID)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("paged order differs for %+v: got %v, want %v", filter, got, want)
		}
	}
	byInterestTitle, err := a.Items(ctx, ItemFilters{Query: "Page search interest"})
	if err != nil || len(byInterestTitle) != 1 {
		t.Fatalf("Interest title search: %d Items, %v", len(byInterestTitle), err)
	}
	if _, _, err = a.ItemsPage(ctx, ItemFilters{View: "todo"}, "missing", 2); err == nil {
		t.Fatal("accepted cursor outside collection")
	}
	if _, _, err = a.ItemsPage(ctx, ItemFilters{Sort: "unknown"}, "", 2); err == nil {
		t.Fatal("unknown sort accepted")
	}
}

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

func TestFollowThroughMarksViewedContentSeen(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := New(s)
	for _, action := range []Action{
		{Type: "set_todo", State: "todo"},
		{Type: "set_todo", State: "done"},
		{Type: "set_reminder", Date: "2026-10-09", Timezone: "Asia/Singapore"},
	} {
		t.Run(action.Type+action.State, func(t *testing.T) {
			input := PutItem{RequestID: t.Name() + "create", DedupeKey: t.Name(), Kind: "note", Title: "Viewed update", Summary: "Fixture evidence", Report: Report{SchemaVersion: 1, BodyMD: "First evidence"}}
			raw, err := a.PutItem(ctx, "", input)
			if err != nil {
				t.Fatal(err)
			}
			var item Item
			if err = json.Unmarshal(raw, &item); err != nil {
				t.Fatal(err)
			}
			input.RequestID = t.Name() + "update"
			input.ExpectedContentVersion = 1
			input.Title = "Newer evidence"
			if _, err = a.PutItem(ctx, item.ID, input); err != nil {
				t.Fatal(err)
			}
			// Content can advance without changing state_version. Only mark the viewed version.
			action.ContentVersion = 1
			raw, err = a.ApplyItemAction(ctx, item.ID, ApplyItemAction{RequestID: t.Name() + "action", ExpectedStateVersion: 1, Action: action})
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(raw, &item); err != nil {
				t.Fatal(err)
			}
			if item.AcknowledgedContentVersion != 1 || item.ContentVersion != 2 || item.StateVersion != 2 {
				t.Fatalf("incorrect viewed version: %+v", item)
			}
			// Repeating the same follow-through can still mark a newly viewed update seen.
			action.ContentVersion = 2
			raw, err = a.ApplyItemAction(ctx, item.ID, ApplyItemAction{RequestID: t.Name() + "seen", ExpectedStateVersion: 2, Action: action})
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(raw, &item); err != nil {
				t.Fatal(err)
			}
			if item.AcknowledgedContentVersion != 2 || item.StateVersion != 3 {
				t.Fatalf("no-op action missed seen state: %+v", item)
			}
		})
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
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
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

func TestInterestLevelItemDoesNotCountAsWatchCoverage(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	a := New(s)
	raw, err := a.CreateInterest(ctx, CreateInterest{RequestID: "interest-level-interest", Title: "Cross-source context"})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	_ = json.Unmarshal(raw, &interest)
	id := interest.ID
	raw, err = a.UpsertInterestItem(ctx, PutItem{DedupeKey: "interest:cross-source", ExpectedContentVersion: 0, Kind: "note", Interests: []ItemInterest{{ID: id, Reason: "Cross-source context"}}, Title: "Cross-source note", Summary: "Relevant beyond one Watch.", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "This finding is Interest-level."}})
	if err != nil {
		t.Fatal(err)
	}
	var item Item
	_ = json.Unmarshal(raw, &item)
	if len(item.Interests) != 1 || item.Interests[0].ID != interest.ID || item.WatchID != nil {
		t.Fatalf("bad Interest-level item: %+v", item)
	}
	if _, err = a.UpsertInterestItem(ctx, PutItem{RequestID: "bad-watch-item", DedupeKey: "interest:bad", ExpectedContentVersion: 0, Kind: "note", Interests: []ItemInterest{{ID: id, Reason: "Test"}}, WatchID: &id, Title: "Bad", Summary: "Bad", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "Bad"}}); err == nil {
		t.Fatal("accepted Watch-bound Interest-level item")
	}
	watchRaw, err := a.CreateWatch(ctx, CreateWatch{RequestID: "interest-level-watch", InterestIDs: []string{interest.ID}, MatchingPolicy: "explicit", Source: WatchSource{Kind: "fixture", Locator: "scope"}})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	if err = json.Unmarshal(watchRaw, &watch); err != nil {
		t.Fatal(err)
	}
	runRaw, err := a.StartRun(ctx, StartRun{RunnerLabel: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Run Run `json:"run"`
	}
	if err = json.Unmarshal(runRaw, &started); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	watchItemRaw, err := a.SubmitWatchFindings(ctx, started.Run.ID, watch.ID, SubmitWatchFindings{ExpectedWatchRevision: 1, Status: "success", Coverage: Coverage{CursorBefore: json.RawMessage("null"), CursorAfter: json.RawMessage("null"), ObservedThrough: now}, Items: []PutItem{{DedupeKey: "scope:watch", Kind: "note", Interests: []ItemInterest{{ID: id, Reason: "Test"}}, Title: "Watch finding", Summary: "Watch-scoped content.", Sources: []Source{{ID: "fixture", URL: "https://example.com/scope", Label: "Fixture", ObservedAt: now}}, Report: Report{SchemaVersion: 1, BodyMD: "Watch content."}}}})
	if err != nil {
		t.Fatal(err)
	}
	var submitted struct {
		Items []SubmittedItem `json:"items"`
	}
	if err = json.Unmarshal(watchItemRaw, &submitted); err != nil {
		t.Fatal(err)
	}
	watchItem, err := a.Item(ctx, submitted.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.UpsertInterestItem(ctx, PutItem{RequestID: "scope-conflict", DedupeKey: "scope:watch", ExpectedContentVersion: 1, Kind: "note", Interests: []ItemInterest{{ID: id, Reason: "Test"}}, Title: "Cross-source replacement", Summary: "Must not detach the Watch.", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "Replacement."}}); err == nil {
		t.Fatal("Interest-level upsert detached a Watch-scoped item")
	}
	scoped, err := a.Item(ctx, watchItem.ID)
	if err != nil || scoped.WatchID == nil || *scoped.WatchID != watch.ID {
		t.Fatalf("Watch scope changed after rejected Interest upsert: %+v %v", scoped, err)
	}
}
