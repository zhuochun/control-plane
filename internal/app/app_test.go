package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/zhuochun/control-plane/internal/store"
)

func TestSettingRetryDoesNotOverwriteLaterEdit(t *testing.T) {
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
	first := SetSettings{RequestID: "first", Timezone: "Asia/Singapore"}
	original, err := a.SetSettings(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	var saved Settings
	if err = json.Unmarshal(original, &saved); err != nil || saved.ContextGuidance == nil || saved.ContextGuidance.UpdatePolicy == "" {
		t.Fatalf("settings save lost guidance: %s %v", original, err)
	}
	var payload string
	if err = s.DB.QueryRowContext(ctx, "SELECT payload FROM events WHERE change_type='settings.updated' ORDER BY seq DESC LIMIT 1").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err = json.Unmarshal([]byte(payload), &event); err != nil || event["context_guidance"] != nil {
		t.Fatalf("read-time help leaked into captured changes: %s %v", payload, err)
	}
	if _, err := a.SetSettings(ctx, SetSettings{RequestID: "second", Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	replay, err := a.SetSettings(ctx, first)
	if err != nil || string(original) != string(replay) {
		t.Fatalf("receipt mismatch: %s %v", replay, err)
	}
	current, _ := a.Settings(ctx)
	if current.ContextGuidance.AgentsMD == "" || current.ContextGuidance.UserMD == "" || current.ContextGuidance.ItemContext == "" || current.ContextGuidance.UpdatePolicy == "" {
		t.Fatal("settings omitted contextual editing guidance")
	}
	if current.Timezone != "UTC" {
		t.Fatal("retry overwrote later edit")
	}
	var count int
	if err := s.DB.QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("retry duplicated event: %d", count)
	}
	_, err = a.SetSettings(ctx, SetSettings{RequestID: "first", Timezone: "Europe/London"})
	var conflict *Error
	if !errors.As(err, &conflict) || conflict.Code != "idempotency_conflict" {
		t.Fatalf("expected conflict, got %v", err)
	}
	_, err = a.SetSettings(ctx, SetSettings{RequestID: "invalid", Timezone: "No/SuchZone"})
	if err == nil {
		t.Fatal("accepted invalid timezone")
	}
	if err := s.DB.QueryRow("SELECT count(*) FROM command_receipts WHERE request_id = 'invalid'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed command committed receipt")
	}
}
