// Package mcpserver exposes the agent-facing subset over MCP stdio.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/client"
)

type Caller struct{ Client *client.Client }
type empty struct{}
type briefInput struct {
	Cursor string `json:"cursor,omitempty" jsonschema:"Continuation cursor from an earlier brief page"`
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
	RequestID   string   `json:"request_id,omitempty"`
	RunnerLabel string   `json:"runner_label,omitempty"`
	WatchIDs    []string `json:"watch_ids,omitempty"`
	Force       bool     `json:"force,omitempty"`
}
type finishRunInput struct {
	RunID         string `json:"run_id,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
	Summary       string `json:"summary,omitempty"`
	AckThroughSeq *int64 `json:"ack_through_seq,omitempty"`
}
type findingsInput struct {
	RunID                 string        `json:"run_id,omitempty"`
	WatchID               string        `json:"watch_id"`
	RequestID             string        `json:"request_id,omitempty"`
	ExpectedWatchRevision int64         `json:"expected_watch_revision"`
	Status                string        `json:"status"`
	Error                 string        `json:"error,omitempty"`
	Coverage              app.Coverage  `json:"coverage"`
	Items                 []app.PutItem `json:"items"`
}
type actionInput struct {
	ItemID               string     `json:"item_id"`
	RequestID            string     `json:"request_id,omitempty"`
	ExpectedStateVersion int64      `json:"expected_state_version"`
	Action               app.Action `json:"action"`
}
type proposalInput struct {
	RequestID        string         `json:"request_id,omitempty"`
	ProposalKey      string         `json:"proposal_key"`
	TargetType       string         `json:"target_type"`
	TargetID         *string        `json:"target_id,omitempty"`
	ExpectedRevision *int64         `json:"expected_revision,omitempty"`
	Operation        string         `json:"operation"`
	Payload          map[string]any `json:"payload,omitempty"`
	RationaleMD      string         `json:"rationale_md"`
	EvidenceLinks    []string       `json:"evidence_links,omitempty"`
	Confidence       *float64       `json:"confidence,omitempty"`
	DuplicateOf      *string        `json:"duplicate_of,omitempty"`
	ExpiresAt        *time.Time     `json:"expires_at,omitempty"`
}
type configRef struct {
	Ref string `json:"ref"`
}
type configList struct {
	State string `json:"state,omitempty"`
}
type configCreate struct {
	Payload map[string]any `json:"payload"`
}
type configUpdate struct {
	Ref     string         `json:"ref"`
	Payload map[string]any `json:"payload"`
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
	for _, entity := range []struct{ name, path string }{{"interest", "/interests"}, {"watcher", "/watches"}} {
		entity := entity
		mcp.AddTool(server, &mcp.Tool{Name: "list_" + entity.name + "s", Description: "Read current " + entity.name + " configuration without starting a source Run."}, func(ctx context.Context, _ *mcp.CallToolRequest, input configList) (*mcp.CallToolResult, map[string]any, error) {
			query := url.Values{}
			if input.State != "" {
				query.Set("state", input.State)
			}
			value, err := caller.call(ctx, http.MethodGet, entity.path+"?"+query.Encode(), nil)
			return respond(value, err)
		})
		mcp.AddTool(server, &mcp.Tool{Name: "get_" + entity.name, Description: "Read one " + entity.name + " by slug or immutable ID, without starting a source Run."}, func(ctx context.Context, _ *mcp.CallToolRequest, input configRef) (*mcp.CallToolResult, map[string]any, error) {
			value, err := caller.call(ctx, http.MethodGet, entity.path+"/"+url.PathEscape(input.Ref), nil)
			return respond(value, err)
		})
		mcp.AddTool(server, &mcp.Tool{Name: "create_" + entity.name, Description: "Create a " + entity.name + " directly from a configuration payload; emits an auditable config event without claiming source work."}, func(ctx context.Context, _ *mcp.CallToolRequest, input configCreate) (*mcp.CallToolResult, map[string]any, error) {
			value, err := caller.call(ctx, http.MethodPost, entity.path, input.Payload)
			return respond(value, err)
		})
		mcp.AddTool(server, &mcp.Tool{Name: "update_" + entity.name, Description: "Update a " + entity.name + " directly. Payload must include expected_revision. Watcher interest_ids replace explicit links while retaining identity and cursor."}, func(ctx context.Context, _ *mcp.CallToolRequest, input configUpdate) (*mcp.CallToolResult, map[string]any, error) {
			value, err := caller.call(ctx, http.MethodPatch, entity.path+"/"+url.PathEscape(input.Ref), input.Payload)
			return respond(value, err)
		})
	}
	mcp.AddTool(server, &mcp.Tool{Name: "get_brief", Description: "Preview aicp's active Interests, due Watches, unarchived Attention summaries, captured changes, contexts, and health. Read-only; follow continuation cursors until the requested collection is complete."}, func(ctx context.Context, _ *mcp.CallToolRequest, input briefInput) (*mcp.CallToolResult, map[string]any, error) {
		path := "/brief"
		if input.Cursor != "" {
			path += "?cursor=" + url.QueryEscape(input.Cursor)
		}
		value, err := caller.call(ctx, http.MethodGet, path, nil)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "set_user_context", Description: "Save the person's approved USER.md draft directly without starting a Run. Include their priorities and follow-through; never put source credentials here. This changes future Run snapshots only."}, func(ctx context.Context, _ *mcp.CallToolRequest, input app.SetUserContext) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodPut, "/settings/user-context", input)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_changes", Description: "Read pages from the exact captured event range returned by start_run. Consume every page before relying on older assumptions."}, func(ctx context.Context, _ *mcp.CallToolRequest, input changesInput) (*mcp.CallToolResult, map[string]any, error) {
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
	mcp.AddTool(server, &mcp.Tool{Name: "start_run", Description: "Check in for one heartbeat, claim the single active run slot, and receive the authoritative bounded work packet. Active due Watches are selected by default; consume all Interest, Attention, and change continuations before finishing."}, func(ctx context.Context, _ *mcp.CallToolRequest, input startRunInput) (*mcp.CallToolResult, map[string]any, error) {
		body := map[string]any{"request_id": input.RequestID, "runner_label": input.RunnerLabel, "force": input.Force}
		if input.WatchIDs != nil {
			body["watch_ids"] = input.WatchIDs
		}
		value, err := caller.call(ctx, http.MethodPost, "/runs", body)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "submit_watch_findings", Description: "Submit one final success, partial, failed, or empty result for one selected Watch. Atomically save its Items, coverage, limitations, and successful checkpoint. Call exactly once per selected Watch."}, func(ctx context.Context, _ *mcp.CallToolRequest, input findingsInput) (*mcp.CallToolResult, map[string]any, error) {
		body := app.SubmitWatchFindings{RequestID: input.RequestID, ExpectedWatchRevision: input.ExpectedWatchRevision, Status: input.Status, Error: input.Error, Coverage: input.Coverage, Items: input.Items}
		path := "/runs/watches/" + input.WatchID + "/findings"
		if input.RunID != "" {
			path = "/runs/" + input.RunID + "/watches/" + input.WatchID + "/findings"
		}
		value, err := caller.call(ctx, http.MethodPut, path, body)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "upsert_item", Description: "Create or update an Interest-level Item that is not specific to a selected Watch. Use a stable dedupe key and expected content version; this never advances Watch coverage."}, func(ctx context.Context, _ *mcp.CallToolRequest, input app.PutItem) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodPost, "/items/interest", input)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "finish_run", Description: "Check out the active heartbeat run after every selected Watch has a terminal coverage record. Store a summary and optionally acknowledge the exact captured change range."}, func(ctx context.Context, _ *mcp.CallToolRequest, input finishRunInput) (*mcp.CallToolResult, map[string]any, error) {
		path := "/runs/finish"
		if input.RunID != "" {
			path = "/runs/" + input.RunID + "/finish"
		}
		value, err := caller.call(ctx, http.MethodPost, path, app.FinishRun{RequestID: input.RequestID, Summary: input.Summary, AckThroughSeq: input.AckThroughSeq})
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_item", Description: "Read one complete Item, including current content, source references, versions, and independent user state before updating it."}, func(ctx context.Context, _ *mcp.CallToolRequest, input itemID) (*mcp.CallToolResult, map[string]any, error) {
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
	mcp.AddTool(server, &mcp.Tool{Name: "propose_change", Description: "Suggest creation, revision, or deprecation of one Interest or Watch for human review. Never apply configuration changes implicitly."}, func(ctx context.Context, _ *mcp.CallToolRequest, input proposalInput) (*mcp.CallToolResult, map[string]any, error) {
		var payload json.RawMessage
		if input.Payload != nil {
			payload, _ = json.Marshal(input.Payload)
		}
		body := app.CreateProposal{RequestID: input.RequestID, ProposalKey: input.ProposalKey, TargetType: input.TargetType, TargetID: input.TargetID, ExpectedRevision: input.ExpectedRevision, Operation: input.Operation, Payload: payload, RationaleMD: input.RationaleMD, EvidenceLinks: input.EvidenceLinks, Confidence: input.Confidence, DuplicateOf: input.DuplicateOf, ExpiresAt: input.ExpiresAt}
		value, err := caller.call(ctx, http.MethodPost, "/proposals", body)
		return respond(value, err)
	})
	return server
}
