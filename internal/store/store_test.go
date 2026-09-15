package store

import (
	"context"
	"strings"
	"testing"
)

func TestPersistenceAndExclusiveOwnership(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	s, err := Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, "UPDATE settings SET value = ? WHERE key = 'timezone'", "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(ctx, directory)
	if err == nil {
		_ = other.Close()
		t.Fatal("second server acquired the same directory")
	}
	if !strings.Contains(err.Error(), "in use") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var timezone string
	if err := reopened.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'timezone'").Scan(&timezone); err != nil {
		t.Fatal(err)
	}
	if timezone != "Asia/Singapore" {
		t.Fatalf("lost setting: %q", timezone)
	}
	for name, want := range map[string]int{"foreign_keys": 1, "synchronous": 2, "busy_timeout": 5000} {
		var got int
		if err := reopened.DB.QueryRowContext(ctx, "PRAGMA "+name).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
}

func TestFutureSchemaFailsWithoutReset(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	s, err := Open(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO goose_db_version(version_id, is_applied) VALUES (99, 1)"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	future, err := Open(ctx, directory)
	if err == nil {
		_ = future.Close()
		t.Fatal("accepted newer schema")
	}
	if !strings.Contains(err.Error(), "upgrade") {
		t.Fatalf("expected upgrade guidance: %v", err)
	}
}
