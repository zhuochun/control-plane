package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestItemWorkCommandAndDelegationFilters(t *testing.T) {
	var path, method, body string
	var executor, reference, status string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		executor, reference, status = r.URL.Query().Get("executor"), r.URL.Query().Get("external_ref"), r.URL.Query().Get("delegation_status")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "work.json")
	if err := os.WriteFile(file, []byte(`{"expected_content_version":2,"title":"Corrected title","summary":"Corrected summary","reason":"Owner request","context_md":"Result based on v1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := command()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"--server", server.URL, "--json", "item", "work", "item-1", "--file", file})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/items/item-1/work" || method != "PATCH" || !strings.Contains(body, `"expected_content_version":2`) || !strings.Contains(body, `"title":"Corrected title"`) || !strings.Contains(body, `"reason":"Owner request"`) {
		t.Fatalf("bad work command: %s %s %s", method, path, body)
	}
	cmd = command()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"--server", server.URL, "--json", "item", "process-input", "item-1", "--file", file})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/items/item-1/inputs/process" || method != "POST" || !strings.Contains(body, `"expected_content_version":2`) {
		t.Fatalf("bad processing command: %s %s %s", method, path, body)
	}
	cmd = command()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"--server", server.URL, "item", "note", "item-1", "--file", file, "--actor", "agent"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/items/item-1/note" || method != "PUT" || !strings.Contains(body, `"actor":"agent"`) {
		t.Fatalf("CLI actor lost: %s %s %s", method, path, body)
	}
	cmd = command()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"--server", server.URL, "--json", "item", "list", "--executor", "agent:A & B", "--external-ref", "session:1?&", "--delegation-status", "pending,blocked"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/items" || method != "GET" || executor != "agent:A & B" || reference != "session:1?&" || status != "pending,blocked" {
		t.Fatalf("lost filters: %s %s %s %s %s", method, path, executor, reference, status)
	}
}
