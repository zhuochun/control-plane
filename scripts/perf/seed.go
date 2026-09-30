// Run from the repository root: go run ./scripts/perf/seed.go -data-dir DIR -profile mature
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/store"
)

type profile struct{ items, runs, interests, watches, attentionPercent int }

var profiles = map[string]profile{
	"fresh":  {100, 30, 3, 2, 1},
	"mature": {10000, 3000, 8, 6, 1},
	"large":  {100000, 30000, 8, 6, 1},
	"skewed": {20000, 10000, 5, 4, 20},
}

func id(kind string, n int) string { return fmt.Sprintf("%08x-0000-4000-8000-%012x", len(kind), n) }
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func exec(stmt *sql.Stmt, args ...any)           { _, err := stmt.Exec(args...); must(err) }
func prepare(tx *sql.Tx, query string) *sql.Stmt { s, err := tx.Prepare(query); must(err); return s }

// Match the application's content identity for seeded records that may later
// receive real HTTP updates. Keep this in sync with app.contentHash.
func fixtureHash(input app.PutItem) string {
	input.Sources = append([]app.Source{}, input.Sources...)
	input.Interests = append([]app.ItemInterest{}, input.Interests...)
	for i := range input.Sources {
		input.Sources[i].ObservedAt = time.Time{}
	}
	sort.Slice(input.Sources, func(i, j int) bool { return input.Sources[i].ID < input.Sources[j].ID })
	sort.Slice(input.Interests, func(i, j int) bool { return input.Interests[i].ID < input.Interests[j].ID })
	input.Report.Actions = []app.ReportAction{}
	data, err := json.Marshal(input)
	must(err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func main() {
	var directory, name string
	var attentionPercent int
	flag.StringVar(&directory, "data-dir", "", "new isolated data directory")
	flag.StringVar(&name, "profile", "mature", "fresh, mature, large, or skewed")
	flag.IntVar(&attentionPercent, "attention-percent", -1, "override percentage of Items in Attention (0-100)")
	flag.Parse()
	p, ok := profiles[name]
	if !ok || directory == "" {
		panic("provide -data-dir and a known -profile")
	}
	if attentionPercent != -1 {
		if attentionPercent < 0 || attentionPercent > 100 {
			panic("attention-percent must be between 0 and 100")
		}
		p.attentionPercent = attentionPercent
	}
	absolute, err := filepath.Abs(directory)
	must(err)
	if _, err = os.Stat(absolute); err == nil {
		panic("refusing to seed an existing directory")
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	ctx := context.Background()
	s, err := store.Open(ctx, absolute)
	must(err)
	defer func() {
		if err := s.Close(); err != nil {
			log.Printf("close store: %v", err)
		}
	}()
	tx, err := s.DB.BeginTx(ctx, nil)
	must(err)
	defer func() { _ = tx.Rollback() }()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC).UnixMilli()
	interest := prepare(tx, `INSERT INTO interests(id,title,instructions_md,state,revision,created_at,updated_at,slug) VALUES(?,?,?,?,1,?,?,?)`)
	watch := prepare(tx, `INSERT INTO watches(id,source,instructions_md,interval_seconds,lookback_seconds,state,revision,cursor,next_due_at,created_at,updated_at,slug,matching_policy,source_generation) VALUES(?,?,?,86400,604800,?,1,NULL,?,?,?,?,'explicit',1)`)
	watchInterest := prepare(tx, `INSERT INTO watch_interests(watch_id,interest_id,linked_at) VALUES(?,?,?)`)
	activeInterests := p.interests
	if p.interests > 3 {
		activeInterests -= 2
	}
	for i := 0; i < p.interests; i++ {
		state := "active"
		if i >= p.interests-2 && p.interests > 3 {
			state = "deprecated"
		}
		exec(interest, id("interest", i), fmt.Sprintf("Interest %d", i), "Track material changes in this area.", state, base, base, fmt.Sprintf("interest-%d", i))
	}
	for i := 0; i < p.watches; i++ {
		state := "active"
		if i == p.watches-3 && p.watches >= 6 {
			state = "paused"
		}
		if i >= p.watches-2 && p.watches >= 4 {
			state = "deprecated"
		}
		source, _ := json.Marshal(map[string]string{"kind": "fixture", "locator": fmt.Sprintf("perf/source/%d", i)})
		exec(watch, id("watch", i), string(source), "Inspect new source changes.", state, now-86400000, base, base, fmt.Sprintf("watcher-%d", i))
		exec(watchInterest, id("watch", i), id("interest", i%activeInterests), base)
	}
	item := prepare(tx, `INSERT INTO items(id,dedupe_key,kind,watch_id,parent_id,title,summary,content,content_version,content_hash,todo_state,remind_at,reminder_timezone,acknowledged_content_version,user_note,state_version,created_at,content_updated_at,state_updated_at,origin) VALUES(?,?,?,?,?,?,?,?,?,?,?,NULL,NULL,?,?,1,?,?,?,?)`)
	itemInterest := prepare(tx, `INSERT INTO item_interests(item_id,interest_id,reason) VALUES(?,?,?)`)
	version := prepare(tx, `INSERT INTO item_versions(item_id,content_version,snapshot) VALUES(?,?,?)`)
	event := prepare(tx, `INSERT INTO events(occurred_at,actor,entity_type,entity_id,change_type,payload) VALUES(?,'agent','item',?,'item.content_updated','{}')`)
	var versions int
	var sampleInput app.PutItem
	for i := 0; i < p.items; i++ {
		kind := []string{"note", "report", "task", "outcome"}[i%4]
		origin := "agent"
		var watchID any = id("watch", i%p.watches)
		watchValue := id("watch", i%p.watches)
		watchPointer := &watchValue
		if i%20 == 0 {
			origin = "user"
			watchID = nil
			watchPointer = nil
		}
		contentVersion := 1
		if name == "mature" && i%2 == 0 {
			contentVersion = 4
		}
		if name == "large" {
			contentVersion = 3
		}
		if name == "skewed" && i%5 == 0 {
			contentVersion = 21
		}
		if name == "fresh" && i%5 == 0 {
			contentVersion = 2
		}
		attention := i%100 < p.attentionPercent
		ack := contentVersion
		if attention {
			ack = contentVersion - 1
		}
		todo := "none"
		if attention && i%3 == 0 {
			todo = "todo"
		} else if i%17 == 0 {
			todo = "done"
		}
		sources := []app.Source{{ID: "source", URL: fmt.Sprintf("https://example.com/%d", i), Label: "Fixture evidence", ObservedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}}
		report := app.Report{SchemaVersion: 1, BodyMD: "## Update\n\nEvidence and next steps for this matter."}
		contextMD := "Continuing matter from the source."
		body, _ := json.Marshal(app.ItemContent{Sources: sources, ContextMD: contextMD, Report: report})
		stamp := base + int64(i%900)*86400000
		var parent any
		var parentPointer *string
		if i > 0 && i%13 == 0 {
			parent = id("item", i-1)
			parentValue := id("item", i-1)
			parentPointer = &parentValue
		}
		reasons := []app.ItemInterest{{ID: id("interest", i%activeInterests), Reason: "Relevant source change"}}
		if i%7 == 0 {
			reasons = append(reasons, app.ItemInterest{ID: id("interest", (i+1)%activeInterests), Reason: "Also relevant here"})
		}
		input := app.PutItem{DedupeKey: fmt.Sprintf("fixture:%d", i), Kind: kind, WatchID: watchPointer, ParentID: parentPointer, Title: fmt.Sprintf("Item %06d", i), Summary: fmt.Sprintf("Finding %d has a meaningful summary.", i), Sources: sources, Interests: reasons, ContextMD: contextMD, Report: report}
		if i == 0 {
			sampleInput = input
			sampleInput.ExpectedContentVersion = int64(contentVersion)
			sampleInput.RequestID = "fixture-hash-validation"
		}
		exec(item, id("item", i), input.DedupeKey, kind, watchID, parent, input.Title, input.Summary, string(body), contentVersion, fixtureHash(input), todo, ack, "", stamp, stamp, stamp, origin)
		exec(itemInterest, id("item", i), id("interest", i%activeInterests), "Relevant source change")
		if i%7 == 0 {
			exec(itemInterest, id("item", i), id("interest", (i+1)%activeInterests), "Also relevant here")
		}
		for v := 1; v <= contentVersion; v++ {
			summary := fmt.Sprintf("Revision %d of finding %d.", v, i)
			if v == contentVersion {
				summary = fmt.Sprintf("Finding %d has a meaningful summary.", i)
			}
			snapshot, _ := json.Marshal(map[string]any{"title": input.Title, "summary": summary, "kind": kind, "content": json.RawMessage(body), "interests": reasons})
			exec(version, id("item", i), v, string(snapshot))
			versions++
		}
		if i%4 == 0 {
			exec(event, stamp, id("item", i))
		}
	}
	run := prepare(tx, `INSERT INTO runs(id,runner_label,status,started_at,ended_at,lease_expires_at,selected_watches,after_seq,through_seq,summary,context_snapshot) VALUES(?,'fixture','completed',?,?,0,?,0,0,'Historical inspection','{}')`)
	result := prepare(tx, `INSERT INTO watch_results(run_id,watch_id,status,recorded_at,result) VALUES(?,?,'success',?,'{"coverage":{"cursor_before":null,"cursor_after":null,"limitations":[]},"items":[]}')`)
	runEvent := prepare(tx, `INSERT INTO events(occurred_at,actor,entity_type,entity_id,change_type,payload) VALUES(?,'agent','run',?,'run.finished','{}')`)
	receipt := prepare(tx, `INSERT INTO command_receipts(request_id,request_hash,response) VALUES(?,'fixture',?)`)
	for i := 0; i < p.runs; i++ {
		stamp := base + int64(i)*3600000
		watchID := id("watch", i%p.watches)
		selected, _ := json.Marshal([]map[string]any{{"id": watchID, "revision": 1, "source_generation": 1, "source": map[string]string{"kind": "fixture", "locator": fmt.Sprintf("perf/source/%d", i%p.watches)}, "interests": []map[string]any{{"id": id("interest", (i%p.watches)%activeInterests), "revision": 1}}}})
		exec(run, id("run", i), stamp, stamp+1000, string(selected))
		exec(result, id("run", i), watchID, stamp+1000)
		exec(runEvent, stamp+1000, id("run", i))
		exec(receipt, fmt.Sprintf("fixture-run-%d", i), fmt.Sprintf(`{"id":%q}`, id("run", i)))
	}
	must(tx.Commit())
	rows, err := s.DB.Query(`PRAGMA foreign_key_check`)
	must(err)
	if rows.Next() {
		panic("fixture has foreign-key violations")
	}
	must(rows.Err())
	must(rows.Close())
	for table, want := range map[string]int{"items": p.items, "item_versions": versions, "runs": p.runs, "watch_results": p.runs, "command_receipts": p.runs} {
		var got int
		must(s.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&got))
		if got != want {
			panic(fmt.Sprintf("%s count %d, want %d", table, got, want))
		}
	}
	a := app.New(s)
	if _, err = a.Item(ctx, id("item", p.items-1)); err != nil {
		panic(err)
	}
	if _, err = a.ItemHistory(ctx, id("item", 0)); err != nil {
		panic(err)
	}
	if _, err = a.RunDetail(ctx, id("run", p.runs-1)); err != nil {
		panic(err)
	}
	raw, err := a.PutItem(ctx, "", sampleInput)
	must(err)
	var unchanged app.Item
	must(json.Unmarshal(raw, &unchanged))
	if unchanged.ContentVersion != sampleInput.ExpectedContentVersion {
		panic("fixture content hash changed a no-op Item")
	}
	manifest, _ := json.MarshalIndent(map[string]any{"profile": name, "items": p.items, "versions": versions, "runs": p.runs, "interests": p.interests, "watches": p.watches, "attention_percent": p.attentionPercent, "fixture_version": 4}, "", "  ")
	must(os.WriteFile(filepath.Join(absolute, "perf-manifest.json"), append(manifest, '\n'), 0600))
	fmt.Printf("%s\n", manifest)
}
