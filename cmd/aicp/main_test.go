package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhuochun/control-plane/internal/client"
)

func TestExitCodeContract(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"validation", &client.Error{Status: 422, Code: "invalid_request"}, 2},
		{"conflict", &client.Error{Status: 409, Code: "revision_conflict"}, 3},
		{"unavailable", &client.Error{Code: "server_unavailable"}, 4},
		{"overlap", &client.Error{Status: 409, Code: "run_in_progress"}, 5},
		{"usage", errors.New("unknown flag: --wat"), 2},
		{"other", errors.New("disk failure"), 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := exitCode(test.err); got != test.want {
				t.Fatalf("exitCode() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestUserContextCommandSendsApprovedFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "USER.md")
	if err := os.WriteFile(file, []byte("# Owner\n\nReview releases."), 0600); err != nil {
		t.Fatal(err)
	}
	var path, method, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_md":"# Owner"}`))
	}))
	defer server.Close()
	cmd := command()
	cmd.SetArgs([]string{"--server", server.URL, "config", "user-context", "set", "--file", file})
	cmd.SetOut(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/settings/user-context" || method != http.MethodPut || !strings.Contains(body, `"user_md":"# Owner\n\nReview releases."`) {
		t.Fatalf("unexpected context request: %s %s %s", method, path, body)
	}
}
