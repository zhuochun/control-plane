package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestBroadAndExplicitWatchersKeepIndependentCheckpoints(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	createInterest := func(slug string) Interest {
		raw, err := a.CreateInterest(ctx, CreateInterest{Slug: slug, Title: slug, InstructionsMD: "Assess " + slug})
		if err != nil {
			t.Fatal(err)
		}
		var value Interest
		if err = json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	delivery := createInterest("delivery")
	security := createInterest("security")
	createWatch := func(slug, policy string, ids []string) Watch {
		raw, err := a.CreateWatch(ctx, CreateWatch{Slug: slug, MatchingPolicy: policy, InterestIDs: ids, Source: WatchSource{Kind: "fixture", Locator: slug}})
		if err != nil {
			t.Fatal(err)
		}
		var value Watch
		if err = json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	broad := createWatch("gmail-inbox", "broad", nil)
	focused := createWatch("gmail-security", "explicit", []string{security.ID})
	if _, err = a.Watch(ctx, "gmail-inbox"); err != nil {
		t.Fatal(err)
	}
	raw, err := a.StartRun(ctx, StartRun{RunnerLabel: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Run   Run `json:"run"`
		Brief struct {
			Watches []SelectedWatch `json:"watches"`
		} `json:"brief"`
	}
	if err = json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	selected := map[string]SelectedWatch{}
	for _, watch := range packet.Brief.Watches {
		selected[watch.ID] = watch
	}
	if len(selected[broad.ID].Interests) != 2 || len(selected[focused.ID].Interests) != 1 || selected[focused.ID].Interests[0].ID != security.ID {
		t.Fatalf("wrong captured Interest sets: %+v", selected)
	}
	oldDue := focused.NextDueAt
	_, err = a.UpdateWatch(ctx, focused.Slug, UpdateWatch{ExpectedRevision: 1, Slug: Field[string]{Set: true, Value: "gmail-delivery"}, InterestIDs: Field[[]string]{Set: true, Value: []string{delivery.Slug}}})
	if err != nil {
		t.Fatal(err)
	}
	focused, err = a.Watch(ctx, focused.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Watch(ctx, "gmail-security"); err == nil {
		t.Fatal("old slug still resolves after rename")
	}
	if focused.Revision != 2 || focused.Slug != "gmail-delivery" || !focused.NextDueAt.Equal(oldDue) || len(focused.InterestIDs) != 1 || focused.InterestIDs[0] != delivery.ID {
		t.Fatalf("link edit changed checkpoint or schedule: %+v", focused)
	}
	source := Source{ID: "mail", URL: "https://example.com/mail", Label: "Mail", ObservedAt: now}
	_, err = a.SubmitWatchFindings(ctx, packet.Run.ID, focused.ID, SubmitWatchFindings{ExpectedWatchRevision: 1, Status: "success", Coverage: Coverage{CursorBefore: json.RawMessage("null"), CursorAfter: json.RawMessage(`{"position":1}`), ObservedThrough: now}, Items: []PutItem{{DedupeKey: "mail:one", Kind: "note", Title: "Security mail", Summary: "Review security mail", Interests: []ItemInterest{{ID: security.ID, Reason: "Security decision"}}, Sources: []Source{source}, Report: Report{SchemaVersion: 1, BodyMD: "Read mail"}}}})
	if err != nil {
		t.Fatalf("old snapshot publication failed after link edit: %v", err)
	}
	focused, err = a.Watch(ctx, focused.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(focused.Cursor) != `{"position":1}` || !focused.NextDueAt.Equal(oldDue) {
		t.Fatalf("old run changed newer schedule: %+v", focused)
	}
	items, err := a.Items(ctx, ItemFilters{InterestID: security.Slug})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].WatchID == nil || *items[0].WatchID != focused.ID || len(items[0].Interests) != 1 {
		t.Fatalf("lost captured provenance: %+v", items)
	}
	now = now.Add(time.Minute)
	_, err = a.UpdateWatch(ctx, broad.Slug, UpdateWatch{ExpectedRevision: 1, Source: Field[WatchSource]{Set: true, Value: WatchSource{Kind: "fixture", Locator: "new-inbox"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.UpdateWatch(ctx, broad.Slug, UpdateWatch{ExpectedRevision: 2, Source: Field[WatchSource]{Set: true, Value: broad.Source}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.SubmitWatchFindings(ctx, packet.Run.ID, broad.ID, SubmitWatchFindings{ExpectedWatchRevision: 1, Status: "success", Coverage: Coverage{CursorBefore: json.RawMessage("null"), CursorAfter: json.RawMessage(`{"position":9}`), ObservedThrough: now}})
	if err != nil {
		t.Fatalf("old source result failed: %v", err)
	}
	broad, err = a.Watch(ctx, broad.ID)
	if err != nil {
		t.Fatal(err)
	}
	if broad.Cursor != nil || broad.SourceGeneration != 3 || !broad.NextDueAt.Equal(now) {
		t.Fatalf("old source result advanced new scope: %+v", broad)
	}
}

func TestUserItemWithoutLinkOrWithDateOnlySource(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	base := PutItem{DedupeKey: "user:idea", Kind: "task", Title: "Draft idea", Summary: "Think this through", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "Personal context"}}
	raw, err := a.PutItem(ctx, "", base)
	if err != nil {
		t.Fatal(err)
	}
	var item Item
	if err = json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	if len(item.Sources) != 0 || item.TodoState != "todo" {
		t.Fatalf("source-free user Item failed: %+v", item)
	}
	base.ExpectedContentVersion = 1
	base.Sources = []Source{{ID: "date", SourceDate: "2026-09-27"}}
	if _, err = a.PutItem(ctx, item.ID, base); err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Sources) != 1 || item.Sources[0].URL != "" || item.Sources[0].SourceDate != "2026-09-27" {
		t.Fatalf("date-only source lost: %+v", item.Sources)
	}
	encoded, err := json.Marshal(item.Sources[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"id":"date","label":"","source_date":"2026-09-27"}` {
		t.Fatalf("date-only source claimed observation: %s", encoded)
	}
}

func TestAgentItemWithoutWatcherDoesNotClaimAttention(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	raw, err := a.CreateInterest(ctx, CreateInterest{Title: "Review", Slug: "review"})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	if err = json.Unmarshal(raw, &interest); err != nil {
		t.Fatal(err)
	}
	_, err = a.UpsertInterestItem(ctx, PutItem{DedupeKey: "agent:unwatched", Kind: "task", Interests: []ItemInterest{{ID: interest.ID, Reason: "Potentially useful"}}, Title: "Review", Summary: "No Watcher evidence", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "Unwatched"}})
	if err != nil {
		t.Fatal(err)
	}
	attention, err := a.Items(ctx, ItemFilters{View: "attention"})
	if err != nil {
		t.Fatal(err)
	}
	if len(attention) != 0 {
		t.Fatalf("unwatched agent Item entered Attention: %+v", attention)
	}
	userRaw, err := a.PutItem(ctx, "", PutItem{DedupeKey: "user:task", Kind: "task", Title: "User task", Summary: "User-owned", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "Do it"}})
	if err != nil {
		t.Fatal(err)
	}
	var user Item
	if err = json.Unmarshal(userRaw, &user); err != nil {
		t.Fatal(err)
	}
	attention, err = a.Items(ctx, ItemFilters{View: "attention"})
	if err != nil {
		t.Fatal(err)
	}
	if len(attention) != 1 || attention[0].ID != user.ID {
		t.Fatalf("user task missing Attention: %+v", attention)
	}
	health, err := a.OperationalHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts := health["item_counts"].(ItemCounts)
	if counts.All != 2 || counts.Attention != len(attention) || counts.Todo != 2 {
		t.Fatalf("navigation counts disagree with Item views: %+v", counts)
	}
}

func TestOneMatterAddsEvidenceAndInterestReasonWithoutLosingState(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	createInterest := func(slug string) Interest {
		raw, err := a.CreateInterest(ctx, CreateInterest{Slug: slug, Title: slug})
		if err != nil {
			t.Fatal(err)
		}
		var i Interest
		_ = json.Unmarshal(raw, &i)
		return i
	}
	first := createInterest("first")
	second := createInterest("second")
	raw, err := a.CreateWatch(ctx, CreateWatch{Slug: "one-inbox", MatchingPolicy: "broad", Source: WatchSource{Kind: "gmail", Locator: "inbox"}})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	_ = json.Unmarshal(raw, &watch)
	submit := func(runID, request string, before, after json.RawMessage, version int64, reason ItemInterest, source Source) {
		_, err := a.SubmitWatchFindings(ctx, runID, watch.ID, SubmitWatchFindings{RequestID: request, ExpectedWatchRevision: 1, Status: "success", Coverage: Coverage{CursorBefore: before, CursorAfter: after, ObservedThrough: now}, Items: []PutItem{{DedupeKey: "matter-1", ExpectedContentVersion: version, Kind: "note", Title: "One matter", Summary: "Still relevant", Interests: []ItemInterest{reason}, Sources: []Source{source}, Report: Report{SchemaVersion: 1, BodyMD: "Matter"}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	start := func(request string, force bool) Run {
		input := StartRun{RequestID: request, RunnerLabel: "fixture"}
		if force {
			input.WatchIDs = Field[[]string]{Set: true, Value: []string{watch.ID}}
			input.Force = true
		}
		raw, err := a.StartRun(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Run Run `json:"run"`
		}
		_ = json.Unmarshal(raw, &result)
		return result.Run
	}
	run := start("first-run", false)
	submit(run.ID, "first-result", json.RawMessage("null"), json.RawMessage(`{"n":1}`), 0, ItemInterest{ID: first.ID, Reason: "First reason"}, Source{ID: "mail", URL: "https://example.com/mail", Label: "Mail", ObservedAt: now})
	if _, err = a.FinishRun(ctx, run.ID, FinishRun{Summary: "First"}); err != nil {
		t.Fatal(err)
	}
	items, err := a.Items(ctx, ItemFilters{DedupeKey: "matter-1"})
	if err != nil || len(items) != 1 {
		t.Fatalf("first Item missing: %+v %v", items, err)
	}
	if _, err = a.ApplyItemAction(ctx, items[0].ID, ApplyItemAction{ExpectedStateVersion: 1, Action: Action{Type: "set_todo", State: "todo"}}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	run = start("second-run", true)
	submit(run.ID, "second-result", json.RawMessage(`{"n":1}`), json.RawMessage(`{"n":2}`), 1, ItemInterest{ID: second.ID, Reason: "Second reason"}, Source{ID: "thread", URL: "https://example.com/thread", Label: "Thread", ObservedAt: now})
	item, err := a.Item(ctx, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.TodoState != "todo" || len(item.Interests) != 2 || len(item.Sources) != 2 || item.ContentVersion != 2 {
		t.Fatalf("reason/evidence merge lost state: %+v", item)
	}
}
