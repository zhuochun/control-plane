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

func TestRunSelectionAndChangeAcknowledgement(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	now := time.Date(2026, 9, 15, 2, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	raw, err := a.CreateInterest(ctx, CreateInterest{RequestID: "interest", Title: "Releases", InstructionsMD: "Notice useful changes."})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	_ = json.Unmarshal(raw, &interest)
	raw, err = a.CreateWatch(ctx, CreateWatch{RequestID: "watch", InterestID: interest.ID, Source: WatchSource{Kind: "web", Locator: "https://example.com/releases"}, InstructionsMD: "Read release notes."})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	_ = json.Unmarshal(raw, &watch)
	raw, err = a.StartRun(ctx, StartRun{RequestID: "run-one", RunnerLabel: "test-runner"})
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Run   Run `json:"run"`
		Brief struct {
			Watches  []SelectedWatch   `json:"watches"`
			After    int64             `json:"after_seq"`
			Through  int64             `json:"through_seq"`
			Contexts map[string]string `json:"contexts"`
		} `json:"brief"`
	}
	if err = json.Unmarshal(raw, &started); err != nil {
		t.Fatal(err)
	}
	if len(started.Brief.Watches) != 1 || started.Brief.Watches[0].ID != watch.ID || started.Brief.Watches[0].IntervalSeconds != 7200 || started.Brief.Watches[0].LookbackSeconds != 604800 || started.Brief.Through != 2 {
		t.Fatalf("bad captured brief: %+v", started)
	}
	if started.Brief.Contexts["AGENTS.md"] == "" || started.Brief.Contexts["USER.md"] == "" {
		t.Fatalf("agent contexts missing from captured brief: %+v", started.Brief.Contexts)
	}
	history, err := a.Runs(ctx)
	if err != nil || len(history) != 1 || len(history[0].SelectedWatches) != 1 {
		t.Fatalf("run history omitted selected Watches: %+v %v", history, err)
	}
	historyJSON, err := json.Marshal(history)
	if err != nil || !strings.Contains(string(historyJSON), `"selected_watches"`) {
		t.Fatalf("run history JSON omitted selected Watches: %s %v", historyJSON, err)
	}
	if _, err = a.StartRun(ctx, StartRun{RequestID: "overlap", RunnerLabel: "other"}); err == nil {
		t.Fatal("allowed overlapping run")
	}
	changes, err := a.Changes(ctx, started.Brief.After, started.Brief.Through)
	if err != nil || len(changes) != 2 {
		t.Fatalf("human changes missing: %d %v", len(changes), err)
	}
	if _, err = a.SubmitWatchFindings(ctx, started.Run.ID, watch.ID, SubmitWatchFindings{RequestID: "failed-coverage", ExpectedWatchRevision: watch.Revision, Status: "failed", Error: "source unavailable", Coverage: Coverage{CursorBefore: nil, ObservedThrough: now, Limitations: []string{"source unavailable"}}}); err != nil {
		t.Fatal("recorded failed Watch coverage:", err)
	}
	if _, err = a.FinishRun(ctx, started.Run.ID, FinishRun{RequestID: "finish-failed", Summary: "Source unavailable", AckThroughSeq: &started.Brief.Through}); err == nil {
		t.Fatal("failed run acknowledged changes")
	}
	raw, err = a.FinishRun(ctx, started.Run.ID, FinishRun{RequestID: "finish", Summary: "Source unavailable"})
	if err != nil {
		t.Fatal(err)
	}
	var finished Run
	_ = json.Unmarshal(raw, &finished)
	if finished.Status != "failed" {
		t.Fatalf("unexpected status: %s", finished.Status)
	}
	empty := Field[[]string]{Set: true, Value: []string{}}
	raw, err = a.StartRun(ctx, StartRun{RequestID: "run-empty", RunnerLabel: "test-runner", WatchIDs: empty})
	if err != nil {
		t.Fatal(err)
	}
	var second struct {
		Run   Run `json:"run"`
		Brief struct {
			Through int64 `json:"through_seq"`
		} `json:"brief"`
	}
	_ = json.Unmarshal(raw, &second)
	if _, err = a.FinishRun(ctx, second.Run.ID, FinishRun{RequestID: "finish-empty", Summary: "Changes consumed", AckThroughSeq: &second.Brief.Through}); err != nil {
		t.Fatal(err)
	}
	var acknowledged string
	if err = s.DB.QueryRow(`SELECT value FROM settings WHERE key='acknowledged_event_seq'`).Scan(&acknowledged); err != nil || acknowledged != "2" {
		t.Fatalf("change cursor not advanced: %s %v", acknowledged, err)
	}
	now = now.Add(time.Minute)
	raw, err = a.StartRun(ctx, StartRun{RequestID: "long-run", RunnerLabel: "test-runner"})
	if err != nil {
		t.Fatal(err)
	}
	var longRun struct {
		Run Run `json:"run"`
	}
	_ = json.Unmarshal(raw, &longRun)
	now = now.Add(31 * time.Minute)
	if _, err = a.StartRun(ctx, StartRun{RequestID: "overlap-after-30-minutes", RunnerLabel: "test-runner", WatchIDs: empty}); err == nil {
		t.Fatal("long-running inspection lost the active run slot")
	}
	if _, err = a.SubmitWatchFindings(ctx, longRun.Run.ID, watch.ID, SubmitWatchFindings{RequestID: "long-run-coverage", ExpectedWatchRevision: watch.Revision, Status: "failed", Error: "source unavailable", Coverage: Coverage{CursorBefore: nil, ObservedThrough: now}}); err != nil {
		t.Fatal("long-running inspection could not submit coverage:", err)
	}
	if _, err = a.FinishRun(ctx, longRun.Run.ID, FinishRun{RequestID: "long-run-finish"}); err != nil {
		t.Fatal("long-running inspection could not finish:", err)
	}
}

