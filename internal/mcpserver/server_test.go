package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
			"watches":        []any{},
			"more_due_count": 0,
			"changes":        map[string]any{"after_seq": 2, "through_seq": 4},
		})
	}))
	defer api.Close()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := New(api.URL, "test").Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "aicp-test", Version: "test"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	tools, err := clientSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"get_brief", "start_run", "submit_watch_findings", "upsert_item", "finish_run"} {
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
	if !ok || brief["more_due_count"] != float64(0) {
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
