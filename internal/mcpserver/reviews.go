package mcpserver

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zhuochun/control-plane/internal/app"
)

type answerHistoryInput struct {
	ItemID string `json:"item_id"`
	After  int64  `json:"after,omitempty"`
}

// The SDK rejects recursive Go types. Publish a finite schema for the same
// bounded report -> review -> option display nesting enforced by the app.
func reportInputSchema[T any]() *jsonschema.Schema {
	var blockSchema func(int) *jsonschema.Schema
	blockSchema = func(depth int) *jsonschema.Schema {
		zero := 0
		children := &jsonschema.Schema{Type: "array", MaxItems: &zero}
		if depth > 0 {
			children = &jsonschema.Schema{Type: "array", Items: blockSchema(depth - 1)}
		}
		schema, err := jsonschema.ForType(reflect.TypeFor[app.ReportBlock](), &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[[]app.ReportBlock](): children}})
		if err != nil {
			panic(err)
		}
		return schema
	}
	schema, err := jsonschema.ForType(reflect.TypeFor[T](), &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[app.ReportBlock](): blockSchema(2)}})
	if err != nil {
		panic(err)
	}
	return schema
}

func addReviewTools(server *mcp.Server, caller Caller) {
	mcp.AddTool(server, &mcp.Tool{Name: "get_review_capabilities", Description: "Discover report block types, renderer availability, and evidence limits before publishing a review."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodGet, "/review-capabilities", nil)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "list_review_formats", Description: "Read registered immutable review formats; use get_review_format for an exact version."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ empty) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodGet, "/review-formats", nil)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_review_format", Description: "Read one immutable review format. Publish a complete instance matching its ordered fields and constraints."}, func(ctx context.Context, _ *mcp.CallToolRequest, input app.FormatRef) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodGet, "/review-formats/"+url.PathEscape(input.ID)+"/"+strconv.Itoa(input.Version), nil)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "register_review_format", Description: "Register an immutable declarative review layout. Reuse identical versions; changes need a new version. No executable rendering code."}, func(ctx context.Context, _ *mcp.CallToolRequest, input app.RegisterReviewFormat) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodPost, "/review-formats", input)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "upload_review_artifact", Description: "Upload immutable base64 PNG/JPEG evidence, up to 2 MiB. Use the returned digest ID in image blocks."}, func(ctx context.Context, _ *mcp.CallToolRequest, input app.UploadReviewArtifact) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodPost, "/review-artifacts", input)
		return respond(value, err)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_answer_history", Description: "Read paged historical human answers. get_item returns current submissions and applicability directly. Historical answers are not current authorization."}, func(ctx context.Context, _ *mcp.CallToolRequest, input answerHistoryInput) (*mcp.CallToolResult, map[string]any, error) {
		value, err := caller.call(ctx, http.MethodGet, "/items/"+url.PathEscape(input.ItemID)+"/answers?after="+strconv.FormatInt(input.After, 10), nil)
		return respond(value, err)
	})
}
