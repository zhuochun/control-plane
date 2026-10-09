package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/httpapi"
	"github.com/zhuochun/control-plane/internal/store"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetBriefOverMCP(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/items/missing" {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "content_conflict", "message": "Read again", "retryable": true, "details": map[string]any{"current_content_version": 3}}})
			return
		}
		if r.URL.Path != "/api/v1/brief" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"consistency": "live",
			"due_watches": map[string]any{"count": 0, "items": []any{}},
		})
	}))
	defer api.Close()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := New(api.URL, "test").Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := serverSession.Close(); err != nil {
			t.Errorf("close MCP server session: %v", err)
		}
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "aicp-test", Version: "test"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := clientSession.Close(); err != nil {
			t.Errorf("close MCP client session: %v", err)
		}
	})
	tools, err := clientSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"get_brief", "start_run", "submit_watch_findings", "upsert_item", "finish_run", "list_watchers", "get_watcher", "create_watcher", "update_watcher", "list_interests", "create_interest", "update_interest", "list_items", "get_status", "get_settings"} {
		if !names[name] {
			t.Fatalf("missing final MCP tool %q", name)
		}
	}
	for _, old := range []string{"renew_run", "publish_watch_result"} {
		if names[old] {
			t.Fatalf("superseded MCP tool still exposed: %s", old)
		}
	}
	for _, tool := range tools.Tools {
		if tool.Name == "start_run" && strings.Contains(strings.ToLower(tool.Description), "lease") {
			t.Fatalf("start_run description still teaches lease management: %s", tool.Description)
		}
	}

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_brief", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned an error: %+v", result.Content)
	}
	brief, ok := result.StructuredContent.(map[string]any)
	if !ok || brief["consistency"] != "live" {
		t.Fatalf("unexpected structured brief: %#v", result.StructuredContent)
	}
	errorResult, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_item", Arguments: map[string]any{"item_id": "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	if !errorResult.IsError || len(errorResult.Content) != 1 {
		t.Fatalf("expected MCP tool error, got %+v", errorResult)
	}
	text, ok := errorResult.Content[0].(*mcp.TextContent)
	if !ok || text.Text == "" || !json.Valid([]byte(text.Text)) {
		t.Fatalf("error details were not preserved as JSON: %#v", errorResult.Content)
	}
}

