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

func TestOwnerReceiptAttributionCompatibility(t *testing.T) {
	ctx := context.Background()
	a, _, _ := workFixture(t)
	create := PutItem{RequestID: "legacy-create", DedupeKey: "legacy-receipt", Kind: "report", Title: "Original", Summary: "Owner capture", Report: Report{SchemaVersion: 1, BodyMD: "Original content"}}
	first, err := a.PutItem(ctx, "", create)
	if err != nil {
		t.Fatal(err)
	}
	var item Item
	if err = json.Unmarshal(first, &item); err != nil {
		t.Fatal(err)
	}
	create.Actor = "agent"
	replay, err := a.PutItem(ctx, "", create)
	if err != nil || string(first) != string(replay) {
		t.Fatalf("legacy create replay changed: %v", err)
	}
	create.Title = "Changed payload"
	_, err = a.PutItem(ctx, "", create)
	var problem *Error
	if !errors.As(err, &problem) || problem.Code != "idempotency_conflict" {
		t.Fatalf("changed legacy create accepted: %v", err)
	}
	note := SetUserNote{RequestID: "legacy-note", ExpectedStateVersion: 1, UserNote: "Owner instruction"}
	first, err = a.SetUserNote(ctx, item.ID, note)
	if err != nil {
		t.Fatal(err)
	}
	note.Actor = "agent"
	replay, err = a.SetUserNote(ctx, item.ID, note)
	if err != nil || string(first) != string(replay) {
		t.Fatalf("legacy note replay changed: %v", err)
	}
	note.Reason = "New request basis"
	_, err = a.SetUserNote(ctx, item.ID, note)
	if !errors.As(err, &problem) || problem.Code != "idempotency_conflict" {
		t.Fatalf("changed reason accepted: %v", err)
	}
	var count int
	if err = a.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE entity_id=? AND actor='user'", item.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy history was rewritten or duplicated: %d %v", count, err)
	}
	action := ApplyItemAction{RequestID: "attributed-action", Actor: "agent", ExpectedStateVersion: 2, Action: Action{Type: "set_todo", State: "todo"}}
	if _, err = a.ApplyItemAction(ctx, item.ID, action); err != nil {
		t.Fatal(err)
	}
	action.Actor = "user"
	_, err = a.ApplyItemAction(ctx, item.ID, action)
	if !errors.As(err, &problem) || problem.Code != "idempotency_conflict" {
		t.Fatalf("explicit actor change accepted: %v", err)
	}
}

