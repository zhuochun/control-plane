package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewCLIRegistersAndLooksUpExactVersion(t *testing.T) {
	var path, method string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"format"}`))
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "format.json")
	if err := os.WriteFile(file, []byte(`{"format":{"id":"format","version":1,"title":"Choice","fields":[{"id":"input","type":"text_input"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := command()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"--server", server.URL, "review", "register", "--file", file})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/review-formats" || method != "POST" {
		t.Fatalf("register: %s %s", method, path)
	}
	cmd = command()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"--server", server.URL, "review", "format", "format", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/review-formats/format/1" || method != "GET" {
		t.Fatalf("lookup: %s %s", method, path)
	}
}