func TestItemLookupAndCapturedAttentionThroughMCP(t *testing.T) {
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
	a := app.New(s)
	for _, key := range []string{"matter:A & B", "matter:other"} {
		if _, err = a.PutItem(ctx, "", app.PutItem{DedupeKey: key, Kind: "task", Title: "Existing matter", Summary: key, Sources: []app.Source{}, InitialTodoState: "todo", Report: app.Report{SchemaVersion: 1, BodyMD: "Keep user state"}}); err != nil {
			t.Fatal(err)
		}
	}
	api := httptest.NewServer(httpapi.New(a, http.NotFoundHandler(), "test"))
	defer api.Close()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := New(api.URL, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := serverSession.Close(); err != nil {
			t.Errorf("close MCP server session: %v", err)
		}
	})
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := clientSession.Close(); err != nil {
			t.Errorf("close MCP client session: %v", err)
		}
	})
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("%s: %+v %v", name, result, err)
		}
		return result.StructuredContent.(map[string]any)
	}
	exact := call("list_items", map[string]any{"dedupe_key": "matter:A & B", "limit": 1})
	items := exact["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["dedupe_key"] != "matter:A & B" {
		t.Fatalf("exact lookup failed: %#v", exact)
	}
	assignedID := items[0].(map[string]any)["id"]
	updated := call("update_item_work", map[string]any{"item_id": assignedID, "update": map[string]any{"expected_content_version": 1, "title": "Existing matter clarified", "summary": "Corrected summary", "reason": "Owner requested correction", "delegations": []any{map[string]any{"id": "research", "executor": "agent:A & B", "external_ref": "session:1?&", "instructions_md": "Read assigned Item v1", "status": "pending"}}}})
	if updated["content_version"] != float64(2) || updated["title"] != "Existing matter clarified" || updated["summary"] != "Corrected summary" {
		t.Fatalf("work update failed: %#v", updated)
	}
	call("apply_item_action", map[string]any{"item_id": assignedID, "expected_state_version": 1, "reason": "Owner asked to mark this version seen", "action": map[string]any{"type": "acknowledge", "content_version": 2}})
	var actor string
	if err = s.DB.QueryRowContext(ctx, "SELECT actor FROM events WHERE entity_id=? AND change_type='item.state_updated' ORDER BY seq DESC LIMIT 1", assignedID).Scan(&actor); err != nil || actor != "agent" {
		t.Fatalf("MCP action impersonated user: %s %v", actor, err)
	}
	legacy := app.ApplyItemAction{RequestID: "legacy-mcp-action", ExpectedStateVersion: 2, Action: app.Action{Type: "acknowledge", ContentVersion: 1}}
	legacyResponse, err := a.ApplyItemAction(ctx, assignedID.(string), legacy)
	if err != nil {
		t.Fatal(err)
	}
	replayed := call("apply_item_action", map[string]any{"item_id": assignedID, "request_id": legacy.RequestID, "expected_state_version": legacy.ExpectedStateVersion, "action": map[string]any{"type": "acknowledge", "content_version": 1}})
	var committed map[string]any
	if err = json.Unmarshal(legacyResponse, &committed); err != nil || replayed["state_version"] != committed["state_version"] {
		t.Fatalf("legacy MCP receipt did not replay: %#v %v", replayed, err)
	}
	delegated := call("list_items", map[string]any{"executor": "agent:A & B", "external_ref": "session:1?&", "delegation_status": "pending,blocked", "limit": 1})
	if len(delegated["items"].([]any)) != 1 || delegated["items"].([]any)[0].(map[string]any)["id"] != assignedID {
		t.Fatalf("MCP delegation filters lost: %#v", delegated)
	}
	page := call("list_items", map[string]any{"view": "todo", "query": "Existing matter", "limit": 1})
	next := call("list_items", map[string]any{"view": "todo", "query": "Existing matter", "limit": 1, "cursor": page["next_cursor"]})
	if page["items"].([]any)[0].(map[string]any)["id"] == next["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("lookup pagination repeated first item")
	}
	started := call("start_run", map[string]any{"request_id": "mcp-context", "watch_ids": []string{}})
	packet := started["context"].(map[string]any)
	if _, old := started["brief"]; old {
		t.Fatal("MCP still returned old contract")
	}
	if _, mandatory := packet["attention_items"]; mandatory {
		t.Fatal("MCP enumerated optional backlog")
	}
	available := packet["available"].(map[string]any)["attention"].(map[string]any)
	captured := call("get_brief", map[string]any{"cursor": available["cursor"]})
	if captured["consistency"] != "captured" || len(captured["items"].([]any)) != 2 {
		t.Fatalf("missing optional capture: %#v", captured)
	}
	brief := call("get_brief", map[string]any{})
	if brief["consistency"] != "live" || brief["active_run"] == nil {
		t.Fatalf("brief unavailable during active Run: %#v", brief)
	}
	settings := call("get_settings", map[string]any{})
	if settings["agents_md"] == nil {
		t.Fatal("context unavailable on demand")
	}
	status := call("get_status", map[string]any{})
	if status["health"] == nil {
		t.Fatal("full limitations unavailable on demand")
	}
	call("finish_run", map[string]any{"run_id": started["run"].(map[string]any)["id"], "summary": "Only requested source scope"})
	inputRaw, err := a.PutItem(ctx, "", app.PutItem{DedupeKey: "user:mcp-input", Kind: "note", Title: "MCP inbox", Summary: "Capture", Report: app.Report{SchemaVersion: 1, BodyMD: "Reference capture"}})
	if err != nil {
		t.Fatal(err)
	}
	var inboxItem app.Item
	if err = json.Unmarshal(inputRaw, &inboxItem); err != nil {
		t.Fatal(err)
	}
	current := call("get_item", map[string]any{"item_id": inboxItem.ID})
	inputRun := call("start_run", map[string]any{"watch_ids": []string{}})
	entry := current["pending_inputs"].([]any)[0].(map[string]any)
	handled := call("process_item_input", map[string]any{"item_id": inboxItem.ID, "input": map[string]any{"run_id": inputRun["run"].(map[string]any)["id"], "input_id": entry["id"], "expected_content_version": current["content_version"], "expected_state_version": current["state_version"], "outcome": "responded", "result_md": "Reference saved on the Item", "archive": true}})
	if handled["inbox_archived"] != true {
		t.Fatalf("MCP processing failed: %#v", handled)
	}
	history := call("get_item_inputs", map[string]any{"item_id": inboxItem.ID})
	if len(history["items"].([]any)) != 1 {
		t.Fatalf("MCP original history missing: %#v", history)
	}
	call("finish_run", map[string]any{"run_id": inputRun["run"].(map[string]any)["id"], "summary": "Handled capture"})
}
