package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zhuochun/control-plane/internal/store"
)

func reviewFixture(t *testing.T) (*App, Item, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a := New(s)
	t.Cleanup(func() {
		if err := a.Store.Close(); err != nil {
			t.Error(err)
		}
	})
	format := ReviewFormat{ID: "architecture", Version: 1, Title: "Architecture choice", Fields: []FormatField{
		{ID: "approach", Type: "choice", Selection: "single", Required: true},
		{ID: "features", Type: "choice", Selection: "multiple", MaxSelections: 2},
		{ID: "feedback", Type: "text_input"},
	}}
	if _, err = a.RegisterReviewFormat(ctx, RegisterReviewFormat{Format: format}); err != nil {
		t.Fatal(err)
	}
	report := Report{SchemaVersion: 2, BodyMD: "Review the options", Blocks: []ReportBlock{
		{ID: "intro", Type: "markdown", BodyMD: "Choose deliberately"},
		{ID: "decision", Type: "review", Title: "Rendering approach", Format: &FormatRef{ID: format.ID, Version: 1}, Blocks: []ReportBlock{
			{ID: "approach", Type: "choice", Selection: "single", Required: true, Question: "Which approach?", Options: []ReviewOption{{ID: "local", Label: "Local rendering"}, {ID: "remote", Label: "External service"}}},
			{ID: "features", Type: "choice", Selection: "multiple", MaxSelections: 2, Question: "Which features?", Options: []ReviewOption{{ID: "mermaid", Label: "Mermaid"}, {ID: "plantuml", Label: "PlantUML"}}},
			{ID: "feedback", Type: "text_input", Question: "Additional instructions"},
		}},
	}}
	raw, err := a.PutItem(ctx, "", PutItem{DedupeKey: "review:1", Kind: "report", Title: "Architecture review", Summary: "Choose a renderer", Report: report})
	if err != nil {
		t.Fatal(err)
	}
	var item Item
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	return a, item, dir
}

func reviewSubmission(item Item) SubmitReviewAnswer {
	return SubmitReviewAnswer{RequestID: "answer", ReviewID: "decision", ExpectedContentVersion: item.ContentVersion, ExpectedStateVersion: item.StateVersion, ReviewMaterialHash: item.ReviewMaterialHash, Values: []AnswerValue{
		{FieldID: "approach", Disposition: "answered", SelectedIDs: []string{"local"}},
		{FieldID: "features", Disposition: "answered", SelectedIDs: []string{}},
		{FieldID: "feedback", Disposition: "answered", Text: "Avoid external services."},
	}}
}

func TestReviewAnswersPersistReplayAndCorrect(t *testing.T) {
	ctx := context.Background()
	a, item, dir := reviewFixture(t)
	input := reviewSubmission(item)
	raw, err := a.SubmitReviewAnswer(ctx, item.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	current, err := a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Answers) != 1 || !current.Answers[0].Applicable || !strings.Contains(current.Answers[0].Note, "Which approach?: Local rendering") || current.Answers[0].Values[1].Answer != "Selected none" || current.StateVersion != item.StateVersion+1 || current.ContentVersion != item.ContentVersion || current.TodoState != item.TodoState || current.UserNote != item.UserNote || current.AcknowledgedContentVersion != item.AcknowledgedContentVersion {
		t.Fatalf("answer/state mismatch: %+v", current)
	}
	if err := a.Store.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a.Store = s
	replay, err := a.SubmitReviewAnswer(ctx, item.ID, input)
	if err != nil || string(raw) != string(replay) {
		t.Fatalf("replay failed: %s %v", replay, err)
	}
	current, err = a.Item(ctx, item.ID)
	if err != nil || len(current.Answers) != 1 {
		t.Fatalf("persistence: %+v %v", current.Answers, err)
	}
	correction := reviewSubmission(current)
	correction.RequestID = "correction"
	correction.Supersedes = current.Answers[0].ID
	correction.Values[0].SelectedIDs = []string{"remote"}
	if _, err := a.SubmitReviewAnswer(ctx, item.ID, correction); err != nil {
		t.Fatal(err)
	}
	current, err = a.Item(ctx, item.ID)
	if err != nil || len(current.Answers) != 1 || current.Answers[0].Values[0].Answer != "External service" {
		t.Fatalf("correction failed: %+v %v", current, err)
	}
	history, err := a.AnswerHistory(ctx, item.ID, 0)
	if err != nil || len(history.Items) != 2 || history.Items[1].Supersedes != history.Items[0].ID {
		t.Fatalf("history failed: %+v %v", history, err)
	}
	var events int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE change_type='item.answer_submitted'").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("duplicate answer events: %d", events)
	}
}