func TestAbandonRunPreservesSubmittedCoverageAndReleasesSlot(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	now := time.Date(2026, 9, 23, 2, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	raw, err := a.CreateInterest(ctx, CreateInterest{Title: "Recovery", InstructionsMD: "Inspect sources"})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	if err = json.Unmarshal(raw, &interest); err != nil {
		t.Fatal(err)
	}
	watches := make([]Watch, 2)
	for index := range watches {
		raw, err = a.CreateWatch(ctx, CreateWatch{InterestID: interest.ID, Source: WatchSource{Kind: "web", Locator: fmt.Sprintf("https://example.com/%d", index)}})
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &watches[index]); err != nil {
			t.Fatal(err)
		}
	}
	raw, err = a.StartRun(ctx, StartRun{RequestID: "recover-start"})
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Run   Run `json:"run"`
		Brief struct {
			Through int64           `json:"through_seq"`
			Watches []SelectedWatch `json:"watches"`
		} `json:"brief"`
	}
	if err = json.Unmarshal(raw, &started); err != nil {
		t.Fatal(err)
	}
	if len(started.Brief.Watches) != 2 {
		t.Fatalf("selected %d Watches", len(started.Brief.Watches))
	}
	health, err := a.OperationalHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active, ok := health["active_run"].(ActiveRunHealth)
	if !ok || active.ID != started.Run.ID || active.SelectedCount != 2 || active.SubmittedCount != 0 || health["due_count"] != 2 {
		t.Fatalf("active health omitted work: %+v", health)
	}
	_, err = a.SubmitWatchFindings(ctx, started.Run.ID, watches[0].ID, SubmitWatchFindings{RequestID: "recover-result", ExpectedWatchRevision: watches[0].Revision, Status: "success", Coverage: Coverage{ObservedThrough: now, CursorAfter: json.RawMessage("null")}})
	if err != nil {
		t.Fatal(err)
	}
	health, err = a.OperationalHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active, ok = health["active_run"].(ActiveRunHealth)
	if !ok || active.SubmittedCount != 1 || health["due_count"] != 1 {
		t.Fatalf("health omitted committed coverage: %+v", health)
	}
	if _, err = a.AbandonRun(ctx, started.Run.ID, AbandonRun{RequestID: "missing-reason"}); err == nil {
		t.Fatal("accepted abandonment without reason")
	}
	raw, err = a.AbandonRun(ctx, started.Run.ID, AbandonRun{RequestID: "abandon", Reason: "agent exited"})
	if err != nil {
		t.Fatal(err)
	}
	var abandoned Run
	if err = json.Unmarshal(raw, &abandoned); err != nil {
		t.Fatal(err)
	}
	if abandoned.Status != "failed" || abandoned.EndedAt == nil || abandoned.Summary != "Abandoned by owner: agent exited" {
		t.Fatalf("bad abandoned run: %+v", abandoned)
	}
	if replay, replayErr := a.AbandonRun(ctx, started.Run.ID, AbandonRun{RequestID: "abandon", Reason: "agent exited"}); replayErr != nil || string(replay) != string(raw) {
		t.Fatalf("abandon replay: %s %v", replay, replayErr)
	}
	if _, err = a.SubmitWatchFindings(ctx, started.Run.ID, watches[1].ID, SubmitWatchFindings{Status: "failed"}); err == nil {
		t.Fatal("accepted late result")
	}
	if _, err = a.AbandonRun(ctx, started.Run.ID, AbandonRun{Reason: "again"}); err == nil {
		t.Fatal("abandoned a finished run")
	}
	var acknowledged string
	if err = s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='acknowledged_event_seq'`).Scan(&acknowledged); err != nil || acknowledged != "0" {
		t.Fatalf("changes acknowledged: %s %v", acknowledged, err)
	}
	raw, err = a.StartRun(ctx, StartRun{RequestID: "recover-next"})
	if err != nil {
		t.Fatal(err)
	}
	var next struct {
		Brief struct {
			Watches []SelectedWatch `json:"watches"`
			After   int64           `json:"after_seq"`
		} `json:"brief"`
	}
	if err = json.Unmarshal(raw, &next); err != nil {
		t.Fatal(err)
	}
	if len(next.Brief.Watches) != 1 || next.Brief.Watches[0].ID != watches[1].ID || next.Brief.After != 0 {
		t.Fatalf("next run lost due Watch or changes: %+v", next.Brief)
	}
	health, err = a.OperationalHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active, ok = health["active_run"].(ActiveRunHealth); !ok || active.SelectedCount != 1 || health["last_run"].(LastRunHealth).ID != active.ID {
		t.Fatalf("same-time recovery run omitted from health: %+v", health)
	}
}

func TestStartRunNormalizesInterestAndAttentionContext(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	now := time.Date(2026, 9, 16, 1, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	createInterest := func(request, title string) Interest {
		raw, createErr := a.CreateInterest(ctx, CreateInterest{RequestID: request, Title: title, InstructionsMD: "Shared Interest guidance."})
		if createErr != nil {
			t.Fatal(createErr)
		}
		var item Interest
		if json.Unmarshal(raw, &item) != nil {
			t.Fatal("decode Interest")
		}
		return item
	}
	watched := createInterest("watched-interest", "Watched interest")
	unwatched := createInterest("unwatched-interest", "Unwatched interest")
	watchRaw, err := a.CreateWatch(ctx, CreateWatch{RequestID: "context-watch", InterestID: watched.ID, Source: WatchSource{Kind: "fixture", Locator: "context"}})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	_ = json.Unmarshal(watchRaw, &watch)
	interestID := unwatched.ID
	_, err = a.PutItem(ctx, "", PutItem{RequestID: "attention-context", DedupeKey: "attention:unwatched", ExpectedContentVersion: 0, Kind: "note", InterestID: &interestID, Title: "Unwatched attention", Summary: "This must still be visible to the heartbeat.", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "Review this local finding."}, InitialTodoState: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := a.StartRun(ctx, StartRun{RequestID: "context-run"})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Brief struct {
			Interests []Interest        `json:"interests"`
			Attention []AttentionItem   `json:"attention_items"`
			Watches   []SelectedWatch   `json:"watches"`
			Contexts  map[string]string `json:"contexts"`
		} `json:"brief"`
	}
	if err = json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	if len(packet.Brief.Interests) != 2 || len(packet.Brief.Attention) != 1 || len(packet.Brief.Watches) != 1 || packet.Brief.Attention[0].Title != "Unwatched attention" {
		t.Fatalf("normalized packet omitted context: %+v", packet.Brief)
	}
	if packet.Brief.Watches[0].InterestID != watched.ID || packet.Brief.Contexts["AGENTS.md"] == "" {
		t.Fatalf("watch/context normalization failed: %+v", packet.Brief)
	}
	if strings.Contains(string(raw), "interest_instructions_md") {
		t.Fatal("Watch duplicated Interest instructions")
	}
}

func TestRunContextContinuationReturnsRemainingInterests(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	for index := 0; index < contextPageSize+1; index++ {
		if _, err = a.CreateInterest(ctx, CreateInterest{RequestID: fmt.Sprintf("interest-%d", index), Title: fmt.Sprintf("Interest %d", index)}); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := a.StartRun(ctx, StartRun{RequestID: "continuation-run", WatchIDs: Field[[]string]{Set: true, Value: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Brief struct {
			Interests     []Interest     `json:"interests"`
			Continuations map[string]any `json:"continuations"`
		} `json:"brief"`
	}
	if err = json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	cursor, ok := packet.Brief.Continuations["interests"].(string)
	if !ok || len(packet.Brief.Interests) != contextPageSize {
		t.Fatalf("expected bounded Interest page, packet=%+v", packet.Brief)
	}
	page, err := a.BriefPage(ctx, cursor)
	if err != nil {
		t.Fatal(err)
	}
	remaining, ok := page["items"].([]Interest)
	if !ok || len(remaining) != 1 {
		t.Fatalf("unexpected continuation page: %#v", page)
	}
	seen := map[string]bool{}
	for _, interest := range append(packet.Brief.Interests, remaining...) {
		if seen[interest.Title] {
			t.Fatalf("duplicate Interest across pages: %s", interest.Title)
		}
		seen[interest.Title] = true
	}
	for index := 0; index < contextPageSize+1; index++ {
		if !seen[fmt.Sprintf("Interest %d", index)] {
			t.Fatalf("Interest %d missing across pages", index)
		}
	}
}

func TestRunChangeContextUsesBoundedContinuationPages(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	for index := 0; index < contextPageSize+1; index++ {
		if _, err = a.CreateInterest(ctx, CreateInterest{RequestID: fmt.Sprintf("change-interest-%d", index), Title: fmt.Sprintf("Change %d", index)}); err != nil {
			t.Fatal(err)
		}
	}
	empty := Field[[]string]{Set: true, Value: []string{}}
	raw, err := a.StartRun(ctx, StartRun{RequestID: "change-page-run", WatchIDs: empty})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Brief struct {
			After      int64   `json:"after_seq"`
			Through    int64   `json:"through_seq"`
			Changes    []Event `json:"changes"`
			NextCursor string  `json:"changes_next_cursor"`
		} `json:"brief"`
	}
	if err = json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	if len(packet.Brief.Changes) != contextPageSize || packet.Brief.NextCursor == "" {
		t.Fatalf("expected a bounded change page with continuation: %+v", packet.Brief)
	}
	page, err := a.ChangesPage(ctx, packet.Brief.After, packet.Brief.Through, packet.Brief.NextCursor, contextPageSize)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := page["items"].([]Event)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected continuation page: %#v", page)
	}
	if _, ok = page["next_cursor"]; ok {
		t.Fatalf("unexpected third change page: %#v", page)
	}
}
