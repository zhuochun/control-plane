package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestRunLeaseSelectionAndChangeAcknowledgement(t *testing.T) {
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
	if _, err = a.StartRun(ctx, StartRun{RequestID: "overlap", RunnerLabel: "other"}); err == nil {
		t.Fatal("allowed overlapping run")
	}
	changes, err := a.Changes(ctx, started.Brief.After, started.Brief.Through)
	if err != nil || len(changes) != 2 {
		t.Fatalf("human changes missing: %d %v", len(changes), err)
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
	raw, err = a.StartRun(ctx, StartRun{RequestID: "lease", RunnerLabel: "test-runner", WatchIDs: empty})
	if err != nil {
		t.Fatal(err)
	}
	var leased struct {
		Run Run `json:"run"`
	}
	_ = json.Unmarshal(raw, &leased)
	now = now.Add(31 * time.Minute)
	if _, err = a.RenewRun(ctx, leased.Run.ID, RenewRun{RequestID: "late-renew"}); err == nil {
		t.Fatal("renewed an expired lease")
	}
	if _, err = a.StartRun(ctx, StartRun{RequestID: "after-expiry", RunnerLabel: "test-runner", WatchIDs: empty}); err != nil {
		t.Fatal("expired lease blocked a new run:", err)
	}
}
