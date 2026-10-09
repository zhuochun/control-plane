package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/store"
)

func TestWorkHTTPBoundsAndRecovery(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	a := app.New(s)
	raw, err := a.PutItem(ctx, "", app.PutItem{DedupeKey: "http:work", Kind: "note", Title: "Spec", Summary: "New", Report: app.Report{SchemaVersion: 1, BodyMD: "Original"}})
	if err != nil {
		t.Fatal(err)
	}
	var item app.Item
	if err = json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	handler := New(a, http.NotFoundHandler(), "test")
	call := func(method, path, body string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, "http://127.0.0.1:7331/api/v1"+path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, response.Code, response.Body.String())
		}
		return response.Body.Bytes()
	}
	path := "/items/" + item.ID + "/work"
	body := `{"request_id":"http-handoff","expected_content_version":1,"title":"Clarified title","summary":"Corrected summary","reason":"Owner requested correction","delegations":[{"id":"research","executor":"agent:A & B","external_ref":"session:1?&","instructions_md":"Read spec v1","status":"pending","context_md":"Input Item v1"}]}`
	first := call("PATCH", path, body, 200)
	var edited app.Item
	if err = json.Unmarshal(first, &edited); err != nil || edited.Title != "Clarified title" || edited.Summary != "Corrected summary" || edited.Origin != "user" || edited.StateVersion != item.StateVersion {
		t.Fatalf("simple HTTP edit lost fields/ownership: %s %v", first, err)
	}
	call("PUT", "/items/"+item.ID+"/note", `{"actor":"agent","expected_state_version":1,"user_note":"Transcribed owner instruction"}`, 200)
	var actor string
	if err = s.DB.QueryRowContext(ctx, "SELECT actor FROM events WHERE entity_id=? AND change_type='item.note_updated'", item.ID).Scan(&actor); err != nil || actor != "agent" {
		t.Fatalf("HTTP actor lost: %s %v", actor, err)
	}
	call("PUT", "/items/"+item.ID+"/note", `{"actor":"unrecognized","expected_state_version":2,"user_note":"Invalid"}`, 422)
	call("PATCH", path, `{"expected_content_version":1,"context_md":"Stale"}`, 409)
	call("PATCH", path, `{"expected_content_version":2,"watch_id":"changed"}`, 400)
	call("PATCH", path, `{"expected_content_version":2,"user_note":"changed"}`, 400)
	call("PATCH", "/items/missing/work", `{"expected_content_version":1}`, 404)
	if string(call("PATCH", path, body, 200)) != string(first) {
		t.Fatal("HTTP retry changed response")
	}
	query := url.Values{"executor": {"agent:A & B"}, "external_ref": {"session:1?&"}, "delegation_status": {"pending,blocked"}, "limit": {"1"}}
	var found struct {
		Items []app.Item `json:"items"`
	}
	if err = json.Unmarshal(call("GET", "/items?"+query.Encode(), "", 200), &found); err != nil {
		t.Fatal(err)
	}
	if len(found.Items) != 1 || found.Items[0].ID != item.ID || len(found.Items[0].MatchingDelegationIDs) != 1 || found.Items[0].MatchingDelegationIDs[0] != "research" {
		t.Fatalf("missing matching handoff: %+v", found)
	}
	call("GET", "/items?delegation_status=running", "", 422)
}
