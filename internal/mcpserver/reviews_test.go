package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/httpapi"
	"github.com/zhuochun/control-plane/internal/store"
)

func TestReviewFormatsPublicationAndAnswersThroughMCP(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	a := app.New(s)
	raw, err := a.PutItem(ctx, "", app.PutItem{DedupeKey: "mcp-review", Kind: "report", Title: "Review", Summary: "Review", Report: app.Report{SchemaVersion: 1, BodyMD: "Original"}})
	if err != nil {
		t.Fatal(err)
	}
	var item app.Item
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(httpapi.New(a, http.NotFoundHandler(), "test"))
	defer api.Close()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server, err := New(api.URL, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Error(err)
		}
	})
	client, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	call := func(name string, arguments any) map[string]any {
		t.Helper()
		result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("%s: %+v %v", name, result, err)
		}
		value, ok := result.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("%s has no structured result", name)
		}
		return value
	}
	call("get_review_capabilities", map[string]any{})
	format := app.ReviewFormat{ID: "mcp-format", Version: 1, Title: "Input", Fields: []app.FormatField{{ID: "text", Type: "text_input", Required: true}}}
	call("register_review_format", app.RegisterReviewFormat{Format: format})
	if len(call("list_review_formats", map[string]any{})["items"].([]any)) != 1 {
		t.Fatal("format list failed")
	}
	report := app.Report{SchemaVersion: 2, BodyMD: "Provide instructions", Blocks: []app.ReportBlock{{ID: "review", Type: "review", Title: "Instructions", Format: &app.FormatRef{ID: format.ID, Version: 1}, Blocks: []app.ReportBlock{{ID: "text", Type: "text_input", Question: "Instructions?", Required: true}}}}}
	call("update_item_work", workInput{ItemID: item.ID, Update: app.UpdateItemWork{ExpectedContentVersion: 1, Report: &report}})
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SubmitReviewAnswer(ctx, item.ID, app.SubmitReviewAnswer{RequestID: "human-answer", ReviewID: "review", ExpectedContentVersion: item.ContentVersion, ExpectedStateVersion: item.StateVersion, ReviewMaterialHash: item.ReviewMaterialHash, Values: []app.AnswerValue{{FieldID: "text", Disposition: "answered", Text: "Keep local"}}}); err != nil {
		t.Fatal(err)
	}
	answers := call("get_item", itemID{ItemID: item.ID})["answers"].([]any)
	if len(answers) != 1 || answers[0].(map[string]any)["applicable"] != true {
		t.Fatal("next agent could not read answer")
	}
	pending := call("get_brief", map[string]any{})["pending_changes"].(map[string]any)
	changes := call("get_changes", changesInput{AfterSeq: int64(pending["after_seq"].(float64)), ThroughSeq: int64(pending["through_seq"].(float64))})["items"].([]any)
	found := false
	for _, value := range changes {
		event := value.(map[string]any)
		if event["change_type"] == "item.answer_submitted" && event["entity_id"] == item.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("next agent could not discover the answer through the brief change range")
	}
	tools, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "submit_review_answer" {
			t.Fatal("agent can impersonate a human through MCP")
		}
	}
}
