// Package mcpserver exposes the agent-facing subset over MCP stdio.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/client"
)

type Caller struct{ Client *client.Client }
type empty struct{}
type runID struct {
	RunID     string `json:"run_id" jsonschema:"ID of the server-held run"`
	RequestID string `json:"request_id" jsonschema:"Stable idempotency key"`
}
type itemID struct {
	ItemID string `json:"item_id"`
}
type changesInput struct {
	AfterSeq   int64  `json:"after_seq"`
	ThroughSeq int64  `json:"through_seq"`
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}
type startRunInput struct {
	RequestID   string   `json:"request_id"`
	RunnerLabel string   `json:"runner_label"`
	WatchIDs    []string `json:"watch_ids,omitempty"`
	Force       bool     `json:"force,omitempty"`
}
type finishRunInput struct {
	RunID         string `json:"run_id"`
	RequestID     string `json:"request_id"`
	Summary       string `json:"summary"`
	AckThroughSeq *int64 `json:"ack_through_seq,omitempty"`
}
type publishInput struct {
	RunID                 string        `json:"run_id"`
	WatchID               string        `json:"watch_id"`
	RequestID             string        `json:"request_id"`
	ExpectedWatchRevision int64         `json:"expected_watch_revision"`
	Status                string        `json:"status"`
	Error                 string        `json:"error,omitempty"`
	Coverage              app.Coverage  `json:"coverage"`
	Items                 []app.PutItem `json:"items"`
}
type actionInput struct {
	ItemID               string     `json:"item_id"`
	RequestID            string     `json:"request_id"`
	ExpectedStateVersion int64      `json:"expected_state_version"`
	Action               app.Action `json:"action"`
}
type proposalInput struct {
	RequestID        string         `json:"request_id"`
	ProposalKey      string         `json:"proposal_key"`
	TargetType       string         `json:"target_type"`
	TargetID         *string        `json:"target_id,omitempty"`
	ExpectedRevision *int64         `json:"expected_revision,omitempty"`
	Operation        string         `json:"operation"`
	Payload          map[string]any `json:"payload,omitempty"`
	RationaleMD      string         `json:"rationale_md"`
}

func (c Caller) call(ctx context.Context, method, path string, body any) (map[string]any, error) {
	raw, err := c.Client.Do(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func respond(value map[string]any, err error) (*mcp.CallToolResult, map[string]any, error) {
	if err == nil {
		return nil, value, nil
	}
	var remote *client.Error
	if !errors.As(err, &remote) {
		return nil, nil, err
	}
	payload, marshalErr := json.Marshal(map[string]any{"error": map[string]any{"code": remote.Code, "message": remote.Message, "retryable": remote.Retryable, "details": remote.Details}})
	if marshalErr != nil {
		return nil, nil, marshalErr
	}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}}}, nil, nil
}

func New(serverURL, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "aicp", Version: version}, nil)
	caller := Caller{Client: client.New(serverURL)}
	mcp.AddTool(server, &mcp.Tool{Name: "get_brief", Description: "Read due Watches, bounded human changes, open work, reminders, and health from aicp."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodGet, "/brief", nil)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_changes", Description: "Read a captured, bounded range of durable human and configuration changes."}, func(ctx context.Context, _ *mcp.CallToolRequest, input changesInput) (*mcp.CallToolResult, map[string]any, error) {
		query := url.Values{"after_seq": {strconv.FormatInt(input.AfterSeq, 10)}, "through_seq": {strconv.FormatInt(input.ThroughSeq, 10)}}
		if input.Cursor != "" {
			query.Set("cursor", input.Cursor)
		}
		if input.Limit > 0 {
			query.Set("limit", strconv.Itoa(input.Limit))
		}
		value, err := caller.call(ctx, http.MethodGet, "/changes?"+query.Encode(), nil)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "start_run", Description: "Claim aicp's single 30-minute agent lease and receive its authoritative brief."}, func(ctx context.Context, _ *mcp.CallToolRequest, input startRunInput) (*mcp.CallToolResult, map[string]any, error) {
		body := map[string]any{"request_id": input.RequestID, "runner_label": input.RunnerLabel, "force": input.Force}
		if input.WatchIDs != nil {
			body["watch_ids"] = input.WatchIDs
		}
		value, err := caller.call(ctx, http.MethodPost, "/runs", body)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "renew_run", Description: "Extend a live aicp run lease by 30 minutes."}, func(ctx context.Context, _ *mcp.CallToolRequest, input runID) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodPost, "/runs/"+input.RunID+"/renew", map[string]any{"request_id": input.RequestID})
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "publish_watch_result", Description: "Atomically publish one final Watch result, item updates, and its successful checkpoint."}, func(ctx context.Context, _ *mcp.CallToolRequest, input publishInput) (*mcp.CallToolResult, map[string]any, error) {
		body := app.PublishWatchResult{RequestID: input.RequestID, ExpectedWatchRevision: input.ExpectedWatchRevision, Status: input.Status, Error: input.Error, Coverage: input.Coverage, Items: input.Items}
		value, err := caller.call(ctx, http.MethodPut, "/runs/"+input.RunID+"/watches/"+input.WatchID+"/result", body)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "finish_run", Description: "Close a run and optionally acknowledge exactly its consumed captured change range."}, func(ctx context.Context, _ *mcp.CallToolRequest, input finishRunInput) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodPost, "/runs/"+input.RunID+"/finish", app.FinishRun{RequestID: input.RequestID, Summary: input.Summary, AckThroughSeq: input.AckThroughSeq})
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_item", Description: "Read one complete aicp item with current content and independent local state."}, func(ctx context.Context, _ *mcp.CallToolRequest, input itemID) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodGet, "/items/"+input.ItemID, nil)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_context", Description: "Read one item with its bounded recent content history for reconciliation."}, func(ctx context.Context, _ *mcp.CallToolRequest, input itemID) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodGet, "/items/"+input.ItemID+"/context", nil)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "apply_item_action", Description: "Apply an explicit Todo, reminder, or acknowledgement action with state-version fencing."}, func(ctx context.Context, _ *mcp.CallToolRequest, input actionInput) (*mcp.CallToolResult, map[string]any, error) {
		body := app.ApplyItemAction{RequestID: input.RequestID, ExpectedStateVersion: input.ExpectedStateVersion, Action: input.Action}
		value, err := caller.call(ctx, http.MethodPost, "/items/"+input.ItemID+"/actions", body)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "propose_change", Description: "Propose a typed Interest or Watch configuration change for human review."}, func(ctx context.Context, _ *mcp.CallToolRequest, input proposalInput) (*mcp.CallToolResult, map[string]any, error) {
		var payload json.RawMessage
		if input.Payload != nil {
			payload, _ = json.Marshal(input.Payload)
		}
		body := app.CreateProposal{RequestID: input.RequestID, ProposalKey: input.ProposalKey, TargetType: input.TargetType, TargetID: input.TargetID, ExpectedRevision: input.ExpectedRevision, Operation: input.Operation, Payload: payload, RationaleMD: input.RationaleMD}
		value, err := caller.call(ctx, http.MethodPost, "/proposals", body)
		return respond(value, err)
	})
	return server
}
