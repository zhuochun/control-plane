package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestRunsPageMatchesFullHistory(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 7; i++ {
		_, err = s.DB.ExecContext(ctx, `INSERT INTO runs(id,runner_label,status,started_at,ended_at,lease_expires_at,selected_watches,after_seq,through_seq,context_snapshot) VALUES(?, 'fixture', 'completed', ?, ?, 0, '[]', 0, 0, '{}')`, fmt.Sprintf("run-%d", i), i/2, i/2)
		if err != nil {
			t.Fatal(err)
		}
	}
	a := New(s)
	all, err := a.Runs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got, want []string
	for _, run := range all {
		want = append(want, run.ID)
	}
	cursor := ""
	for {
		page, more, pageErr := a.RunsPage(ctx, cursor, 2)
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		for _, run := range page {
			got = append(got, run.ID)
		}
		if !more {
			break
		}
		cursor = page[len(page)-1].ID
	}
	if !slices.Equal(got, want) {
		t.Fatalf("paged history: got %v, want %v", got, want)
	}
	if _, _, err = a.RunsPage(ctx, "missing", 2); err == nil {
		t.Fatal("accepted missing cursor")
	}
}

func TestRunDetailProjectsRelatedItemsWithoutChangingCapture(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fullSummary := strings.Repeat("Full related Item summary. ", 30)
	selected, err := json.Marshal([]SelectedWatch{{ID: "watch", RelatedItems: []CapturedAttentionItem{{ID: "item", Title: "Related", Summary: fullSummary}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO runs(id,runner_label,status,started_at,ended_at,lease_expires_at,selected_watches,after_seq,through_seq,context_snapshot) VALUES('abcd1234','fixture','completed',1,2,0,?,0,0,'{}')`, string(selected)); err != nil {
		t.Fatal(err)
	}
	detail, err := New(s).RunDetail(ctx, "abcd1234")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		SelectedWatches []struct {
			RelatedItems []AttentionItem `json:"related_items"`
		} `json:"selected_watches"`
	}
	if err = json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.SelectedWatches) != 1 || len(response.SelectedWatches[0].RelatedItems) != 1 {
		t.Fatalf("missing related Item in Run detail: %s", encoded)
	}
	item := response.SelectedWatches[0].RelatedItems[0]
	var captured string
	if err = s.DB.QueryRowContext(ctx, `SELECT selected_watches FROM runs WHERE id='abcd1234'`).Scan(&captured); err != nil {
		t.Fatal(err)
	}
	if len(item.Summary) > attentionSummaryMaxBytes || !slices.Contains(item.TruncatedFields, "summary") || !strings.Contains(captured, fullSummary) || strings.Contains(captured, "truncated_fields") {
		t.Fatalf("Run detail or capture lost its intended shape: %s", encoded)
	}
}

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
	raw, err = a.CreateWatch(ctx, CreateWatch{RequestID: "watch", InterestIDs: []string{interest.ID}, MatchingPolicy: "explicit", Source: WatchSource{Kind: "web", Locator: "https://example.com/releases"}, InstructionsMD: "Read release notes."})
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
		} `json:"context"`
	}
	if err = json.Unmarshal(raw, &started); err != nil {
		t.Fatal(err)
	}
	if len(started.Brief.Watches) != 1 || started.Brief.Watches[0].ID != watch.ID || started.Brief.Watches[0].IntervalSeconds != 7200 || started.Brief.Watches[0].LookbackSeconds != 604800 || started.Run.ThroughSeq != 2 {
		t.Fatalf("bad captured brief: %+v", started)
	}
	if started.Brief.Contexts["AGENTS.md"] == "" || started.Brief.Contexts["USER.md"] == "" {
		t.Fatalf("agent contexts missing from captured brief: %+v", started.Brief.Contexts)
	}
	history, err := a.Runs(ctx)
	if err != nil || len(history) != 1 || history[0].SelectedCount != 1 {
		t.Fatalf("run history omitted selected Watches: %+v %v", history, err)
	}
	historyJSON, err := json.Marshal(history)
	if err != nil || strings.Contains(string(historyJSON), `"selected_watches"`) || !strings.Contains(string(historyJSON), `"selected_count":1`) {
		t.Fatalf("run history JSON omitted selected Watches: %s %v", historyJSON, err)
	}
	if _, err = a.StartRun(ctx, StartRun{RequestID: "overlap", RunnerLabel: "other"}); err == nil {
		t.Fatal("allowed overlapping run")
	}
	changes, err := a.Changes(ctx, started.Run.AfterSeq, started.Run.ThroughSeq)
	if err != nil || len(changes) != 2 {
		t.Fatalf("human changes missing: %d %v", len(changes), err)
	}
	if _, err = a.SubmitWatchFindings(ctx, started.Run.ID, watch.ID, SubmitWatchFindings{RequestID: "failed-coverage", ExpectedWatchRevision: watch.Revision, Status: "failed", Error: "source unavailable", Coverage: Coverage{CursorBefore: nil, ObservedThrough: now, Limitations: []string{"source unavailable"}}}); err != nil {
		t.Fatal("recorded failed Watch coverage:", err)
	}
	if _, err = a.FinishRun(ctx, started.Run.ID, FinishRun{RequestID: "finish-failed", Summary: "Source unavailable", AckThroughSeq: &started.Run.ThroughSeq}); err == nil {
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
		} `json:"context"`
	}
	_ = json.Unmarshal(raw, &second)
	if _, err = a.FinishRun(ctx, second.Run.ID, FinishRun{RequestID: "finish-empty", Summary: "Changes consumed", AckThroughSeq: &second.Run.ThroughSeq}); err != nil {
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
		raw, err = a.CreateWatch(ctx, CreateWatch{InterestIDs: []string{interest.ID}, MatchingPolicy: "explicit", Source: WatchSource{Kind: "web", Locator: fmt.Sprintf("https://example.com/%d", index)}})
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
		} `json:"context"`
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
		Run   Run `json:"run"`
		Brief struct {
			Watches []SelectedWatch `json:"watches"`
		} `json:"context"`
	}
	if err = json.Unmarshal(raw, &next); err != nil {
		t.Fatal(err)
	}
	if len(next.Brief.Watches) != 1 || next.Brief.Watches[0].ID != watches[1].ID || next.Run.AfterSeq != 0 {
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
	watchRaw, err := a.CreateWatch(ctx, CreateWatch{RequestID: "context-watch", InterestIDs: []string{watched.ID}, MatchingPolicy: "explicit", Source: WatchSource{Kind: "fixture", Locator: "context"}})
	if err != nil {
		t.Fatal(err)
	}
	var watch Watch
	_ = json.Unmarshal(watchRaw, &watch)
	interestID := unwatched.ID
	longSummary := strings.Repeat("This must still be visible to the heartbeat. ", 20)
	itemRaw, err := a.PutItem(ctx, "", PutItem{RequestID: "attention-context", DedupeKey: "attention:unwatched", ExpectedContentVersion: 0, Kind: "note", Interests: []ItemInterest{{ID: interestID, Reason: "Local context"}}, Title: "Unwatched attention", Summary: longSummary, Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "Review this local finding."}, InitialTodoState: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	var stored Item
	if err = json.Unmarshal(itemRaw, &stored); err != nil {
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
			Available map[string]struct {
				Cursor string `json:"cursor"`
			} `json:"available"`
		} `json:"context"`
	}
	if err = json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	if len(packet.Brief.Interests) != 1 || len(packet.Brief.Attention) != 0 || len(packet.Brief.Watches) != 1 {
		t.Fatalf("normalized packet omitted context: %+v", packet.Brief)
	}
	optional, err := a.BriefPage(ctx, packet.Brief.Available["attention"].Cursor)
	if err != nil {
		t.Fatal(err)
	}
	packet.Brief.Attention = optional["items"].([]AttentionItem)
	if len(packet.Brief.Attention) != 1 || packet.Brief.Attention[0].Title != "Unwatched attention" || optional["consistency"] != "captured" {
		t.Fatalf("optional captured Attention: %#v", optional)
	}
	if len(packet.Brief.Attention[0].Summary) > attentionSummaryMaxBytes || !slices.Contains(packet.Brief.Attention[0].TruncatedFields, "summary") {
		t.Fatalf("long summary was not marked and capped: %+v", packet.Brief.Attention[0])
	}
	var captured, receipt string
	if err = s.DB.QueryRowContext(ctx, `SELECT context_snapshot FROM runs WHERE id=(SELECT id FROM runs WHERE status='running')`).Scan(&captured); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT response FROM command_receipts WHERE request_id='context-run'`).Scan(&receipt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(captured, longSummary) || strings.Contains(receipt, longSummary) || !strings.Contains(receipt, `"context"`) || strings.Contains(captured, "truncated_fields") {
		t.Fatal("Run snapshot or receipt stored output truncation")
	}
	replay, replayErr := a.StartRun(ctx, StartRun{RequestID: "context-run"})
	if replayErr != nil || string(replay) != string(raw) {
		t.Fatalf("start Run replay changed rendered packet: %s %v", replay, replayErr)
	}
	full, err := a.Item(ctx, stored.ID)
	if err != nil || full.Summary != longSummary {
		t.Fatalf("full Item detail changed: %+v %v", full, err)
	}
	if len(packet.Brief.Watches[0].Interests) != 1 || packet.Brief.Watches[0].Interests[0].ID != watched.ID || packet.Brief.Contexts["AGENTS.md"] == "" {
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
	if _, err = a.CreateWatch(ctx, CreateWatch{RequestID: "continuation-watch", MatchingPolicy: "broad", Source: WatchSource{Kind: "fixture", Locator: "all-interests"}}); err != nil {
		t.Fatal(err)
	}
	raw, err := a.StartRun(ctx, StartRun{RequestID: "continuation-run"})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Brief struct {
			Interests     []Interest     `json:"interests"`
			Continuations map[string]any `json:"continuations"`
		} `json:"context"`
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
	if _, repeated := page["contexts"]; repeated {
		t.Fatal("continuation repeated AGENTS.md and USER.md")
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

func TestContextContinuationPageUsesItemAndByteBounds(t *testing.T) {
	items := make([]CapturedAttentionItem, contextPageSize+contextContinuationPageSize+1)
	for index := range items {
		items[index] = CapturedAttentionItem{ID: fmt.Sprint(index), Title: "Item", Summary: "Short summary"}
	}
	first, err := contextPage("run", "attention_items", items, 0, contextPageSize)
	if err != nil || len(first["items"].([]CapturedAttentionItem)) != contextPageSize {
		t.Fatalf("first page size: %#v %v", first, err)
	}
	continuation, err := contextPage("run", "attention_items", items, contextPageSize, contextContinuationPageSize)
	if err != nil || len(continuation["items"].([]CapturedAttentionItem)) != contextContinuationPageSize || continuation["next_cursor"] == nil {
		t.Fatalf("continuation page size: %#v %v", continuation, err)
	}
	for index := range items {
		items[index].Summary = strings.Repeat("long summary ", 120)
	}
	bounded, err := contextPageProjected("run", "attention_items", items, contextPageSize, contextContinuationPageSize, compactAttentionItem)
	if err != nil {
		t.Fatal(err)
	}
	pageItems := bounded["items"].([]AttentionItem)
	encoded, err := json.Marshal(pageItems)
	if err != nil || len(pageItems) < 1 || len(pageItems) >= contextContinuationPageSize || len(encoded) > contextPageBytes || len(pageItems[0].Summary) > attentionSummaryMaxBytes || !slices.Contains(pageItems[0].TruncatedFields, "summary") {
		t.Fatalf("byte cap failed: %d items, %d bytes, %v", len(pageItems), len(encoded), err)
	}
}

func TestAttentionProjectionBoundsProseAndPreservesDetail(t *testing.T) {
	item := Item{ID: "item", Kind: "report", Title: strings.Repeat("界", 100), Summary: strings.Repeat("🙂", 200), ContentVersion: 2, AcknowledgedContentVersion: 1, StateVersion: 7, Interests: []ItemInterest{{ID: "one", Reason: strings.Repeat("é", 150)}, {ID: "two", Reason: "Short reason"}}}
	projected := compactAttentionItem(attentionItem(item))
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{{"title", projected.Title, attentionTitleMaxBytes}, {"summary", projected.Summary, attentionSummaryMaxBytes}, {"reason", projected.Interests[0].Reason, attentionReasonMaxBytes}} {
		if len(field.value) > field.limit || !utf8.ValidString(field.value) || !strings.HasSuffix(field.value, "…") {
			t.Errorf("%s cap: %d bytes, %q", field.name, len(field.value), field.value)
		}
	}
	if !slices.Equal(projected.TruncatedFields, []string{"title", "summary", "interest_reason"}) || projected.Interests[1].Reason != "Short reason" {
		t.Fatalf("projection lost meaning: %+v", projected)
	}
	if len(item.Title) != 300 || len(item.Summary) != 800 || len(item.Interests[0].Reason) != 300 {
		t.Fatal("projection mutated the full Item")
	}
	encoded, err := json.Marshal(projected)
	if err != nil || strings.Contains(string(encoded), "acknowledged_content_version") || strings.Contains(string(encoded), "state_version") || strings.Contains(string(encoded), "unacknowledged") {
		t.Fatalf("projection exposed internal versions: %s %v", encoded, err)
	}
}

func TestAcknowledgementSincePreviousRunIsAChangeNotAttention(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	raw, err := a.PutItem(ctx, "", PutItem{RequestID: "item-before-ack", DedupeKey: "ack-flow", ExpectedContentVersion: 0, Kind: "note", Title: "Already read", Summary: "No open task", Sources: []Source{}, Report: Report{SchemaVersion: 1, BodyMD: "Body"}})
	if err != nil {
		t.Fatal(err)
	}
	var item Item
	if err = json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE settings SET value=(SELECT CAST(MAX(seq) AS TEXT) FROM events) WHERE key='acknowledged_event_seq'`); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ApplyItemAction(ctx, item.ID, ApplyItemAction{RequestID: "ack-between-runs", ExpectedStateVersion: item.StateVersion, Action: Action{Type: "acknowledge", ContentVersion: item.ContentVersion}}); err != nil {
		t.Fatal(err)
	}
	empty := Field[[]string]{Set: true, Value: []string{}}
	raw, err = a.StartRun(ctx, StartRun{RequestID: "run-after-ack", WatchIDs: empty})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct {
		Brief struct {
			Attention []AttentionItem `json:"attention_items"`
			Changes   []Event         `json:"changes"`
		} `json:"context"`
	}
	if err = json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	if len(packet.Brief.Attention) != 0 || len(packet.Brief.Changes) != 1 || packet.Brief.Changes[0].ChangeType != "item.state_updated" || !strings.Contains(string(packet.Brief.Changes[0].Payload), `"type":"acknowledge"`) {
		t.Fatalf("acknowledgement was not represented by the change range: %+v", packet.Brief)
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
		Run   Run `json:"run"`
		Brief struct {
			After         int64             `json:"after_seq"`
			Through       int64             `json:"through_seq"`
			Changes       []Event           `json:"changes"`
			NextCursor    string            `json:"changes_next_cursor"`
			Continuations map[string]string `json:"continuations"`
		} `json:"context"`
	}
	if err = json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	packet.Brief.NextCursor = packet.Brief.Continuations["changes"]
	if len(packet.Brief.Changes) != contextPageSize || packet.Brief.NextCursor == "" {
		t.Fatalf("expected a bounded change page with continuation: %+v", packet.Brief)
	}
	page, err := a.ChangesPage(ctx, packet.Run.AfterSeq, packet.Run.ThroughSeq, packet.Brief.NextCursor, contextPageSize)
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
