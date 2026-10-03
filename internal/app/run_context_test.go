package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestQuietRunKeepsLargeAttentionOptionalAndCaptured(t *testing.T) {
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
	now := time.Date(2026, 9, 30, 1, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	if _, err = s.DB.ExecContext(ctx, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<10000)
 INSERT INTO items(id,dedupe_key,kind,origin,title,summary,content,content_version,content_hash,todo_state,state_version,created_at,content_updated_at,state_updated_at)
 SELECT printf('item-%05d',x),printf('quiet:%d',x),'task','user','Old todo','Unchanged work','{"sources":[],"report":{"schema_version":1,"body_md":"Keep this"}}',1,'hash','todo',1,1,1,1 FROM n`); err != nil {
		t.Fatal(err)
	}
	raw, err := a.StartRun(ctx, StartRun{RequestID: "quiet-large", WatchIDs: Field[[]string]{Set: true, Value: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Run     Run `json:"run"`
		Context struct {
			Interests []Interest `json:"interests"`
			Overview  struct {
				Count int `json:"attention_count"`
			} `json:"overview"`
			Available map[string]struct {
				Cursor string `json:"cursor"`
			} `json:"available"`
		} `json:"context"`
	}
	if err = json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	if packet.Context.Overview.Count != 10000 || len(packet.Context.Interests) != 0 || len(raw) > 20000 || strings.Contains(string(raw), "Old todo") || strings.Contains(string(raw), `"brief"`) {
		t.Fatalf("quiet Run still enumerates backlog: %d bytes %s", len(raw), raw)
	}
	page, err := a.BriefPage(ctx, packet.Context.Available["attention"].Cursor)
	if err != nil {
		t.Fatal(err)
	}
	items := page["items"].([]AttentionItem)
	if len(items) == 0 || page["consistency"] != "captured" {
		t.Fatalf("optional snapshot missing: %#v", page)
	}
	original := items[0]
	if _, err = s.DB.ExecContext(ctx, "UPDATE items SET summary='Changed later',todo_state='done',acknowledged_content_version=content_version WHERE id=?", original.ID); err != nil {
		t.Fatal(err)
	}
	replay, err := a.StartRun(ctx, StartRun{RequestID: "quiet-large", WatchIDs: Field[[]string]{Set: true, Value: []string{}}})
	if err != nil || string(replay) != string(raw) {
		t.Fatalf("receipt drifted: %v", err)
	}
	same, err := a.BriefPage(ctx, packet.Context.Available["attention"].Cursor)
	if err != nil || same["items"].([]AttentionItem)[0].Summary != original.Summary {
		t.Fatalf("captured optional context drifted: %#v %v", same, err)
	}
	if _, err = a.FinishRun(ctx, packet.Run.ID, FinishRun{RequestID: "quiet-finish", Summary: "No selected source work"}); err != nil {
		t.Fatal(err)
	}
	history, _, err := a.RunsPage(ctx, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(history)
	if strings.Contains(string(encoded), "selected_watches") || strings.Contains(string(encoded), "Old todo") || history[0].SelectedCount != 0 {
		t.Fatalf("history enumerated capture: %s", encoded)
	}
}

func TestBriefFailuresAndRunCoverageFollowSourceGeneration(t *testing.T) {
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
	now := time.Date(2026, 9, 30, 1, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	interestRaw, err := a.CreateInterest(ctx, CreateInterest{Title: "Focus"})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	if err = json.Unmarshal(interestRaw, &interest); err != nil {
		t.Fatal(err)
	}
	raw, err := a.CreateWatch(ctx, CreateWatch{MatchingPolicy: "explicit", InterestIDs: []string{interest.ID}, Source: WatchSource{Kind: "fixture", Locator: "A"}})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	if err = json.Unmarshal(raw, &watch); err != nil {
		t.Fatal(err)
	}
	started, err := a.StartRun(ctx, StartRun{RequestID: "failure-run"})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Run     Run `json:"run"`
		Context struct {
			Watches []SelectedWatch `json:"watches"`
		} `json:"context"`
	}
	if err = json.Unmarshal(started, &packet); err != nil {
		t.Fatal(err)
	}
	limitation := strings.Repeat("unread source ", 70)
	if _, err = a.SubmitWatchFindings(ctx, packet.Run.ID, watch.ID, SubmitWatchFindings{ExpectedWatchRevision: watch.Revision, Status: "partial", Error: limitation, Coverage: Coverage{ObservedThrough: now, Limitations: []string{"Last page unread"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.FinishRun(ctx, packet.Run.ID, FinishRun{Summary: "Incomplete"}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.UpdateWatch(ctx, watch.ID, UpdateWatch{ExpectedRevision: watch.Revision, State: Field[string]{Set: true, Value: "paused"}}); err != nil {
		t.Fatal(err)
	}
	brief, err := a.Brief(ctx)
	if err != nil {
		t.Fatal(err)
	}
	failure := brief["unresolved_failures"].(map[string]any)
	handles := failure["items"].([]watcherHandle)
	if failure["count"] != 1 || len(handles) != 1 || handles[0].State != "paused" || len(handles[0].TruncatedFields) != 1 || brief["due_watches"].(map[string]any)["count"] != 0 {
		t.Fatalf("paused failure was hidden: %#v", brief)
	}
	if _, ok := brief["contexts"]; ok {
		t.Fatal("brief included full contexts")
	}
	watch, err = a.Watch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.UpdateWatch(ctx, watch.ID, UpdateWatch{ExpectedRevision: watch.Revision, State: Field[string]{Set: true, Value: "active"}}); err != nil {
		t.Fatal(err)
	}
	started, err = a.StartRun(ctx, StartRun{RequestID: "retry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(started, &packet); err != nil {
		t.Fatal(err)
	}
	attempt := packet.Context.Watches[0].PreviousAttempt
	if attempt == nil || attempt.Status != "partial" || attempt.Error != limitation || len(attempt.Coverage.Limitations) != 1 {
		t.Fatalf("retry lacks full captured limitation: %+v", attempt)
	}
	if _, err = a.AbandonRun(ctx, packet.Run.ID, AbandonRun{Reason: "Test source replacement"}); err != nil {
		t.Fatal(err)
	}
	watch, err = a.Watch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.UpdateWatch(ctx, watch.ID, UpdateWatch{ExpectedRevision: watch.Revision, Source: Field[WatchSource]{Set: true, Value: WatchSource{Kind: "fixture", Locator: "B"}}}); err != nil {
		t.Fatal(err)
	}
	brief, err = a.Brief(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if brief["unresolved_failures"].(map[string]any)["count"] != 0 {
		t.Fatalf("old source failure became current: %#v", brief)
	}
	health, err := a.OperationalHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if health["watches"].([]WatchHealth)[0].LastError != "" {
		t.Fatalf("full health contradicts brief: %#v", health)
	}
	replay, err := a.StartRun(ctx, StartRun{RequestID: "retry-run"})
	if err != nil || string(replay) != string(started) {
		t.Fatalf("captured failure drifted after replacement: %v", err)
	}
}

func TestLiveBriefHandlesPageWithoutClaimingOrAcknowledging(t *testing.T) {
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
	for i := 0; i < 22; i++ {
		if _, err = a.CreateInterest(ctx, CreateInterest{Title: fmt.Sprintf("Focus %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	started, err := a.StartRun(ctx, StartRun{RequestID: "brief-active", WatchIDs: Field[[]string]{Set: true, Value: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Run Run `json:"run"`
	}
	if err = json.Unmarshal(started, &packet); err != nil {
		t.Fatal(err)
	}
	brief, err := a.Brief(ctx)
	if err != nil {
		t.Fatal(err)
	}
	focus := brief["focus"].(map[string]any)
	if focus["count"] != 22 || len(focus["items"].([]focusHandle)) != 20 || brief["active_run"].(RunSummary).ID != packet.Run.ID {
		t.Fatalf("bad live overview: %#v", brief)
	}
	page, err := a.BriefPage(ctx, focus["next_cursor"].(string))
	if err != nil || len(page["items"].([]focusHandle)) != 2 || page["consistency"] != "live" {
		t.Fatalf("bad handle page: %#v %v", page, err)
	}
	var ack string
	var running int
	if err = s.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='acknowledged_event_seq'").Scan(&ack); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRowContext(ctx, "SELECT count(*) FROM runs WHERE status='running'").Scan(&running); err != nil {
		t.Fatal(err)
	}
	if ack != "0" || running != 1 {
		t.Fatalf("brief mutated state: ack=%s running=%d", ack, running)
	}
	if _, err = a.BriefPage(ctx, encodeContextCursor(contextCursor{RunID: "missing", Collection: "attention_items"})); err == nil {
		t.Fatal("unknown Run cursor fell back to live data")
	}
	if _, err = a.BriefPage(ctx, encodeContextCursor(contextCursor{RunID: packet.Run.ID, Collection: "focus"})); err == nil {
		t.Fatal("invalid captured collection fell back to live data")
	}
}

func TestVersionSixStartReceiptReplaysNewCapturedContract(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			if err := s.Close(); err != nil {
				t.Errorf("close store: %v", err)
			}
		}
	}()
	a := New(s)
	ids := []string{}
	for i := 0; i < 85; i++ {
		raw, createErr := a.CreateInterest(ctx, CreateInterest{Title: fmt.Sprintf("Migration focus %02d", i), InstructionsMD: fmt.Sprintf("Captured instructions %02d", i)})
		if createErr != nil {
			t.Fatal(createErr)
		}
		var interest Interest
		if err = json.Unmarshal(raw, &interest); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, interest.ID)
	}
	raw, err := a.CreateWatch(ctx, CreateWatch{MatchingPolicy: "explicit", InterestIDs: ids[10:70], Source: WatchSource{Kind: "fixture", Locator: "migration"}})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	if err = json.Unmarshal(raw, &watch); err != nil {
		t.Fatal(err)
	}
	fullSummary := strings.Repeat("Preserved optional summary 界 ", 40)
	if _, err = a.PutItem(ctx, "", PutItem{DedupeKey: "migration:optional", Kind: "task", Title: "Retain historical evidence", Summary: fullSummary, InitialTodoState: "todo", Report: Report{SchemaVersion: 1, BodyMD: "Full original content"}}); err != nil {
		t.Fatal(err)
	}
	custom := "# Owner edited protocol\nKeep my custom guidance."
	if _, err = a.SetSettings(ctx, SetSettings{Timezone: "browser", AgentsMD: &custom}); err != nil {
		t.Fatal(err)
	}
	input := StartRun{RequestID: "migration-replay", RunnerLabel: "migration"}
	started, err := a.StartRun(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var original struct {
		Run     Run `json:"run"`
		Context struct {
			Changes       []Event           `json:"changes"`
			Continuations map[string]string `json:"continuations"`
		} `json:"context"`
	}
	if err = json.Unmarshal(started, &original); err != nil {
		t.Fatal(err)
	}
	var snapshotRaw, selectedRaw string
	if err = s.DB.QueryRowContext(ctx, "SELECT context_snapshot,selected_watches FROM runs WHERE id=?", original.Run.ID).Scan(&snapshotRaw, &selectedRaw); err != nil {
		t.Fatal(err)
	}
	var snapshot runContextSnapshot
	if err = json.Unmarshal([]byte(snapshotRaw), &snapshot); err != nil {
		t.Fatal(err)
	}
	var selected []SelectedWatch
	if err = json.Unmarshal([]byte(selectedRaw), &selected); err != nil {
		t.Fatal(err)
	}
	// Model a v6 receipt, including the global Interest page and duplicated scope
	// that older adapters emitted. The database schema itself is unchanged by v7.
	runFields := map[string]any{"id": original.Run.ID, "runner_label": original.Run.RunnerLabel, "status": "running", "started_at": original.Run.StartedAt, "ended_at": nil, "after_seq": original.Run.AfterSeq, "through_seq": original.Run.ThroughSeq, "summary": "", "selected_watches": selected}
	legacy := map[string]any{"run": runFields, "brief": map[string]any{
		"contexts": snapshot.Contexts, "interests": snapshot.Interests[:50], "attention_items": snapshot.Attention, "watches": selected,
		"changes": original.Context.Changes, "changes_next_cursor": original.Context.Continuations["changes"], "more_due_count": 0,
		"continuations": map[string]string{"interests": encodeContextCursor(contextCursor{RunID: original.Run.ID, Collection: "interests", Offset: 50})},
	}}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, "UPDATE command_receipts SET response=? WHERE request_id=?", string(legacyJSON), input.RequestID); err != nil {
		t.Fatal(err)
	}
	if _, err = a.UpdateInterest(ctx, ids[10], UpdateInterest{ExpectedRevision: 1, InstructionsMD: Field[string]{Set: true, Value: "Changed live instructions"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, "DELETE FROM goose_db_version WHERE version_id>=7"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = nil
	s, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a = New(s)
	replay, err := a.StartRun(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Run     Run `json:"run"`
		Context struct {
			Interests     []Interest        `json:"interests"`
			Changes       []Event           `json:"changes"`
			Continuations map[string]string `json:"continuations"`
			Contexts      map[string]string `json:"contexts"`
			Available     map[string]struct {
				Cursor string `json:"cursor"`
			} `json:"available"`
		} `json:"context"`
	}
	if err = json.Unmarshal(replay, &packet); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(replay), `"brief"`) || strings.Contains(string(replay), `"selected_watches"`) || len(packet.Context.Interests) != 50 || packet.Context.Contexts["AGENTS.md"] != custom || packet.Context.Interests[0].InstructionsMD != "Captured instructions 10" {
		t.Fatalf("migration reconstructed live or wrong scope: %s", replay)
	}
	page, err := a.BriefPage(ctx, packet.Context.Continuations["interests"])
	if err != nil {
		t.Fatal(err)
	}
	if len(page["items"].([]Interest)) != 10 {
		t.Fatalf("migrated required Interest pages incomplete: %#v", page)
	}
	changes, err := a.ChangesPage(ctx, packet.Run.AfterSeq, packet.Run.ThroughSeq, packet.Context.Continuations["changes"], 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes["items"].([]Event))+len(packet.Context.Changes) != int(packet.Run.ThroughSeq) {
		t.Fatalf("migration lost captured events: %#v", changes)
	}
	attention, err := a.BriefPage(ctx, packet.Context.Available["attention"].Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(attention["items"].([]AttentionItem)) != 1 {
		t.Fatalf("optional migration context missing: %#v", attention)
	}
	var retained string
	if err = s.DB.QueryRowContext(ctx, "SELECT context_snapshot FROM runs WHERE id=?", packet.Run.ID).Scan(&retained); err != nil || !strings.Contains(retained, fullSummary) {
		t.Fatalf("full optional capture lost: %v", err)
	}
	settings, err := a.Settings(ctx)
	if err != nil || settings.AgentsMD != custom || strings.Contains(settings.DefaultAgentsMD, "consume every Interest, Attention") {
		t.Fatalf("migration overwrote owner guidance or left stale defaults: %+v %v", settings, err)
	}
	again, err := a.StartRun(ctx, input)
	if err != nil || string(again) != string(replay) {
		t.Fatalf("migrated retry changed: %v", err)
	}
	if _, err = a.SubmitWatchFindings(ctx, packet.Run.ID, watch.ID, SubmitWatchFindings{ExpectedWatchRevision: 1, Status: "success", Coverage: Coverage{CursorAfter: json.RawMessage("null"), ObservedThrough: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.FinishRun(ctx, packet.Run.ID, FinishRun{Summary: "Migrated source attempt", AckThroughSeq: &packet.Run.ThroughSeq}); err != nil {
		t.Fatal(err)
	}
}
