package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestWatchPublicationAdvancesCoverageAndPreservesHumanState(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	now := time.Date(2026, 9, 15, 3, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	raw, err := a.CreateInterest(ctx, CreateInterest{RequestID: "interest", Title: "Delivery", InstructionsMD: "Notice decisions."})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	_ = json.Unmarshal(raw, &interest)
	raw, err = a.CreateWatch(ctx, CreateWatch{RequestID: "watch", InterestID: interest.ID, Source: WatchSource{Kind: "slack", Locator: "channel"}, InstructionsMD: "Read new threads."})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	_ = json.Unmarshal(raw, &watch)
	raw, err = a.StartRun(ctx, StartRun{RequestID: "run-one", RunnerLabel: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Run Run `json:"run"`
	}
	_ = json.Unmarshal(raw, &started)
	entry := PutItem{DedupeKey: "slack:channel:thread:1", ExpectedContentVersion: 0, Kind: "report", Title: "Choose rollout order", Summary: "A decision is needed.", Sources: []Source{{ID: "thread", URL: "https://example.com/thread", Label: "Thread", ObservedAt: now}}, Report: Report{SchemaVersion: 1, BodyMD: "Source status: open."}}
	publication := PublishWatchResult{RequestID: "publish-one", ExpectedWatchRevision: 1, Status: "success", Coverage: Coverage{CursorBefore: json.RawMessage("null"), CursorAfter: json.RawMessage(`{"position":1}`), ObservedThrough: now, Limitations: []string{}}, Items: []PutItem{entry}}
	withoutSource := publication
	withoutSource.RequestID = "publish-without-source"
	withoutSource.Items = []PutItem{entry}
	withoutSource.Items[0].Sources = nil
	if _, err = a.PublishWatchResult(ctx, started.Run.ID, watch.ID, withoutSource); err == nil {
		t.Fatal("Watch publication accepted a source-less finding")
	}
	raw, err = a.PublishWatchResult(ctx, started.Run.ID, watch.ID, publication)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Items []PublishedItem `json:"items"`
	}
	_ = json.Unmarshal(raw, &result)
	if len(result.Items) != 1 || !result.Items[0].Changed {
		t.Fatalf("bad publication response: %s", raw)
	}
	var storedResult string
	if err = s.DB.QueryRow(`SELECT result FROM watch_results WHERE run_id=? AND watch_id=?`, started.Run.ID, watch.ID).Scan(&storedResult); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedResult, entry.Title) || strings.Contains(storedResult, entry.Report.BodyMD) {
		t.Fatalf("Watch result duplicated report content: %s", storedResult)
	}
	detail, err := a.RunDetail(ctx, started.Run.ID)
	if err != nil || len(detail["results"].([]WatchResult)) != 1 {
		t.Fatalf("run detail omitted Watch result: %#v %v", detail, err)
	}
	health, err := a.OperationalHealth(ctx)
	if err != nil || len(health["watches"].([]WatchHealth)) != 1 || health["watches"].([]WatchHealth)[0].LastStatus != "success" {
		t.Fatalf("operational health omitted Watch coverage: %#v %v", health, err)
	}
	itemID := result.Items[0].ID
	watch, err = a.Watch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(watch.Cursor) != `{"position":1}` || !watch.NextDueAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("checkpoint did not advance on its grid: %+v", watch)
	}
	if _, err = a.FinishRun(ctx, started.Run.ID, FinishRun{RequestID: "finish-one", Summary: "Published one report"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Minute)
	forced := Field[[]string]{Set: true, Value: []string{watch.ID}}
	raw, err = a.StartRun(ctx, StartRun{RequestID: "run-two", RunnerLabel: "fixture", WatchIDs: forced, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	var second struct {
		Run Run `json:"run"`
	}
	_ = json.Unmarshal(raw, &second)
	if _, err = a.ApplyItemAction(ctx, itemID, ApplyItemAction{RequestID: "todo", ExpectedStateVersion: 1, Action: Action{Type: "set_todo", State: "todo"}}); err != nil {
		t.Fatal(err)
	}
	entry.ExpectedContentVersion = 1
	entry.Title = "Choose a safe rollout order"
	publication.RequestID = "publish-two"
	publication.Coverage.CursorBefore = json.RawMessage(`{"position":1}`)
	publication.Coverage.CursorAfter = json.RawMessage(`{"position":2}`)
	publication.Coverage.ObservedThrough = now
	publication.Items = []PutItem{entry}
	if _, err = a.PublishWatchResult(ctx, second.Run.ID, watch.ID, publication); err != nil {
		t.Fatal(err)
	}
	item, err := a.Item(ctx, itemID)
	if err != nil {
		t.Fatal(err)
	}
	if item.ContentVersion != 2 || item.StateVersion != 2 || item.TodoState != "todo" {
		t.Fatalf("publication overwrote human state: %+v", item)
	}
	watch, err = a.Watch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !watch.NextDueAt.Equal(time.Date(2026, 9, 15, 5, 0, 0, 0, time.UTC)) {
		t.Fatalf("forced run moved future schedule: %s", watch.NextDueAt)
	}
	if _, err = a.FinishRun(ctx, second.Run.ID, FinishRun{RequestID: "finish-two", Summary: "Refreshed report"}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	raw, err = a.StartRun(ctx, StartRun{RequestID: "run-three", RunnerLabel: "fixture", WatchIDs: forced, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	var third struct {
		Run Run `json:"run"`
	}
	_ = json.Unmarshal(raw, &third)
	if _, err = a.UpdateInterest(ctx, interest.ID, UpdateInterest{RequestID: "edit-interest", ExpectedRevision: 1, InstructionsMD: Field[string]{Set: true, Value: "Notice decisions and risks."}}); err != nil {
		t.Fatal(err)
	}
	publication.RequestID = "fenced"
	publication.ExpectedWatchRevision = 1
	publication.Coverage.CursorBefore = json.RawMessage(`{"position":2}`)
	publication.Items = nil
	if _, err = a.PublishWatchResult(ctx, third.Run.ID, watch.ID, publication); err == nil {
		t.Fatal("published against changed Interest instructions")
	}
	var count int
	if err = s.DB.QueryRow(`SELECT count(*) FROM watch_results WHERE run_id=?`, third.Run.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("fenced publication left a result: %d %v", count, err)
	}
}

func TestMixedAndPartialResultsKeepTruthfulCheckpoints(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	now := time.Date(2026, 9, 15, 6, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }

	raw, err := a.CreateInterest(ctx, CreateInterest{RequestID: "mixed-interest", Title: "Sources", InstructionsMD: "Report useful changes."})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	_ = json.Unmarshal(raw, &interest)
	makeWatch := func(request, locator string) Watch {
		raw, createErr := a.CreateWatch(ctx, CreateWatch{RequestID: request, InterestID: interest.ID, Source: WatchSource{Kind: "fixture", Locator: locator}, InstructionsMD: "Inspect fixture."})
		if createErr != nil {
			t.Fatal(createErr)
		}
		var watch Watch
		_ = json.Unmarshal(raw, &watch)
		return watch
	}
	good := makeWatch("mixed-good", "good")
	retry := makeWatch("mixed-retry", "retry")
	raw, err = a.StartRun(ctx, StartRun{RequestID: "mixed-run", RunnerLabel: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Run Run `json:"run"`
	}
	_ = json.Unmarshal(raw, &started)
	coverage := func(after, next string) Coverage {
		return Coverage{CursorBefore: json.RawMessage(after), CursorAfter: json.RawMessage(next), ObservedThrough: now, Limitations: []string{}}
	}
	if _, err = a.PublishWatchResult(ctx, started.Run.ID, good.ID, PublishWatchResult{RequestID: "mixed-success", ExpectedWatchRevision: 1, Status: "success", Coverage: coverage("null", `{"position":1}`)}); err != nil {
		t.Fatal(err)
	}
	sources := []Source{{ID: "fixture", URL: "https://example.com/partial", Label: "Partial fixture", ObservedAt: now}}
	partial := PublishWatchResult{RequestID: "mixed-partial", ExpectedWatchRevision: 1, Status: "partial", Error: "fixture ended early", Coverage: coverage("null", "null"), Items: []PutItem{{DedupeKey: "partial:item", Kind: "note", Title: "Partial finding", Summary: "Saved before the source ended.", Sources: sources, Report: Report{SchemaVersion: 1, BodyMD: "Coverage is partial."}}}}
	first, err := a.PublishWatchResult(ctx, started.Run.ID, retry.ID, partial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.PublishWatchResult(ctx, started.Run.ID, retry.ID, PublishWatchResult{RequestID: "replacement", ExpectedWatchRevision: 1, Status: "failed", Error: "replace", Coverage: coverage("null", "null")}); err == nil {
		t.Fatal("replaced a final result within one run")
	}
	finished, err := a.FinishRun(ctx, started.Run.ID, FinishRun{RequestID: "mixed-finish", Summary: "One source needs retry."})
	if err != nil {
		t.Fatal(err)
	}
	var run Run
	_ = json.Unmarshal(finished, &run)
	if run.Status != "partial" {
		t.Fatalf("run status = %s, want partial", run.Status)
	}
	// Receipts are checked before live-run state, so a transport retry still
	// receives the committed response after the run has finished.
	replayed, err := a.PublishWatchResult(ctx, started.Run.ID, retry.ID, partial)
	if err != nil || string(replayed) != string(first) {
		t.Fatalf("partial result did not replay: %s %s %v", first, replayed, err)
	}
	good, _ = a.Watch(ctx, good.ID)
	retry, _ = a.Watch(ctx, retry.ID)
	if string(good.Cursor) != `{"position":1}` || len(retry.Cursor) != 0 || retry.NextDueAt.After(now) {
		t.Fatalf("mixed checkpoints are not truthful: good=%s retry=%s due=%s", good.Cursor, retry.Cursor, retry.NextDueAt)
	}
	items, err := a.Items(ctx, ItemFilters{DedupeKey: "partial:item"})
	if err != nil || len(items) != 1 {
		t.Fatalf("partial item missing: %d %v", len(items), err)
	}

	now = now.Add(time.Minute)
	selected := Field[[]string]{Set: true, Value: []string{retry.ID}}
	raw, err = a.StartRun(ctx, StartRun{RequestID: "retry-run", RunnerLabel: "fixture", WatchIDs: selected})
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &started)
	sources[0].ObservedAt = now
	complete := PublishWatchResult{RequestID: "retry-success", ExpectedWatchRevision: 1, Status: "success", Coverage: Coverage{CursorBefore: json.RawMessage("null"), CursorAfter: json.RawMessage(`{"position":2}`), ObservedThrough: now, Limitations: []string{}}, Items: []PutItem{{DedupeKey: "partial:item", ExpectedContentVersion: 1, Kind: "note", Title: "Partial finding", Summary: "Coverage is now complete.", Sources: sources, Report: Report{SchemaVersion: 1, BodyMD: "Coverage is complete."}}}}
	if _, err = a.PublishWatchResult(ctx, started.Run.ID, retry.ID, complete); err != nil {
		t.Fatal(err)
	}
	items, _ = a.Items(ctx, ItemFilters{DedupeKey: "partial:item"})
	if len(items) != 1 || items[0].ContentVersion != 2 {
		t.Fatalf("retry duplicated or failed to update item: %+v", items)
	}
}
