package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/store"
)

func TestItemHTTPPagesKeepOrderAndRejectBadCursor(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	a := app.New(s)
	for i := 0; i < 5; i++ {
		_, err = a.PutItem(ctx, "", app.PutItem{RequestID: fmt.Sprintf("http-page-%d", i), DedupeKey: fmt.Sprintf("http-page:%d", i), Kind: "note", Title: fmt.Sprintf("Page item %d", i), Summary: "Paged item", Report: app.Report{SchemaVersion: 1, BodyMD: "Item body"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	handler := New(a, http.NotFoundHandler(), "test")
	var got []string
	path := "/api/v1/items?limit=2"
	for {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:7331"+path, nil))
		if response.Code != 200 {
			t.Fatalf("page: %d %s", response.Code, response.Body.String())
		}
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			NextCursor *string `json:"next_cursor"`
		}
		if err = json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			got = append(got, item.ID)
		}
		if page.NextCursor == nil {
			break
		}
		path = "/api/v1/items?limit=2&cursor=" + *page.NextCursor
	}
	all, err := a.Items(ctx, app.ItemFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(all) {
		t.Fatalf("got %d IDs, want %d", len(got), len(all))
	}
	for i, item := range all {
		if got[i] != item.ID {
			t.Fatalf("page %d: got %s, want %s", i, got[i], item.ID)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:7331/api/v1/items?cursor=bad", nil))
	if response.Code != 422 {
		t.Fatalf("accepted cursor outside collection: %d", response.Code)
	}
}

func TestSettingsHTTPBoundary(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
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
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	request := httptest.NewRequest("GET", "http://attacker.example/api/v1/status", nil)
	response := httptest.NewRecorder()
	New(app.New(s), http.NotFoundHandler(), "test").ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatalf("status %d", response.Code)
	}
}

func TestOwnerContextHTTPDoesNotClaimRun(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	handler := New(app.New(s), http.NotFoundHandler(), "test")
	call := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("PUT", "http://127.0.0.1:7331/api/v1/settings/user-context", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := call(`{}`); response.Code != 422 {
		t.Fatalf("missing owner context accepted: %d %s", response.Code, response.Body.String())
	}
	if response := call(`{"user_md":"# Owner\\n\\nReview releases."}`); response.Code != 200 {
		t.Fatalf("owner context save: %d %s", response.Code, response.Body.String())
	}
	settings, err := app.New(s).Settings(context.Background())
	if err != nil || !strings.Contains(settings.UserMD, "Review releases") {
		t.Fatalf("owner context not saved: %+v, %v", settings, err)
	}
	var count int
	if err = s.DB.QueryRow("SELECT count(*) FROM runs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("context edit started %d Runs", count)
	}
}

func TestWatcherConfigurationUsesNewShapeWithoutStartingRun(t *testing.T) {
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	handler := New(app.New(s), http.NotFoundHandler(), "test")
	call := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "http://127.0.0.1:7331/api/v1"+path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	interest := call("POST", "/interests", `{"slug":"delivery-risk","title":"Delivery risk"}`)
	if interest.Code != 200 {
		t.Fatalf("interest create: %d %s", interest.Code, interest.Body.String())
	}
	legacy := call("POST", "/watches", `{"interest_id":"delivery-risk","source":{"kind":"gmail","locator":"inbox"}}`)
	if legacy.Code != 400 {
		t.Fatalf("obsolete single-parent payload accepted: %d %s", legacy.Code, legacy.Body.String())
	}
	watch := call("POST", "/watches", `{"slug":"gmail-inbox","matching_policy":"broad","source":{"kind":"gmail","locator":"inbox"}}`)
	if watch.Code != 200 {
		t.Fatalf("Watcher create: %d %s", watch.Code, watch.Body.String())
	}
	got := call("GET", "/watches/gmail-inbox", "")
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"matching_policy":"broad"`) {
		t.Fatalf("slug lookup failed: %d %s", got.Code, got.Body.String())
	}
	var count int
	if err = s.DB.QueryRow("SELECT count(*) FROM runs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("configuration claimed a source Run: %d", count)
	}
}
