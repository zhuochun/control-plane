package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestWatcherInterestMigrationPreservesHistory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "aicp.db"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := fs.Sub(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO interests(id,title,instructions_md,state,revision,created_at,updated_at) VALUES('i-1','Focus','Why','active',2,1000,1000)`,
		`INSERT INTO watches(id,interest_id,source,instructions_md,interval_seconds,lookback_seconds,state,revision,cursor,next_due_at,created_at,updated_at) VALUES('w-1','i-1','{"kind":"fixture","locator":"inbox"}','Read',60,600,'active',3,'{"page":9}',2000,1000,1000)`,
		`INSERT INTO items(id,dedupe_key,kind,interest_id,watch_id,title,summary,content,content_version,content_hash,todo_state,state_version,created_at,content_updated_at,state_updated_at) VALUES('item-1','matter','note','i-1','w-1','Title','Summary','{"sources":[],"report":{"schema_version":1,"body_md":"Body"}}',1,'hash','todo',1,1000,1000,1000)`,
		`INSERT INTO runs(id,runner_label,status,started_at,ended_at,lease_expires_at,selected_watches,after_seq,through_seq,summary,context_snapshot) VALUES('r-1','agent','completed',1000,1100,2000,'[{"id":"w-1","revision":3,"interest_id":"i-1","interest_revision":2,"source":{"kind":"fixture","locator":"inbox"}}]',0,0,'done','{"interests":[{"id":"i-1","instructions_md":"Why"}]}')`,
		`INSERT INTO proposals(id,proposal_key,target_type,operation,payload,rationale_md,state) VALUES('p-1','legacy-watch','watch','create','{"interest_id":"i-1","source":{"kind":"fixture","locator":"other"}}','Find missing input','pending')`,
	} {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.ExecContext(ctx, "UPDATE settings SET value='Custom guidance' WHERE key='agents_md'"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE settings SET value='Custom owner context' WHERE key='user_md'"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var policy, cursor, slug string
	if err = store.DB.QueryRowContext(ctx, "SELECT matching_policy,cursor,slug FROM watches WHERE id='w-1'").Scan(&policy, &cursor, &slug); err != nil {
		t.Fatal(err)
	}
	if policy != "explicit" || cursor != `{"page":9}` || slug == "" {
		t.Fatalf("lost watcher state: %s %s %s", policy, cursor, slug)
	}
	var linked int
	if err = store.DB.QueryRowContext(ctx, "SELECT count(*) FROM watch_interests WHERE watch_id='w-1' AND interest_id='i-1'").Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if linked != 1 {
		t.Fatal("lost Watcher Interest link")
	}
	var selected string
	if err = store.DB.QueryRowContext(ctx, "SELECT selected_watches FROM runs WHERE id='r-1'").Scan(&selected); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(selected, `"interests":[{"id":"i-1","revision":2}]`) || !strings.Contains(selected, `"source_generation":1`) || !strings.Contains(selected, `"matching_policy":"explicit"`) {
		t.Fatalf("lost historical snapshot: %s", selected)
	}
	var proposalPayload string
	if err = store.DB.QueryRowContext(ctx, "SELECT payload FROM proposals WHERE id='p-1'").Scan(&proposalPayload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(proposalPayload, `"interest_id"`) || !strings.Contains(proposalPayload, `"interest_ids":["i-1"]`) {
		t.Fatalf("old proposal was not converted: %s", proposalPayload)
	}
	var guidance string
	if err = store.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='agents_md'").Scan(&guidance); err != nil {
		t.Fatal(err)
	}
	if guidance != "Custom guidance" {
		t.Fatalf("migration overwrote owner guidance: %q", guidance)
	}
	var ownerContext string
	if err = store.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='user_md'").Scan(&ownerContext); err != nil {
		t.Fatal(err)
	}
	if ownerContext != "Custom owner context" {
		t.Fatalf("migration overwrote owner context: %q", ownerContext)
	}
	var defaultGuidance string
	if err = store.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='default_agents_md'").Scan(&defaultGuidance); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(defaultGuidance, "Watchers define bounded source inputs") {
		t.Fatalf("missing current reset guidance: %q", defaultGuidance)
	}
}

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
	var guidance string
	if err := reopened.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='agents_md'").Scan(&guidance); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(guidance, "Watchers define bounded source inputs") {
		t.Fatalf("seeded guidance is stale: %q", guidance)
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
