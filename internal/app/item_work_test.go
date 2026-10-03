package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func workText(value string) *string { return &value }

func workFixture(t *testing.T) (*App, Item, Watch) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	a := New(s)
	raw, err := a.CreateInterest(ctx, CreateInterest{Title: "Specs"})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	if err = json.Unmarshal(raw, &interest); err != nil {
		t.Fatal(err)
	}
	raw, err = a.CreateWatch(ctx, CreateWatch{MatchingPolicy: "explicit", InterestIDs: []string{interest.ID}, Source: WatchSource{Kind: "fixture", Locator: "specs"}})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	if err = json.Unmarshal(raw, &watch); err != nil {
		t.Fatal(err)
	}
	raw, err = a.StartRun(ctx, StartRun{})
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Run Run `json:"run"`
	}
	if err = json.Unmarshal(raw, &started); err != nil {
		t.Fatal(err)
	}
	_, err = a.SubmitWatchFindings(ctx, started.Run.ID, watch.ID, SubmitWatchFindings{ExpectedWatchRevision: 1, Status: "success", Coverage: Coverage{CursorBefore: json.RawMessage("null"), CursorAfter: json.RawMessage(`{"n":1}`), ObservedThrough: a.Now()}, Items: []PutItem{{DedupeKey: "spec:one", Kind: "report", Title: "Spec", Summary: "New spec", Interests: []ItemInterest{{ID: interest.ID, Reason: "Assess feasibility"}}, Sources: []Source{{ID: "spec", URL: "https://example.com/spec", Label: "Spec", ObservedAt: a.Now()}}, Report: Report{SchemaVersion: 1, BodyMD: "Source finding"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.FinishRun(ctx, started.Run.ID, FinishRun{Summary: "Read spec"}); err != nil {
		t.Fatal(err)
	}
	items, err := a.Items(ctx, ItemFilters{DedupeKey: "spec:one"})
	if err != nil || len(items) != 1 {
		t.Fatalf("fixture: %v %v", items, err)
	}
	return a, items[0], watch
}

func TestWorkUpdatePreservesOwnershipCoverageAndRetries(t *testing.T) {
	ctx := context.Background()
	a, item, watch := workFixture(t)
	if _, err := a.SetUserNote(ctx, item.ID, SetUserNote{ExpectedStateVersion: 1, UserNote: "My decision"}); err != nil {
		t.Fatal(err)
	}
	state, err := a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.Watch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := UpdateItemWork{RequestID: "handoff", ExpectedContentVersion: item.ContentVersion, Delegations: []DelegationPatch{{ID: "feasibility", Executor: workText("codex:researcher"), InstructionsMD: workText("Research spec version 1"), Status: workText("pending"), ContextMD: workText("Input Item v1; launch not yet confirmed")}}}
	raw, err := a.UpdateItemWork(ctx, item.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	var updated Item
	if err = json.Unmarshal(raw, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Origin != item.Origin || !reflect.DeepEqual(updated.WatchID, item.WatchID) || !reflect.DeepEqual(updated.Interests, item.Interests) || updated.UserNote != state.UserNote || updated.StateVersion != state.StateVersion || updated.ContentVersion != 2 {
		t.Fatalf("damaged ownership/state: %+v", updated)
	}
	result := UpdateItemWork{RequestID: "result", ExpectedContentVersion: 2, Report: &Report{SchemaVersion: 1, BodyMD: "Based on spec v1: feasible with constraints"}, Sources: []Source{{ID: "research", URL: "https://example.com/research", Label: "Research", ObservedAt: a.Now()}}, Delegations: []DelegationPatch{{ID: "feasibility", ExternalRef: workText("session:one"), ContextMD: workText("Result uses Item v1 and original instructions; evidence in report")}}}
	if _, err = a.UpdateItemWork(ctx, item.ID, result); err != nil {
		t.Fatal(err)
	}
	replayed, err := a.UpdateItemWork(ctx, item.ID, input)
	if err != nil || string(replayed) != string(raw) {
		t.Fatalf("handoff replay: %s %v", replayed, err)
	}
	input.RequestID = "stale"
	if _, err = a.UpdateItemWork(ctx, item.ID, input); err == nil {
		t.Fatal("accepted stale update")
	} else {
		var problem *Error
		if !errors.As(err, &problem) || problem.Code != "content_conflict" {
			t.Fatalf("wrong conflict: %v", err)
		}
	}
	input.RequestID = "handoff"
	input.ContextMD = workText("Different request")
	if _, err = a.UpdateItemWork(ctx, item.ID, input); err == nil {
		t.Fatal("accepted changed retry")
	} else {
		var problem *Error
		if !errors.As(err, &problem) || problem.Code != "idempotency_conflict" {
			t.Fatalf("wrong retry error: %v", err)
		}
	}
	after, err := a.Watch(ctx, watch.ID)
	if err != nil || string(before.Cursor) != string(after.Cursor) || !before.NextDueAt.Equal(after.NextDueAt) {
		t.Fatalf("work advanced coverage: %+v %v", after, err)
	}
	if _, err = a.UpdateItemWork(ctx, "missing", UpdateItemWork{ExpectedContentVersion: 1}); err == nil {
		t.Fatal("created missing Item")
	}
}

func TestDelegationRepairLookupAndScanPreservation(t *testing.T) {
	ctx := context.Background()
	a, item, watch := workFixture(t)
	patches := []DelegationPatch{
		{ID: "research", Executor: workText("agent:A"), ExternalRef: workText("thread:1"), InstructionsMD: workText("Research spec v1"), Status: workText("pending")},
		{ID: "other", Executor: workText("agent:B"), ExternalRef: workText("thread:2"), InstructionsMD: workText("Compare alternatives"), Status: workText("blocked"), ContextMD: workText("Missing access")},
	}
	_, err := a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: 1,
		Report:    &Report{SchemaVersion: 1, BodyMD: "Research and conclusions", Actions: []ReportAction{{ID: "research-link", Type: "open_link", Label: "Research", SourceRef: "research"}}},
		Sources:   []Source{{ID: "research", URL: "https://example.com/research", Label: "Research", ObservedAt: a.Now()}},
		ContextMD: workText("Retained work"), Delegations: patches})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyItemAction(ctx, item.ID, ApplyItemAction{ExpectedStateVersion: 1, Action: Action{Type: "acknowledge", ContentVersion: 2}}); err != nil {
		t.Fatal(err)
	}
	items, more, err := a.ItemsPage(ctx, ItemFilters{DelegationStatus: "pending,blocked", Executor: "agent:A", ExternalRef: "thread:1"}, "", 1)
	if err != nil || more || len(items) != 1 || len(items[0].Delegations) != 2 {
		t.Fatalf("lookup after acknowledgement: %+v %v", items, err)
	}
	items, _, err = a.ItemsPage(ctx, ItemFilters{DelegationStatus: "pending", Executor: "agent:B"}, "", 1)
	if err != nil || len(items) != 0 {
		t.Fatalf("filters matched different entries: %+v %v", items, err)
	}
	_, err = a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: 2, Delegations: []DelegationPatch{{ID: "research", Status: workText("closed"), ContextMD: workText("Result v1 retained; resume thread:1 for repair")}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: 3, Delegations: []DelegationPatch{}})
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := a.Item(ctx, item.ID)
	if err != nil || unchanged.ContentVersion != 3 || len(unchanged.Delegations) != 2 {
		t.Fatalf("empty list removed work: %+v %v", unchanged, err)
	}
	_, err = a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: 3, Delegations: []DelegationPatch{{ID: "research", Status: workText("pending"), InstructionsMD: workText("Repair based on spec v2"), ContextMD: workText("Previous result used v1; now investigate v2")}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := a.StartRun(ctx, StartRun{Force: true, WatchIDs: Field[[]string]{Set: true, Value: []string{watch.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Run Run `json:"run"`
	}
	if err = json.Unmarshal(raw, &started); err != nil {
		t.Fatal(err)
	}
	_, err = a.SubmitWatchFindings(ctx, started.Run.ID, watch.ID, SubmitWatchFindings{ExpectedWatchRevision: 1, Status: "success", Coverage: Coverage{CursorBefore: json.RawMessage(`{"n":1}`), CursorAfter: json.RawMessage(`{"n":2}`), ObservedThrough: a.Now().Add(time.Minute)}, Items: []PutItem{{DedupeKey: item.DedupeKey, ExpectedContentVersion: 4, Kind: item.Kind, Title: "Spec v2", Summary: "Source changed", Interests: item.Interests, Sources: item.Sources, Report: Report{SchemaVersion: 1, BodyMD: "Source-only replacement"}}}})
	if err != nil {
		t.Fatal(err)
	}
	current, err := a.Item(ctx, item.ID)
	if err != nil || current.Report.BodyMD != "Research and conclusions" || current.ContextMD != "Retained work" || len(current.Delegations) != 2 || current.Delegations[0].ExternalRef != "thread:1" || current.Title != "Spec v2" || len(current.Sources) != 2 || current.Report.Actions[0].SourceRef != "research" {
		t.Fatalf("scan lost work: %+v %v", current, err)
	}
	if _, err = a.Items(ctx, ItemFilters{DelegationStatus: "running"}); err == nil {
		t.Fatal("accepted invented status")
	}
}

func TestWorkValidationAndExplicitCorrections(t *testing.T) {
	ctx := context.Background()
	a, item, _ := workFixture(t)
	for _, input := range []UpdateItemWork{
		{ExpectedContentVersion: 1, Delegations: []DelegationPatch{{ID: "incomplete"}}},
		{ExpectedContentVersion: 1, Delegations: []DelegationPatch{{ID: "x"}, {ID: "x"}}},
		{ExpectedContentVersion: 1, Sources: []Source{{ID: "spec", URL: "https://evil.example/new", Label: "Changed", ObservedAt: a.Now()}}},
		{ExpectedContentVersion: 1, Sources: []Source{{ID: "new", SourceDate: "2026-10-03"}}},
	} {
		if _, err := a.UpdateItemWork(ctx, item.ID, input); err == nil {
			t.Fatalf("accepted invalid work: %+v", input)
		} else {
			var problem *Error
			if !errors.As(err, &problem) || problem.Code != "validation_error" {
				t.Fatalf("wrong failure: %v", err)
			}
		}
	}
	current, err := a.Item(ctx, item.ID)
	if err != nil || current.ContentVersion != 1 || len(current.Sources) != 1 {
		t.Fatalf("validation partially wrote: %+v %v", current, err)
	}
	if _, err = a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: 1, ContextMD: workText("Retain this")}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: 2, ContextMD: workText(""), Report: &Report{SchemaVersion: 1}}); err != nil {
		t.Fatal(err)
	}
	current, err = a.Item(ctx, item.ID)
	if err != nil || current.ContextMD != "" || current.Report.BodyMD != "" {
		t.Fatalf("explicit clear ignored: %+v %v", current, err)
	}
}

func TestDelegationSurvivesRestartAndPagedLookup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	a := New(s)
	for _, key := range []string{"first", "second", "third"} {
		if _, err = a.PutItem(ctx, "", PutItem{DedupeKey: key, Kind: "note", Title: key, Summary: "Research", Report: Report{SchemaVersion: 1}, Delegations: []DelegationPatch{{ID: "research", Executor: workText("agent:A"), ExternalRef: workText("session:" + key), InstructionsMD: workText("Research"), Status: workText("pending"), ContextMD: workText("Input v1; retained source snapshot")}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a = New(s)
	seen := map[string]bool{}
	var cursor string
	for {
		page, more, pageErr := a.ItemsPage(ctx, ItemFilters{DelegationStatus: "pending", Executor: "agent:A"}, cursor, 1)
		if pageErr != nil || len(page) != 1 {
			t.Fatalf("restarted page: %+v %v", page, pageErr)
		}
		if seen[page[0].ID] || len(page[0].MatchingDelegationIDs) != 1 || page[0].Delegations[0].ContextMD != "Input v1; retained source snapshot" {
			t.Fatalf("lost/repeated handoff: %+v", page)
		}
		seen[page[0].ID] = true
		if !more {
			break
		}
		cursor = page[0].ID
	}
	if len(seen) != 3 {
		t.Fatalf("missing work: %v", seen)
	}
}
