// Run from the repository root against a newly seeded, isolated perf directory.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/httpapi"
	"github.com/zhuochun/control-plane/internal/mcpserver"
	"github.com/zhuochun/control-plane/internal/store"
)

type tokenInput struct {
	Category string `json:"category"`
	Text     string `json:"text"`
}

type tokenResult struct {
	Tokenizer    string `json:"tokenizer"`
	Observations []struct {
		Category string `json:"category"`
		Bytes    int    `json:"bytes"`
		Tokens   int    `json:"tokens"`
	} `json:"observations"`
}

type callRecord struct {
	Tool            string `json:"tool"`
	Category        string `json:"category"`
	Milliseconds    int64  `json:"milliseconds"`
	ContentBytes    int    `json:"content_bytes"`
	StructuredBytes int    `json:"structured_bytes"`
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func asMap(value any) map[string]any {
	result, ok := value.(map[string]any)
	if !ok {
		log.Fatalf("expected object, got %T", value)
	}
	return result
}

func asList(value any) []any {
	result, ok := value.([]any)
	if !ok {
		log.Fatalf("expected list, got %T", value)
	}
	return result
}

func main() {
	var directory, output, mode string
	flag.StringVar(&directory, "data-dir", "", "newly seeded synthetic performance directory")
	flag.StringVar(&output, "output", "", "aggregate JSON output")
	flag.StringVar(&mode, "mode", "quiet", "quiet, backlog, partial-retry, multi-interest, or reconcile")
	flag.Parse()
	if directory == "" || output == "" || (mode != "quiet" && mode != "backlog" && mode != "partial-retry" && mode != "multi-interest" && mode != "reconcile") {
		log.Fatal("provide -data-dir, -output, and -mode quiet|backlog|partial-retry|multi-interest|reconcile")
	}
	directory, err := filepath.Abs(directory)
	must(err)
	manifestRaw, err := os.ReadFile(filepath.Join(directory, "perf-manifest.json"))
	must(err) // Never inspect the user's normal data directory.
	var manifest map[string]any
	must(json.Unmarshal(manifestRaw, &manifest))
	if manifest["fixture_version"] == nil {
		log.Fatal("missing performance fixture version")
	}
	ctx := context.Background()
	s, err := store.Open(ctx, directory)
	must(err)
	defer func() {
		if err := s.Close(); err != nil {
			log.Printf("close store: %v", err)
		}
	}()
	a := app.New(s)
	// A bounded but noticeable owner context makes repeated continuations visible.
	agents := "# Agent guidance\n" + string(bytes.Repeat([]byte("Keep evidence and coverage precise.\n"), 80))
	user := "# Owner context\n" + string(bytes.Repeat([]byte("Prioritize current source-backed matters.\n"), 80))
	_, err = a.SetSettings(ctx, app.SetSettings{RequestID: "agent-eval-settings", Timezone: "browser", AgentsMD: &agents, UserMD: &user})
	must(err)
	if mode == "multi-interest" {
		changed, updateErr := s.DB.ExecContext(ctx, "UPDATE watches SET matching_policy='broad' WHERE slug='watcher-0' AND state='active'")
		must(updateErr)
		rows, rowsErr := changed.RowsAffected()
		must(rowsErr)
		if rows != 1 {
			log.Fatal("fixture has no active watcher-0")
		}
	}
	var targetID, distinctID string
	var originalVersion int64
	var initialItemCount int
	if mode == "reconcile" {
		must(s.DB.QueryRowContext(ctx, "SELECT id FROM items WHERE dedupe_key='fixture:1'").Scan(&targetID))
		target, itemErr := a.Item(ctx, targetID)
		must(itemErr)
		originalVersion = target.ContentVersion
		if target.WatchID == nil {
			log.Fatal("fixture:1 has no originating Watcher")
		}
		distinct := app.PutItem{RequestID: "agent-eval-distinct", DedupeKey: "eval:distinct", Kind: "task", Title: "Separate hiring commitment", Summary: "A different matter in the same source thread.", Interests: target.Interests, Sources: target.Sources, Report: app.Report{SchemaVersion: 1, BodyMD: "No new hiring evidence."}, InitialTodoState: "todo"}
		raw, createErr := a.PutItem(ctx, "", distinct)
		must(createErr)
		var distinctItem app.Item
		must(json.Unmarshal(raw, &distinctItem))
		distinctID = distinctItem.ID
		_, itemErr = a.ApplyItemAction(ctx, targetID, app.ApplyItemAction{RequestID: "agent-eval-todo", ExpectedStateVersion: target.StateVersion, Action: app.Action{Type: "set_todo", State: "todo"}})
		must(itemErr)
		_, itemErr = a.SetUserNote(ctx, targetID, app.SetUserNote{RequestID: "agent-eval-note", ExpectedStateVersion: target.StateVersion + 1, UserNote: "Keep this local follow-up."})
		must(itemErr)
		must(s.DB.QueryRowContext(ctx, "SELECT count(*) FROM items").Scan(&initialItemCount))
	}
	if mode != "backlog" {
		_, err = s.DB.ExecContext(ctx, "UPDATE settings SET value=(SELECT COALESCE(MAX(seq),0) FROM events) WHERE key='acknowledged_event_seq'")
		must(err)
	} else {
		tx, txErr := s.DB.BeginTx(ctx, nil)
		must(txErr)
		for index := 0; index < 250; index++ {
			entityType, changeType := "item", "item.state_updated"
			if index == 0 {
				entityType, changeType = "interest", "interest.updated"
			}
			_, txErr = tx.ExecContext(ctx, "INSERT INTO events(occurred_at,actor,entity_type,entity_id,change_type,payload) VALUES (?, 'user', ?, ?, ?, ?)", time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC).UnixMilli()+int64(index), entityType, fmt.Sprintf("fixture-%d", index), changeType, `{"synthetic":true}`)
			must(txErr)
		}
		must(tx.Commit())
	}
	api := httptest.NewServer(httpapi.New(a, http.NotFoundHandler(), "agent-eval"))
	defer api.Close()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpserver.New(api.URL, "agent-eval").Connect(ctx, serverTransport, nil)
	must(err)
	defer func() {
		if err := serverSession.Close(); err != nil {
			log.Printf("close MCP server session: %v", err)
		}
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "aicp-perf", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	must(err)
	defer func() {
		if err := clientSession.Close(); err != nil {
			log.Printf("close MCP client session: %v", err)
		}
	}()
	tools, err := clientSession.ListTools(ctx, nil)
	must(err)
	toolJSON, err := json.Marshal(tools.Tools)
	must(err)
	inputs := []tokenInput{{Category: "tool_definitions", Text: string(toolJSON)}}
	records := []callRecord{}
	call := func(tool, category string, arguments map[string]any) map[string]any {
		start := time.Now()
		result, callErr := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: arguments})
		must(callErr)
		if result.IsError {
			log.Fatalf("%s returned an error: %+v", tool, result.Content)
		}
		content, marshalErr := json.Marshal(result.Content)
		must(marshalErr)
		structured, marshalErr := json.Marshal(result.StructuredContent)
		must(marshalErr)
		records = append(records, callRecord{Tool: tool, Category: category, Milliseconds: time.Since(start).Milliseconds(), ContentBytes: len(content), StructuredBytes: len(structured)})
		if len(result.Content) > 0 {
			inputs = append(inputs, tokenInput{Category: category + ":content", Text: string(content)})
		}
		if result.StructuredContent != nil {
			inputs = append(inputs, tokenInput{Category: category + ":structured", Text: string(structured)})
		}
		return asMap(result.StructuredContent)
	}
	started := call("start_run", "initial_packet", map[string]any{"request_id": "agent-eval-start", "runner_label": "agent-eval"})
	run := asMap(started["run"])
	brief := asMap(started["context"])
	if mode == "multi-interest" {
		foundBroad := false
		for _, raw := range asList(brief["watches"]) {
			watch := asMap(raw)
			if watch["matching_policy"] == "broad" {
				foundBroad = len(asList(watch["interests"])) >= 2
				if !foundBroad {
					log.Fatal("broad Watcher omitted applicable Interests")
				}
			}
		}
		if !foundBroad {
			log.Fatal("no broad Watcher was selected")
		}
	}
	counts := map[string]int{"interests": len(asList(brief["interests"])), "attention_items": 0, "changes": len(asList(brief["changes"]))}
	duplicateContextPages := 0
	continuations := asMap(brief["continuations"])
	for _, collection := range []string{"interests"} {
		cursor, _ := continuations[collection].(string)
		for cursor != "" {
			page := call("get_brief", collection+"_continuation", map[string]any{"cursor": cursor})
			if page["collection"] != collection {
				log.Fatalf("wrong continuation collection: %v", page["collection"])
			}
			counts[collection] += len(asList(page["items"]))
			if contexts, present := page["contexts"]; present {
				duplicateContextPages++
				contextJSON, marshalErr := json.Marshal(contexts)
				must(marshalErr)
				inputs = append(inputs, tokenInput{Category: "duplicate_context_probe", Text: string(contextJSON)})
			}
			cursor, _ = page["next_cursor"].(string)
		}
	}
	after := int64(run["after_seq"].(float64))
	through := int64(run["through_seq"].(float64))
	cursor, _ := continuations["changes"].(string)
	for cursor != "" {
		page := call("get_changes", "changes_continuation", map[string]any{"after_seq": after, "through_seq": through, "cursor": cursor})
		counts["changes"] += len(asList(page["items"]))
		cursor, _ = page["next_cursor"].(string)
	}
	var snapshotRaw string
	must(s.DB.QueryRowContext(ctx, "SELECT context_snapshot FROM runs WHERE id=?", run["id"]).Scan(&snapshotRaw))
	var snapshot map[string]any
	must(json.Unmarshal([]byte(snapshotRaw), &snapshot))
	required := map[string]bool{}
	for _, raw := range asList(brief["watches"]) {
		for _, match := range asList(asMap(raw)["interests"]) {
			required[asMap(match)["id"].(string)] = true
		}
	}
	if counts["interests"] != len(required) {
		log.Fatalf("required Interest pages incomplete: %d of %d", counts["interests"], len(required))
	}
	if _, mandatory := brief["attention_items"]; mandatory {
		log.Fatal("default Run enumerated Attention")
	}
	if asMap(brief["overview"])["attention_count"] != float64(len(asList(snapshot["attention_items"]))) {
		log.Fatal("optional Attention count omitted retained backlog")
	}
	var expectedChanges int
	must(s.DB.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE seq>? AND seq<=? AND (actor='user' OR entity_type='proposal')", after, through).Scan(&expectedChanges))
	if counts["changes"] != expectedChanges {
		log.Fatalf("change pages incomplete: %d of %d", counts["changes"], expectedChanges)
	}
	var updatedItem map[string]any
	if mode == "reconcile" {
		corpus, readErr := os.ReadFile("scripts/perf/corpus/one-matter-source.json")
		must(readErr)
		inputs = append(inputs, tokenInput{Category: "external_source_material", Text: string(corpus)})
		lookup := call("list_items", "item_lookup", map[string]any{"dedupe_key": "fixture:1", "limit": 1})
		matches := asList(lookup["items"])
		if len(matches) != 1 {
			log.Fatal("existing matter was not discoverable without default Attention")
		}
		updatedItem = call("get_item", "item_detail", map[string]any{"item_id": asMap(matches[0])["id"]})
		if updatedItem["id"] != targetID || updatedItem["content_version"] != float64(originalVersion) {
			log.Fatal("detail read returned the wrong existing Item")
		}
	}
	var partialWatchID string
	var partialCursor any
	for index, raw := range asList(brief["watches"]) {
		watch := asMap(raw)
		coverage := map[string]any{"cursor_before": watch["cursor"], "cursor_after": watch["cursor"], "observed_through": "2026-09-28T00:00:00Z", "limitations": []any{}}
		arguments := map[string]any{"run_id": run["id"], "watch_id": watch["id"], "request_id": fmt.Sprintf("agent-eval-watch-%d", index), "expected_watch_revision": watch["revision"], "status": "success", "coverage": coverage, "items": []any{}}
		if mode == "reconcile" && watch["id"] == updatedItem["watch_id"] {
			entry := map[string]any{"dedupe_key": updatedItem["dedupe_key"], "expected_content_version": updatedItem["content_version"], "kind": updatedItem["kind"], "title": updatedItem["title"], "summary": "Release approval is delayed until the security review finishes.", "interests": updatedItem["interests"], "sources": updatedItem["sources"], "context_md": updatedItem["context_md"], "report": map[string]any{"schema_version": 1, "body_md": "New source evidence: approval is delayed until the security review finishes."}}
			arguments["items"] = []any{entry}
		}
		if mode == "partial-retry" && index == 0 {
			partialWatchID, partialCursor = watch["id"].(string), watch["cursor"]
			arguments["status"] = "partial"
			arguments["error"] = "Synthetic source ended before the captured range was fully read."
			coverage["cursor_after"] = nil
			coverage["limitations"] = []any{"Last page unavailable"}
			first := call("submit_watch_findings", "mutation_receipt", arguments)
			replayed := call("submit_watch_findings", "retry_receipt", arguments)
			if !reflect.DeepEqual(first, replayed) {
				log.Fatal("request-ID replay returned a different result")
			}
			continue
		}
		call("submit_watch_findings", "mutation_receipt", arguments)
	}
	finishArguments := map[string]any{"run_id": run["id"], "request_id": "agent-eval-finish", "summary": "Synthetic source scan after full packet consumption."}
	if mode != "partial-retry" {
		finishArguments["ack_through_seq"] = through
	}
	finished := call("finish_run", "mutation_receipt", finishArguments)
	wantStatus := "completed"
	if mode == "partial-retry" {
		wantStatus = "partial"
	}
	if finished["status"] != wantStatus {
		log.Fatalf("run status %v", finished["status"])
	}
	if partialWatchID != "" {
		watch, watchErr := a.Watch(ctx, partialWatchID)
		must(watchErr)
		var cursorValue any
		if len(watch.Cursor) > 0 {
			must(json.Unmarshal(watch.Cursor, &cursorValue))
		}
		if !reflect.DeepEqual(cursorValue, partialCursor) {
			log.Fatal("partial coverage advanced the Watcher cursor")
		}
		var resultCount int
		must(s.DB.QueryRowContext(ctx, "SELECT count(*) FROM watch_results WHERE run_id=? AND watch_id=?", run["id"], partialWatchID).Scan(&resultCount))
		if resultCount != 1 {
			log.Fatalf("request-ID replay created %d Watcher results", resultCount)
		}
	}
	if mode == "reconcile" {
		target, itemErr := a.Item(ctx, targetID)
		must(itemErr)
		distinct, itemErr := a.Item(ctx, distinctID)
		must(itemErr)
		answerRaw, readErr := os.ReadFile("scripts/perf/corpus/one-matter-answer.json")
		must(readErr)
		var answer struct {
			UpdatedKey      string `json:"updated_dedupe_key"`
			UnchangedKey    string `json:"unchanged_dedupe_key"`
			NewItems        int    `json:"new_items"`
			SourceURL       string `json:"required_source_url"`
			SummaryFragment string `json:"required_summary_fragment"`
			TodoState       string `json:"preserved_todo_state"`
			UserNote        string `json:"preserved_user_note"`
		}
		must(json.Unmarshal(answerRaw, &answer))
		var finalItemCount int
		must(s.DB.QueryRowContext(ctx, "SELECT count(*) FROM items").Scan(&finalItemCount))
		hasSource := false
		for _, source := range target.Sources {
			hasSource = hasSource || source.URL == answer.SourceURL
		}
		if target.ContentVersion != originalVersion+1 || target.DedupeKey != answer.UpdatedKey || !strings.Contains(target.Summary, answer.SummaryFragment) || !hasSource || target.TodoState != answer.TodoState || target.UserNote != answer.UserNote || distinct.ContentVersion != 1 || distinct.DedupeKey != answer.UnchangedKey || finalItemCount != initialItemCount+answer.NewItems {
			log.Fatalf("reconciliation changed wrong Item or user state: target=%+v distinct=%+v", target, distinct)
		}
	}
	var acknowledged int64
	must(s.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='acknowledged_event_seq'").Scan(&acknowledged))
	if mode != "partial-retry" && acknowledged != through {
		log.Fatalf("acknowledged %d, want %d", acknowledged, through)
	}
	inputJSON, err := json.Marshal(inputs)
	must(err)
	node, err := exec.LookPath("node")
	must(err)
	tokenizer := exec.Command(node, "web/perf/count-tokens.mjs")
	tokenizer.Stdin = bytes.NewReader(inputJSON)
	var tokenOutput bytes.Buffer
	tokenizer.Stdout = &tokenOutput
	tokenizer.Stderr = os.Stderr
	must(tokenizer.Run())
	var tokens tokenResult
	must(json.Unmarshal(tokenOutput.Bytes(), &tokens))
	result := map[string]any{"fixture": manifest, "mode": mode, "tokenizer": tokens.Tokenizer, "measurement": "reference tokenizer over deterministic MCP responses", "token_observations": tokens.Observations, "calls": records, "counts": counts, "duplicate_context_pages": duplicateContextPages, "selected_watchers": len(asList(brief["watches"])), "correctness": "deterministic protocol assertions passed", "useful_findings": map[bool]int{true: 1, false: 0}[mode == "reconcile"]}
	encoded, err := json.MarshalIndent(result, "", "  ")
	must(err)
	must(os.WriteFile(output, append(encoded, '\n'), 0600))
	fmt.Printf("%s: %d interests, %d attention summaries, %d changes, %d MCP calls\n", mode, counts["interests"], counts["attention_items"], counts["changes"], len(records))
	fmt.Printf("JSON: %s\n", output)
}
