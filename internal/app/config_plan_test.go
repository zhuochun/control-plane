package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestConfigPlanPreviewApplyAndReplay(t *testing.T) {
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
	plan := ConfigPlan{Operations: []ConfigOperation{
		{TargetType: "interest", Operation: "create", Payload: json.RawMessage(`{"slug":"delivery-risk","title":"Delivery risk","instructions_md":"Watch risk"}`)},
		{TargetType: "watch", Operation: "create", Payload: json.RawMessage(`{"slug":"gmail-risk","matching_policy":"explicit","interest_ids":["delivery-risk"],"source":{"kind":"gmail","locator":"risk-query"}}`)},
	}}
	preview, err := a.PreviewConfigPlan(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != 2 || preview.DueBefore != 0 || preview.DueAfter != 1 {
		t.Fatalf("bad preview: %+v", preview)
	}
	if interests, err := a.Interests(ctx); err != nil || len(interests) != 0 {
		t.Fatalf("preview wrote Interest: %+v %v", interests, err)
	}
	input := ApplyConfigPlan{RequestID: "apply-once", PreviewToken: preview.PreviewToken, Plan: plan}
	first, err := a.CommitConfigPlan(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var applied ConfigPreview
	if err = json.Unmarshal(first, &applied); err != nil {
		t.Fatal(err)
	}
	if applied.Changes[0].After.(map[string]any)["id"] != preview.Changes[0].After.(Interest).ID {
		t.Fatal("previewed and applied IDs differ")
	}
	replay, err := a.CommitConfigPlan(ctx, input)
	if err != nil || string(first) != string(replay) {
		t.Fatalf("receipt replay failed: %s %s %v", first, replay, err)
	}
	watch, err := a.Watch(ctx, "gmail-risk")
	if err != nil {
		t.Fatal(err)
	}
	interest, err := a.Interest(ctx, "delivery-risk")
	if err != nil {
		t.Fatal(err)
	}
	if len(watch.InterestIDs) != 1 || watch.InterestIDs[0] != interest.ID {
		t.Fatalf("atomic links failed: %+v", watch)
	}
	rename := ConfigPlan{Operations: []ConfigOperation{
		{TargetType: "interest", Operation: "update", Target: interest.Slug, ExpectedRevision: &interest.Revision, Payload: json.RawMessage(`{"slug":"delivery-priority"}`)},
		{TargetType: "watch", Operation: "update", Target: watch.Slug, ExpectedRevision: &watch.Revision, Payload: json.RawMessage(`{"slug":"gmail-priority"}`)},
	}}
	renamePreview, err := a.PreviewConfigPlan(ctx, rename)
	if err != nil {
		t.Fatal(err)
	}
	if renamePreview.Changes[0].Target != "delivery-priority" || renamePreview.Changes[1].Target != "gmail-priority" {
		t.Fatalf("rename preview lost new slugs: %+v", renamePreview)
	}
	if _, err = a.CommitConfigPlan(ctx, ApplyConfigPlan{RequestID: "rename-once", PreviewToken: renamePreview.PreviewToken, Plan: rename}); err != nil {
		t.Fatal(err)
	}
	renamedWatch, err := a.Watch(ctx, "gmail-priority")
	if err != nil || renamedWatch.ID != watch.ID || renamedWatch.SourceGeneration != watch.SourceGeneration {
		t.Fatalf("rename lost Watcher identity or source checkpoint: %+v %v", renamedWatch, err)
	}
	_, err = a.CommitConfigPlan(ctx, ApplyConfigPlan{RequestID: "stale-apply", PreviewToken: preview.PreviewToken, Plan: plan})
	if err == nil {
		t.Fatal("stale plan accepted")
	}
}