func TestSimpleEditPreservesStateProvenanceAndIntake(t *testing.T) {
	ctx := context.Background()
	a, item, watch := workFixture(t)
	coverageBefore, err := a.Watch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetUserNote(ctx, item.ID, SetUserNote{ExpectedStateVersion: 1, UserNote: "Please clarify the heading"}); err != nil {
		t.Fatal(err)
	}
	before, err := a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := UpdateItemWork{RequestID: "simple-edit", ExpectedContentVersion: item.ContentVersion, Title: workText("Clarified heading"), Summary: workText("Corrected summary"), Reason: "Owner requested a clearer heading"}
	raw, err := a.UpdateItemWork(ctx, item.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	var edited Item
	if err = json.Unmarshal(raw, &edited); err != nil {
		t.Fatal(err)
	}
	edited, err = a.Item(ctx, edited.ID)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Title != *input.Title || edited.Summary != *input.Summary || edited.ContentVersion != item.ContentVersion+1 || edited.Origin != item.Origin || !reflect.DeepEqual(edited.WatchID, item.WatchID) || !reflect.DeepEqual(edited.Sources, item.Sources) || !reflect.DeepEqual(edited.Interests, item.Interests) || !reflect.DeepEqual(edited.Report, item.Report) || edited.StateVersion != before.StateVersion || edited.AcknowledgedContentVersion != before.AcknowledgedContentVersion || edited.UserNote != before.UserNote || len(edited.PendingInputs) != 1 || edited.PendingInputs[0].ID != before.PendingInputs[0].ID {
		t.Fatalf("edit changed ownership, state, or intake: %+v", edited)
	}
	var snapshot, actor, payload string
	if err = a.Store.DB.QueryRowContext(ctx, "SELECT snapshot FROM item_versions WHERE item_id=? AND content_version=?", item.ID, edited.ContentVersion).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	var recorded map[string]any
	if err = json.Unmarshal([]byte(snapshot), &recorded); err != nil || recorded["actor"] != "agent" || recorded["reason"] != input.Reason {
		t.Fatalf("missing version attribution: %s %v", snapshot, err)
	}
	if err = a.Store.DB.QueryRowContext(ctx, "SELECT actor,payload FROM events WHERE entity_id=? AND change_type='item.content_updated' ORDER BY seq DESC LIMIT 1", item.ID).Scan(&actor, &payload); err != nil || actor != "agent" {
		t.Fatalf("wrong event actor: %s %s %v", actor, payload, err)
	}
	replay, err := a.UpdateItemWork(ctx, item.ID, input)
	if err != nil || string(replay) != string(raw) {
		t.Fatalf("edit retry changed: %v", err)
	}
	input.RequestID = "stale-edit"
	if _, err = a.UpdateItemWork(ctx, item.ID, input); err == nil {
		t.Fatal("stale edit succeeded")
	}
	input.ExpectedContentVersion = edited.ContentVersion
	input.Title = workText("")
	if _, err = a.UpdateItemWork(ctx, item.ID, input); err == nil {
		t.Fatal("blank title succeeded")
	}
	latest, err := a.Watch(ctx, watch.ID)
	if err != nil || string(latest.Cursor) != string(coverageBefore.Cursor) || !latest.NextDueAt.Equal(coverageBefore.NextDueAt) {
		t.Fatalf("edit changed coverage: %+v %v", latest, err)
	}
}

func TestAgentOwnerMutationsRetainAttributionAndRequiredContext(t *testing.T) {
	ctx := context.Background()
	a, _, _ := workFixture(t)
	raw, err := a.PutItem(ctx, "", PutItem{Actor: "agent", DedupeKey: "dictated-input", Kind: "note", Title: "Owner instruction", Summary: "Update my Interests", Report: Report{SchemaVersion: 1, BodyMD: "Please update my Interests"}})
	if err != nil {
		t.Fatal(err)
	}
	var item Item
	if err = json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Origin != "user" || len(item.PendingInputs) != 1 {
		t.Fatalf("lost user intake: %+v", item)
	}
	if _, err = a.SetUserNote(ctx, item.ID, SetUserNote{Actor: "agent", Reason: "Transcribed owner instruction", ExpectedStateVersion: 1, UserNote: "Use the new scope"}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyItemAction(ctx, item.ID, ApplyItemAction{Actor: "agent", ExpectedStateVersion: 2, Action: Action{Type: "set_todo", State: "todo"}}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"item.created", "item.note_updated", "item.state_updated"} {
		var actor string
		if err = a.Store.DB.QueryRowContext(ctx, "SELECT actor FROM events WHERE entity_id=? AND change_type=? ORDER BY seq DESC LIMIT 1", item.ID, kind).Scan(&actor); err != nil || actor != "agent" {
			t.Fatalf("%s actor %s: %v", kind, actor, err)
		}
	}
	current, err := a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range current.PendingInputs {
		var original map[string]any
		if err = json.Unmarshal(input.Original, &original); err != nil || original["submitted_by"] != "agent" {
			t.Fatalf("input attribution lost: %s %v", input.Original, err)
		}
	}
	if _, err = a.SetUserNote(ctx, item.ID, SetUserNote{Actor: "unknown", ExpectedStateVersion: current.StateVersion, UserNote: "Wrong actor"}); err == nil {
		t.Fatal("invalid note actor succeeded")
	}
	if _, err = a.ApplyItemAction(ctx, item.ID, ApplyItemAction{Actor: "unknown", ExpectedStateVersion: current.StateVersion, Action: Action{Type: "set_todo", State: "done"}}); err == nil {
		t.Fatal("invalid action actor succeeded")
	}
	if _, err = a.PutItem(ctx, "", PutItem{Actor: "unknown"}); err == nil {
		t.Fatal("invalid content actor succeeded")
	}
	raw, err = a.StartRun(ctx, StartRun{WatchIDs: Field[[]string]{Set: true, Value: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Run     Run `json:"run"`
		Context struct {
			Changes []Event `json:"changes"`
			Inputs  struct {
				Items []UserInput `json:"items"`
			} `json:"user_inputs"`
		} `json:"context"`
	}
	if err = json.Unmarshal(raw, &started); err != nil || len(started.Context.Inputs.Items) != 2 {
		t.Fatalf("intake not captured: %s %v", raw, err)
	}
	seen := map[string]bool{}
	for _, event := range started.Context.Changes {
		if event.EntityID == item.ID {
			seen[event.ChangeType] = true
		}
	}
	if len(seen) != 3 {
		t.Fatalf("agent owner-state changes vanished: %v", seen)
	}
	var cursor string
	paged := map[string]bool{}
	for {
		page, err := a.ChangesPage(ctx, started.Run.AfterSeq, started.Run.ThroughSeq, cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range page["items"].([]Event) {
			if event.EntityID == item.ID {
				paged[event.ChangeType] = true
			}
		}
		cursor, _ = page["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if !reflect.DeepEqual(seen, paged) {
		t.Fatalf("paged changes disagree: %v %v", seen, paged)
	}
	if _, err = a.FinishRun(ctx, started.Run.ID, FinishRun{}); err == nil {
		t.Fatal("agent submission escaped input accounting")
	}
}

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
