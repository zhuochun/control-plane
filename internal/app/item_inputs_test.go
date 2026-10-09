package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/zhuochun/control-plane/internal/store"
)

func inputFixture(t *testing.T) (*App, Item) {
	t.Helper()
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	a := New(s)
	raw, err := a.PutItem(context.Background(), "", PutItem{DedupeKey: "user:input", Kind: "note", Title: "Inbox", Summary: "Original request", Report: Report{SchemaVersion: 1, BodyMD: "Original request"}})
	if err != nil {
		t.Fatal(err)
	}
	var item Item
	if err = json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	return a, item
}

func inputRun(t *testing.T, a *App) Run {
	t.Helper()
	raw, err := a.StartRun(context.Background(), StartRun{})
	if err != nil {
		t.Fatal(err)
	}
	var packet struct{ Run Run }
	if err = json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	return packet.Run
}

func requireInputError(t *testing.T, err error, code string) {
	t.Helper()
	var problem *Error
	if !errors.As(err, &problem) || problem.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func processCommand(item Item, run Run, id, outcome string) ProcessItemInput {
	return ProcessItemInput{RunID: run.ID, InputID: id, ExpectedContentVersion: item.ContentVersion, ExpectedStateVersion: item.StateVersion, Outcome: outcome, ResultMD: "Visible result and next step"}
}

func TestInputRecoveryAndArchive(t *testing.T) {
	ctx := context.Background()
	a, item := inputFixture(t)
	run := inputRun(t, a)
	id := item.PendingInputs[0].ID
	_, err := a.FinishRun(ctx, run.ID, FinishRun{})
	requireInputError(t, err, "input_handling_missing")
	failed := processCommand(item, run, id, "failed")
	if _, err = a.ProcessItemInput(ctx, item.ID, failed); err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(item.PendingInputs) != 1 || len(item.PendingInputs[0].Attempts) != 1 || item.Report.BodyMD == "Original request" {
		t.Fatalf("failure not visible or input lost: %+v", item)
	}
	command := processCommand(item, run, id, "responded")
	command.RequestID = "recover-input"
	command.Archive = true
	response, err := a.ProcessItemInput(ctx, item.ID, command)
	if err != nil {
		t.Fatal(err)
	}
	finished, err := a.FinishRun(ctx, run.ID, FinishRun{})
	if err != nil {
		t.Fatal(err)
	}
	var closed Run
	if err = json.Unmarshal(finished, &closed); err != nil {
		t.Fatal(err)
	}
	if closed.Status != "completed" {
		t.Fatalf("recovered input should complete: %s", closed.Status)
	}
	replay, err := a.ProcessItemInput(ctx, item.ID, command)
	if err != nil || string(replay) != string(response) {
		t.Fatalf("closed-run replay changed: %v", err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	history, err := a.InputHistory(ctx, item.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	prior := history["items"].([]UserInput)
	if len(item.PendingInputs) != 0 || !item.InboxArchived || len(prior) != 1 || prior[0].AttemptCount != 2 || len(prior[0].Attempts) != 1 || prior[0].Text != "Original request" {
		t.Fatalf("history/archive lost: %+v", item)
	}
	command.RequestID = "late-new-write"
	_, err = a.ProcessItemInput(ctx, item.ID, command)
	requireInputError(t, err, "run_finished")
}

func TestNoteSupersessionAndStateFence(t *testing.T) {
	ctx := context.Background()
	a, item := inputFixture(t)
	_, err := a.SetUserNote(ctx, item.ID, SetUserNote{ExpectedStateVersion: item.StateVersion, UserNote: "A"})
	if err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	var noteID string
	for _, entry := range item.PendingInputs {
		if entry.Kind == "note" {
			noteID = entry.ID
		}
	}
	run := inputRun(t, a)
	stale := item
	_, err = a.SetUserNote(ctx, item.ID, SetUserNote{ExpectedStateVersion: item.StateVersion, UserNote: "B"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.ProcessItemInput(ctx, item.ID, processCommand(stale, run, noteID, "responded"))
	requireInputError(t, err, "input_conflict")
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	var newer string
	for _, entry := range item.PendingInputs {
		if entry.Kind == "note" {
			newer = entry.ID
		}
	}
	_, err = a.ProcessItemInput(ctx, item.ID, processCommand(item, run, newer, "responded"))
	requireInputError(t, err, "input_conflict")
	for _, entry := range item.PendingInputs {
		if entry.Kind == "inbox" {
			if _, err = a.ProcessItemInput(ctx, item.ID, processCommand(item, run, entry.ID, "responded")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err = a.FinishRun(ctx, run.ID, FinishRun{}); err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.UserNote != "B" {
		t.Fatalf("newer note cleared: %q", item.UserNote)
	}
	run = inputRun(t, a)
	before := item.StateVersion
	if _, err = a.ProcessItemInput(ctx, item.ID, processCommand(item, run, newer, "responded")); err != nil {
		t.Fatal(err)
	}
	_, err = a.SetUserNote(ctx, item.ID, SetUserNote{ExpectedStateVersion: before, UserNote: "B"})
	requireInputError(t, err, "state_conflict")
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.UserNote != "" || item.StateVersion != before+1 {
		t.Fatalf("note clearing not fenced: %+v", item)
	}
}

func TestFailedInputIsRetriedAfterAcknowledgement(t *testing.T) {
	ctx := context.Background()
	a, item := inputFixture(t)
	run := inputRun(t, a)
	id := item.PendingInputs[0].ID
	if _, err := a.ProcessItemInput(ctx, item.ID, processCommand(item, run, id, "blocked")); err != nil {
		t.Fatal(err)
	}
	raw, err := a.FinishRun(ctx, run.ID, FinishRun{})
	if err != nil {
		t.Fatal(err)
	}
	var closed Run
	if err = json.Unmarshal(raw, &closed); err != nil {
		t.Fatal(err)
	}
	if closed.Status != "failed" {
		t.Fatalf("input-only failure became %s", closed.Status)
	}
	if _, err = a.Store.DB.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='acknowledged_event_seq'", run.ThroughSeq); err != nil {
		t.Fatal(err)
	}
	next := inputRun(t, a)
	snapshot, err := loadRunContext(ctx, a.Store.DB, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.UserInputs) != 1 || snapshot.UserInputs[0].ID != id {
		t.Fatalf("ack lost pending input: %+v", snapshot.UserInputs)
	}
}

func TestInputRelevanceAndUserStatePreserved(t *testing.T) {
	ctx := context.Background()
	a, item := inputFixture(t)
	raw, err := a.CreateInterest(ctx, CreateInterest{Title: "References"})
	if err != nil {
		t.Fatal(err)
	}
	var interest Interest
	if err = json.Unmarshal(raw, &interest); err != nil {
		t.Fatal(err)
	}
	run := inputRun(t, a)
	command := processCommand(item, run, item.PendingInputs[0].ID, "incorporated")
	relevance := []ItemInterest{{ID: interest.ID, Reason: "User provided reference"}}
	command.Interests = &relevance
	if _, err = a.ProcessItemInput(ctx, item.ID, command); err != nil {
		t.Fatal(err)
	}
	updated, err := a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Origin != item.Origin || updated.StateVersion != item.StateVersion || updated.TodoState != item.TodoState || len(updated.Interests) != 1 {
		t.Fatalf("ownership/relevance mismatch: %+v", updated)
	}
}

func TestFinishedInputSummaryStaysFrozenAfterRecovery(t *testing.T) {
	ctx := context.Background()
	a, item := inputFixture(t)
	run := inputRun(t, a)
	id := item.PendingInputs[0].ID
	if _, err := a.ProcessItemInput(ctx, item.ID, processCommand(item, run, id, "failed")); err != nil {
		t.Fatal(err)
	}
	finish := FinishRun{RequestID: "finish-failed-once", Summary: "Input needs another attempt"}
	if _, err := a.FinishRun(ctx, run.ID, finish); err != nil {
		t.Fatal(err)
	}
	detail, err := a.RunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := json.Marshal(detail["input_summary"])
	if err != nil {
		t.Fatal(err)
	}
	if detail["input_summary"].(map[string]any)["status"] != "failed" {
		t.Fatalf("wrong closed input status: %s", original)
	}
	next := inputRun(t, a)
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ProcessItemInput(ctx, item.ID, processCommand(item, next, id, "responded")); err != nil {
		t.Fatal(err)
	}
	if _, err = a.FinishRun(ctx, next.ID, FinishRun{}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.FinishRun(ctx, run.ID, finish); err != nil {
		t.Fatal(err)
	}
	detail, err = a.RunDetail(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(detail["input_summary"])
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("historical counts changed after recovery: %s -> %s", original, after)
	}
	var count int
	if err = a.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE entity_type='run' AND entity_id=? AND change_type='run.finished'", run.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("completion replay duplicated events: %d %v", count, err)
	}
}

func TestFollowUpRequiresOpenMergedDelegation(t *testing.T) {
	ctx := context.Background()
	a, item := inputFixture(t)
	closed := "closed"
	instructions := "Old finished work"
	contextMD := "Finished; no continuation"
	if _, err := a.UpdateItemWork(ctx, item.ID, UpdateItemWork{ExpectedContentVersion: item.ContentVersion, Delegations: []DelegationPatch{{ID: "old", Executor: workText("executor"), InstructionsMD: &instructions, Status: &closed, ContextMD: &contextMD}}}); err != nil {
		t.Fatal(err)
	}
	item, err := a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	run := inputRun(t, a)
	command := processCommand(item, run, item.PendingInputs[0].ID, "follow_up")
	_, err = a.ProcessItemInput(ctx, item.ID, command)
	requireInputError(t, err, "validation_error")
	after, err := a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ContentVersion != item.ContentVersion || len(after.PendingInputs) != 1 {
		t.Fatal("rejected follow-up changed content/input")
	}
	open := "pending"
	contextMD = "Awaiting executor launch; continue this Item"
	command.Delegations = []DelegationPatch{{ID: "next", Executor: workText("executor"), InstructionsMD: workText("Compare options"), Status: &open, ContextMD: &contextMD}}
	if _, err = a.ProcessItemInput(ctx, item.ID, command); err != nil {
		t.Fatal(err)
	}
}

func TestAttemptHistoryIsBoundedAndComplete(t *testing.T) {
	ctx := context.Background()
	a, item := inputFixture(t)
	run := inputRun(t, a)
	id := item.PendingInputs[0].ID
	for i := 0; i < 25; i++ {
		command := processCommand(item, run, id, "failed")
		command.ResultMD = fmt.Sprintf("Attempt %d failed; user guidance needed", i)
		if _, err := a.ProcessItemInput(ctx, item.ID, command); err != nil {
			t.Fatal(err)
		}
		var err error
		item, err = a.Item(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(item.PendingInputs[0].Attempts) != 1 || item.PendingInputs[0].AttemptCount != 25 || item.PendingInputs[0].Attempts[0].ResultMD != "Attempt 24 failed; user guidance needed" {
		t.Fatalf("unbounded or stale summary: %+v", item.PendingInputs[0])
	}
	page, err := a.InputAttempts(ctx, item.ID, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page["items"].([]InputAttempt)) != 20 || page["next_offset"] != 20 {
		t.Fatalf("wrong first page: %+v", page)
	}
	next, err := a.InputAttempts(ctx, item.ID, id, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(next["items"].([]InputAttempt)) != 5 || next["next_offset"] != nil {
		t.Fatalf("wrong final page: %+v", next)
	}
}
