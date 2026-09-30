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
	"time"

	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/httpapi"
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
	Command      string `json:"command"`
	Category     string `json:"category"`
	Milliseconds int64  `json:"milliseconds"`
	StdoutBytes  int    `json:"stdout_bytes"`
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
	var directory, output, binary, format, mode string
	flag.StringVar(&directory, "data-dir", "", "newly seeded synthetic performance directory")
	flag.StringVar(&output, "output", "", "aggregate JSON output")
	flag.StringVar(&binary, "binary", "", "built aicp executable")
	flag.StringVar(&format, "format", "json", "json or default")
	flag.StringVar(&mode, "mode", "backlog", "quiet or backlog")
	flag.Parse()
	if directory == "" || output == "" || binary == "" || (format != "json" && format != "default") || (mode != "quiet" && mode != "backlog") {
		log.Fatal("provide -data-dir, -output, -binary, -format json|default, and -mode quiet|backlog")
	}
	var err error
	directory, err = filepath.Abs(directory)
	must(err)
	binary, err = filepath.Abs(binary)
	must(err)
	if info, statErr := os.Stat(binary); statErr != nil || info.IsDir() {
		log.Fatalf("binary is not an executable file: %s", binary)
	}
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
	defer s.Close()
	a := app.New(s)
	agents := "# Agent guidance\n" + string(bytes.Repeat([]byte("Keep evidence and coverage precise.\n"), 80))
	user := "# Owner context\n" + string(bytes.Repeat([]byte("Prioritize current source-backed matters.\n"), 80))
	_, err = a.SetSettings(ctx, app.SetSettings{RequestID: "cli-eval-settings", Timezone: "browser", AgentsMD: &agents, UserMD: &user})
	must(err)
	var afterSeq int64
	must(s.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM events").Scan(&afterSeq))
	_, err = s.DB.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='acknowledged_event_seq'", afterSeq)
	must(err)
	if mode == "backlog" {
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
	var throughSeq int64
	must(s.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM events").Scan(&throughSeq))
	var itemID string
	must(s.DB.QueryRowContext(ctx, "SELECT id FROM items WHERE dedupe_key='fixture:1'").Scan(&itemID))
	api := httptest.NewServer(httpapi.New(a, http.NotFoundHandler(), "cli-eval"))
	defer api.Close()
	inputs := []tokenInput{}
	records := []callRecord{}
	call := func(category string, arguments ...string) map[string]any {
		args := []string{"--server", api.URL}
		if format == "json" {
			args = append(args, "--json")
		}
		args = append(args, arguments...)
		command := exec.CommandContext(ctx, binary, args...)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		start := time.Now()
		if runErr := command.Run(); runErr != nil {
			log.Fatalf("aicp %v failed: %v: %s", arguments, runErr, stderr.String())
		}
		if !json.Valid(stdout.Bytes()) {
			log.Fatalf("aicp %v did not return JSON: %q", arguments, stdout.String())
		}
		categoryName := category + ":stdout"
		records = append(records, callRecord{Command: arguments[0], Category: categoryName, Milliseconds: time.Since(start).Milliseconds(), StdoutBytes: stdout.Len()})
		inputs = append(inputs, tokenInput{Category: categoryName, Text: stdout.String()})
		var result map[string]any
		must(json.Unmarshal(stdout.Bytes(), &result))
		return result
	}
	call("version", "version")
	call("status", "doctor")
	call("settings", "config", "get")
	call("interests_page", "interest", "list", "--limit", "100")
	call("interest_detail", "interest", "get", "interest-0")
	call("watchers_page", "watch", "list", "--all", "--limit", "100")
	call("watcher_detail", "watch", "get", "watcher-0")
	call("runs_page", "run", "list", "--limit", "50")
	call("proposals_page", "proposal", "list", "--limit", "50")
	call("item_detail", "item", "get", itemID)
	attentionCount := 0
	cursor := ""
	for {
		args := []string{"item", "list", "--view", "attention", "--limit", "100"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		page := call("attention_page", args...)
		attentionCount += len(asList(page["items"]))
		cursor, _ = page["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	call("brief_preview", "brief")
	started := call("run_start", "run", "start", "--runner-label", "cli-eval")
	brief := asMap(started["context"])
	var runID string
	must(s.DB.QueryRowContext(ctx, "SELECT id FROM runs WHERE runner_label='cli-eval' ORDER BY started_at DESC,id DESC LIMIT 1").Scan(&runID))
	run, err := a.Run(ctx, runID)
	must(err)
	if len(run.SelectedWatches) != len(asList(brief["watches"])) || len(run.SelectedWatches) == 0 {
		log.Fatal("CLI run packet omitted selected Watchers")
	}
	if run.AfterSeq != afterSeq || run.ThroughSeq != throughSeq {
		log.Fatalf("Run captured unexpected change range: (%d,%d]", run.AfterSeq, run.ThroughSeq)
	}
	packetCounts := map[string]int{"interests": len(asList(brief["interests"])), "attention_items": 0}
	continuations := asMap(brief["continuations"])
	for _, collection := range []string{"interests"} {
		cursor, _ = continuations[collection].(string)
		for cursor != "" {
			page := call(collection+"_continuation", "brief", "--cursor", cursor)
			if page["collection"] != collection {
				log.Fatalf("wrong continuation collection: %v", page["collection"])
			}
			packetCounts[collection] += len(asList(page["items"]))
			cursor, _ = page["next_cursor"].(string)
		}
	}
	var snapshotRaw string
	must(s.DB.QueryRowContext(ctx, "SELECT context_snapshot FROM runs WHERE id=?", runID).Scan(&snapshotRaw))
	var snapshot map[string]any
	must(json.Unmarshal([]byte(snapshotRaw), &snapshot))
	required := map[string]bool{}
	for _, watch := range run.SelectedWatches {
		for _, match := range watch.Interests {
			required[match.ID] = true
		}
	}
	if packetCounts["interests"] != len(required) {
		log.Fatalf("CLI required Interests incomplete: %d of %d", packetCounts["interests"], len(required))
	}
	if _, mandatory := brief["attention_items"]; mandatory {
		log.Fatal("CLI default Run enumerated Attention")
	}
	if asMap(brief["overview"])["attention_count"] != float64(len(asList(snapshot["attention_items"]))) {
		log.Fatal("CLI optional Attention count mismatch")
	}
	changeCount := len(asList(brief["changes"]))
	cursor, _ = continuations["changes"].(string)
	for cursor != "" {
		page := call("changes_continuation", "changes", "--after-seq", fmt.Sprint(run.AfterSeq), "--through-seq", fmt.Sprint(run.ThroughSeq), "--limit", "100", "--cursor", cursor)
		changeCount += len(asList(page["items"]))
		cursor, _ = page["next_cursor"].(string)
	}
	wantChanges := 0
	if mode == "backlog" {
		wantChanges = 250
	}
	if changeCount != wantChanges {
		log.Fatalf("read %d captured changes, expected %d", changeCount, wantChanges)
	}
	for index, watch := range run.SelectedWatches {
		body := map[string]any{"request_id": fmt.Sprintf("cli-eval-watch-%d", index), "expected_watch_revision": watch.Revision, "status": "success", "coverage": map[string]any{"cursor_before": nil, "cursor_after": nil, "observed_through": "2026-09-28T00:00:00Z", "limitations": []any{}}, "items": []any{}}
		encoded, marshalErr := json.Marshal(body)
		must(marshalErr)
		requestFile := filepath.Join(directory, fmt.Sprintf("cli-eval-watch-%d.json", index))
		must(os.WriteFile(requestFile, encoded, 0600))
		call("run_submit", "run", "submit", watch.ID, "--file", requestFile)
	}
	finished := call("run_finish", "run", "finish", "--summary", "Synthetic CLI source scan after reading the packet.", "--ack-through-seq", fmt.Sprint(run.ThroughSeq))
	if finished["status"] != "completed" {
		log.Fatalf("run status %v", finished["status"])
	}
	call("run_detail", "run", "get", runID)
	var resultCount int
	must(s.DB.QueryRowContext(ctx, "SELECT count(*) FROM watch_results WHERE run_id=?", runID).Scan(&resultCount))
	if resultCount != len(run.SelectedWatches) {
		log.Fatalf("recorded %d Watcher results, expected %d", resultCount, len(run.SelectedWatches))
	}
	var acknowledged int64
	must(s.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='acknowledged_event_seq'").Scan(&acknowledged))
	if acknowledged != run.ThroughSeq {
		log.Fatalf("acknowledged %d, expected %d", acknowledged, run.ThroughSeq)
	}
	inputJSON, err := json.Marshal(inputs)
	must(err)
	node, err := exec.LookPath("node")
	must(err)
	tokenizer := exec.Command(node, "web/perf/count-tokens.mjs")
	tokenizer.Stdin = bytes.NewReader(inputJSON)
	var tokenOutput bytes.Buffer
	tokenizer.Stdout, tokenizer.Stderr = &tokenOutput, os.Stderr
	must(tokenizer.Run())
	var tokens tokenResult
	must(json.Unmarshal(tokenOutput.Bytes(), &tokens))
	result := map[string]any{"fixture": manifest, "format": format, "mode": mode, "tokenizer": tokens.Tokenizer, "measurement": "reference tokenizer over exact CLI stdout", "token_observations": tokens.Observations, "calls": records, "counts": map[string]int{"attention_items": attentionCount, "packet_attention_items": packetCounts["attention_items"], "packet_interests": packetCounts["interests"], "changes": changeCount, "selected_watchers": len(run.SelectedWatches)}, "correctness": "deterministic CLI packet and state assertions passed"}
	encoded, err := json.MarshalIndent(result, "", "  ")
	must(err)
	must(os.WriteFile(output, append(encoded, '\n'), 0600))
	fmt.Printf("%s/%s: %d Attention Items, %d changes, %d CLI calls; JSON: %s\n", format, mode, attentionCount, changeCount, len(records), output)
}
