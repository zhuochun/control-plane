package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/store"
)

func TestSettingsHTTPBoundary(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	handler := New(app.New(s), http.NotFoundHandler(), "test")
	tests := []struct {
		name, body, origin string
		want               int
	}{
		{"valid", `{"request_id":"one","timezone":"Asia/Singapore"}`, "http://127.0.0.1:7331", 200},
		{"replay", `{"request_id":"one","timezone":"Asia/Singapore"}`, "", 200},
		{"conflict", `{"request_id":"one","timezone":"UTC"}`, "", 409},
		{"unknown field", `{"request_id":"two","timezone":"UTC","extra":true}`, "", 400},
		{"trailing value", `{"request_id":"two","timezone":"UTC"} {}`, "", 400},
		{"foreign origin", `{"request_id":"two","timezone":"UTC"}`, "https://example.com", 403},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("PATCH", "http://127.0.0.1:7331/api/v1/settings", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", test.origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status %d, want %d: %s", response.Code, test.want, response.Body.String())
			}
		})
	}
	settings, err := app.New(s).Settings(context.Background())
	if err != nil || settings.Timezone != "Asia/Singapore" {
		t.Fatalf("rejected requests changed settings: %+v, %v", settings, err)
	}
}

func TestRejectsNonLoopbackHost(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	request := httptest.NewRequest("GET", "http://attacker.example/api/v1/status", nil)
	response := httptest.NewRecorder()
	New(app.New(s), http.NotFoundHandler(), "test").ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatalf("status %d", response.Code)
	}
}
