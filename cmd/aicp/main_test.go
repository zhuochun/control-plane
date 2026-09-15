package main

import (
	"errors"
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