func TestReviewSubmissionFailuresWriteNothing(t *testing.T) {
	ctx := context.Background()
	a, item, _ := reviewFixture(t)
	tests := []struct {
		name, code string
		change     func(*SubmitReviewAnswer)
	}{
		{"stale content", "content_conflict", func(v *SubmitReviewAnswer) { v.ExpectedContentVersion++ }},
		{"stale state", "state_conflict", func(v *SubmitReviewAnswer) { v.ExpectedStateVersion++ }},
		{"wrong material", "content_conflict", func(v *SubmitReviewAnswer) { v.ReviewMaterialHash = "wrong" }},
		{"unknown option", "validation_error", func(v *SubmitReviewAnswer) { v.Values[0].SelectedIDs = []string{"missing"} }},
		{"duplicate option", "validation_error", func(v *SubmitReviewAnswer) { v.Values[1].SelectedIDs = []string{"mermaid", "mermaid"} }},
		{"missing field", "validation_error", func(v *SubmitReviewAnswer) { v.Values = v.Values[:1] }},
		{"required skipped", "validation_error", func(v *SubmitReviewAnswer) { v.Values[0] = AnswerValue{FieldID: "approach", Disposition: "skipped"} }},
		{"unknown field", "validation_error", func(v *SubmitReviewAnswer) {
			v.Values = append(v.Values, AnswerValue{FieldID: "unknown", Disposition: "skipped"})
		}},
		{"wrong predecessor", "answer_conflict", func(v *SubmitReviewAnswer) { v.Supersedes = "missing" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := reviewSubmission(item)
			input.RequestID = test.name
			test.change(&input)
			_, err := a.SubmitReviewAnswer(ctx, item.ID, input)
			var problem *Error
			if !errors.As(err, &problem) || problem.Code != test.code {
				t.Fatalf("expected %s: %v", test.code, err)
			}
		})
	}
	var count int
	if err := a.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM item_answers").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed submission wrote %d answers", count)
	}
	current, err := a.Item(ctx, item.ID)
	if err != nil || current.StateVersion != item.StateVersion {
		t.Fatalf("failure changed state: %v", err)
	}
}

func TestReviewMaterialTracksEvidenceNotHandoff(t *testing.T) {
	ctx := context.Background()
	a, item, _ := reviewFixture(t)
	if _, err := a.SubmitReviewAnswer(ctx, item.ID, reviewSubmission(item)); err != nil {
		t.Fatal(err)
	}
	context := "Executor progress changed"
	if _, err := a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: item.ContentVersion, ContextMD: &context}); err != nil {
		t.Fatal(err)
	}
	current, err := a.Item(ctx, item.ID)
	if err != nil || current.ReviewMaterialHash != item.ReviewMaterialHash || !current.Answers[0].Applicable {
		t.Fatalf("handoff invalidated answer: %+v %v", current, err)
	}
	current.Report.Blocks[1].Blocks[0].Options[0].Label = "Revised local rendering"
	if _, err := a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: current.ContentVersion, Report: &current.Report}); err != nil {
		t.Fatal(err)
	}
	current, err = a.Item(ctx, item.ID)
	if err != nil || current.Answers[0].Applicable || current.Answers[0].Values[0].Answer != "Local rendering" {
		t.Fatalf("changed option lost snapshot: %+v %v", current, err)
	}
}
