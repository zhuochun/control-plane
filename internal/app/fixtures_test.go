package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/zhuochun/control-plane/internal/store"
)

func readFixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSharedReportFixturesMatchBackendValidation(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := New(s)
	compiler := jsonschema.NewCompiler()
	for _, name := range []string{"report-v1.schema.json", "watch-result-v1.schema.json"} {
		var document any
		if err = json.Unmarshal(readFixture(t, "..", "..", "schemas", name), &document); err != nil {
			t.Fatal(err)
		}
		if err = compiler.AddResource("https://aicp.local/schemas/"+name, document); err != nil {
			t.Fatal(err)
		}
	}
	reportSchema, err := compiler.Compile("https://aicp.local/schemas/report-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	resultSchema, err := compiler.Compile("https://aicp.local/schemas/watch-result-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var validDocument map[string]any
	if err = json.Unmarshal(readFixture(t, "..", "..", "fixtures", "valid", "report-item.json"), &validDocument); err != nil {
		t.Fatal(err)
	}
	if err = reportSchema.Validate(validDocument["report"]); err != nil {
		t.Fatalf("valid report fixture does not match its schema: %v", err)
	}
	delete(validDocument, "request_id")
	watchResult := map[string]any{"request_id": "fixture-result", "expected_watch_revision": 1, "status": "success", "coverage": map[string]any{"cursor_before": nil, "cursor_after": map[string]any{"position": 1}, "observed_through": "2026-09-13T02:04:00Z", "limitations": []any{}}, "items": []any{validDocument}}
	if err = resultSchema.Validate(watchResult); err != nil {
		t.Fatalf("valid Watch result does not match its schema: %v", err)
	}
	var valid PutItem
	if err = json.Unmarshal(readFixture(t, "..", "..", "fixtures", "valid", "report-item.json"), &valid); err != nil {
		t.Fatal(err)
	}
	if _, err = a.PutItem(ctx, "", valid); err != nil {
		t.Fatalf("valid shared fixture was rejected: %v", err)
	}
	var invalid PutItem
	invalidData := readFixture(t, "..", "..", "fixtures", "invalid", "report-unknown-action.json")
	if err = json.Unmarshal(invalidData, &invalid); err != nil {
		t.Fatal(err)
	}
	var invalidDocument map[string]any
	_ = json.Unmarshal(invalidData, &invalidDocument)
	if err = reportSchema.Validate(invalidDocument["report"]); err == nil {
		t.Fatal("invalid report fixture matched the schema")
	}
	if _, err = a.PutItem(ctx, "", invalid); err == nil {
		t.Fatal("invalid shared fixture was accepted")
	}
}
