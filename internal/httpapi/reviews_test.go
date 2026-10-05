package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/store"
)

func TestReviewHTTPSubmissionBoundaryAndRetrieval(t *testing.T) {
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
	report := app.Report{SchemaVersion: 2, BodyMD: "A review", Blocks: []app.ReportBlock{{ID: "review", Type: "review", Title: "Review", Blocks: []app.ReportBlock{{ID: "input", Type: "text_input", Question: "Instructions?", Required: true}}}}}
	raw, err := a.PutItem(ctx, "", app.PutItem{DedupeKey: "http-review", Kind: "report", Title: "Review", Summary: "Need input", Report: report})
	if err != nil {
		t.Fatal(err)
	}
	var item app.Item
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	item, err = a.Item(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(a, http.NotFoundHandler(), "test")
	body, err := json.Marshal(app.SubmitReviewAnswer{RequestID: "http-answer", ReviewID: "review", ExpectedContentVersion: item.ContentVersion, ExpectedStateVersion: item.StateVersion, ReviewMaterialHash: item.ReviewMaterialHash, Values: []app.AnswerValue{{FieldID: "input", Disposition: "answered", Text: "Keep it local"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"", "https://foreign.example", "http://127.0.0.1:7331"} {
		request := httptest.NewRequest("POST", "http://127.0.0.1:7331/api/v1/items/"+item.ID+"/answers", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		want := 403
		if origin == "http://127.0.0.1:7331" {
			want = 200
		}
		if response.Code != want {
			t.Fatalf("origin %q: %d %s", origin, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:7331/api/v1/items/"+item.ID, nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Keep it local") || !strings.Contains(response.Body.String(), "Instructions?") {
		t.Fatalf("answer unavailable to next caller: %s", response.Body.String())
	}
}

func TestPlantUMLLocalRenderer(t *testing.T) {
	jar := os.Getenv("AICP_TEST_PLANTUML_JAR")
	if jar == "" {
		t.Skip("set AICP_TEST_PLANTUML_JAR for a real local renderer test")
	}
	t.Setenv("AICP_PLANTUML_JAR", jar)
	renderer := newPlantUMLRenderer()
	if renderer == nil {
		t.Fatal("configured local renderer unavailable")
	}
	data, err := renderer.render(context.Background(), "@startuml\nAlice -> Bob: Review\n@enduml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatalf("not a rendered PNG: %v", err)
	}
	for _, source := range []string{"@startuml\n!include https://example.com/private\n@enduml", "@startuml\nAlice -> Bob: %getenv(SECRET)\n@enduml", "@startuml\nthis is invalid diagram syntax !!!\n@enduml"} {
		if _, err := renderer.render(context.Background(), source); err == nil {
			t.Fatal("accepted external/preprocessor source")
		}
	}
}

func TestPlantUMLUnavailableHasExplicitFailure(t *testing.T) {
	var renderer *plantUMLRenderer
	if _, err := renderer.render(context.Background(), "@startuml\nA -> B\n@enduml"); err == nil {
		t.Fatal("missing renderer succeeded")
	}
}

func TestPlantUMLRejectsUnusableConfiguredJar(t *testing.T) {
	jar := filepath.Join(t.TempDir(), "invalid.jar")
	if err := os.WriteFile(jar, []byte("not a Java archive"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AICP_PLANTUML_JAR", jar)
	if renderer := newPlantUMLRenderer(); renderer != nil {
		t.Fatal("advertised an unusable renderer")
	}
}

func TestPlantUMLSetupDiscovery(t *testing.T) {
	t.Setenv("AICP_PLANTUML_JAR", "")
	mux := http.NewServeMux()
	reviewRoutes(mux, nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:7331/api/v1/review-capabilities", nil))
	var capabilities struct {
		Available bool   `json:"plantuml_available"`
		Setup     string `json:"plantuml_setup"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &capabilities); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || capabilities.Available || !strings.Contains(capabilities.Setup, "AICP_PLANTUML_JAR") {
		t.Fatalf("missing setup guidance: %s", response.Body.String())
	}
	for _, platform := range []string{"darwin", "windows", "linux"} {
		setup := plantUMLSetup(platform)
		if !strings.Contains(setup, "Restart aicp") || !strings.Contains(setup, "PATH") {
			t.Fatalf("incomplete %s setup: %s", platform, setup)
		}
		if platform == "darwin" && !strings.Contains(setup, "brew install plantuml") {
			t.Fatal("missing Homebrew installation guidance")
		}
	}
}
