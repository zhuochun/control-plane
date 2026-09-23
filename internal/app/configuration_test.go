package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestConfigurationPreservesCursorAndFencesStaleEdits(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	created, err := a.CreateInterest(ctx, CreateInterest{RequestID: "interest", Title: "Engineering", InstructionsMD: "Follow useful changes."})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	if err = json.Unmarshal(created, &interest); err != nil {
		t.Fatal(err)
	}
	result, err := a.CreateWatch(ctx, CreateWatch{RequestID: "watch", InterestID: interest.ID, Source: WatchSource{Kind: "github", Locator: "org/repo"}})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	if err = json.Unmarshal(result, &watch); err != nil {
		t.Fatal(err)
	}
	if watch.IntervalSeconds != 7200 || watch.LookbackSeconds != 604800 {
		t.Fatalf("bad defaults: %+v", watch)
	}
	_, err = s.DB.Exec(`UPDATE watches SET cursor='{"position":42}',next_due_at=? WHERE id=?`, now.Add(time.Hour).UnixMilli(), watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	update := UpdateInterest{RequestID: "edit", ExpectedRevision: 1, InstructionsMD: Field[string]{Set: true, Value: "Follow security changes."}}
	response, err := a.UpdateInterest(ctx, interest.ID, update)
	if err != nil {
		t.Fatal(err)
	}
	watch, err = a.Watch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !watch.NextDueAt.Equal(now) || string(watch.Cursor) != `{"position":42}` {
		t.Fatalf("interest edit lost checkpoint or did not reschedule: %+v", watch)
	}
	update.RequestID = "stale"
	if _, err = a.UpdateInterest(ctx, interest.ID, update); err == nil {
		t.Fatal("accepted stale interest edit")
	}
	update.RequestID = "edit"
	replay, err := a.UpdateInterest(ctx, interest.ID, update)
	if err != nil || string(replay) != string(response) {
		t.Fatalf("replay failed: %s %v", replay, err)
	}
	_, err = a.UpdateWatch(ctx, watch.ID, UpdateWatch{RequestID: "source", ExpectedRevision: 1, Source: Field[WatchSource]{Set: true, Value: WatchSource{Kind: "github", Locator: "org/other"}}})
	if err != nil {
		t.Fatal(err)
	}
	watch, err = a.Watch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if watch.Cursor != nil || watch.Revision != 2 {
		t.Fatalf("source change retained cursor: %+v", watch)
	}
}

func TestConfigurationListsActiveByDefaultAndResolvesUniquePrefixes(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	activeRaw, err := a.CreateInterest(ctx, CreateInterest{RequestID: "active-list", Title: "Active"})
	if err != nil {
		t.Fatal(err)
	}
	pausedRaw, err := a.CreateInterest(ctx, CreateInterest{RequestID: "paused-list", Title: "Paused", State: "paused"})
	if err != nil {
		t.Fatal(err)
	}
	var active, paused Interest
	_ = json.Unmarshal(activeRaw, &active)
	_ = json.Unmarshal(pausedRaw, &paused)
	items, err := a.Interests(ctx)
	if err != nil || len(items) != 1 || items[0].ID != active.ID {
		t.Fatalf("default Interest list was not active-only: %+v %v", items, err)
	}
	items, err = a.Interests(ctx, "all")
	if err != nil || len(items) != 2 {
		t.Fatalf("explicit all Interest list failed: %+v %v", items, err)
	}
	resolved, err := a.Interest(ctx, active.ID[:8])
	if err != nil || resolved.ID != active.ID {
		t.Fatalf("unique ID prefix did not resolve: %+v %v", resolved, err)
	}
	if _, err = a.Interest(ctx, paused.ID[:8]); err != nil {
		t.Fatal("direct get should retrieve inactive records:", err)
	}
}
