package app

import (
	"context"
	"errors"
	"github.com/zhuochun/control-plane/internal/store"
	"testing"
)

func TestSettingRetryDoesNotOverwriteLaterEdit(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	first := SetSettings{RequestID: "first", Timezone: "Asia/Singapore"}
	original, err := a.SetSettings(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetSettings(ctx, SetSettings{RequestID: "second", Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	replay, err := a.SetSettings(ctx, first)
	if err != nil || string(original) != string(replay) {
		t.Fatalf("receipt mismatch: %s %v", replay, err)
	}
	current, _ := a.Settings(ctx)
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
