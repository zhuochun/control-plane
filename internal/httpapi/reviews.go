package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/zhuochun/control-plane/internal/app"
)

// Render configuration is operator-owned and captured when the server starts.
// Diagram requests cannot provide commands, paths, environment, or flags.
type plantUMLRenderer struct {
	jar   string
	java  string
	slots chan struct{}
}

func newPlantUMLRenderer() *plantUMLRenderer {
	jar := os.Getenv("AICP_PLANTUML_JAR")
	java, err := exec.LookPath("java")
	info, statErr := os.Stat(jar)
	if jar == "" || !filepath.IsAbs(jar) || statErr != nil || !info.Mode().IsRegular() || err != nil {
		return nil
	}
	renderer := &plantUMLRenderer{jar: jar, java: java, slots: make(chan struct{}, 2)}
	// File presence does not establish Java/JAR compatibility. Exercise the same
	// sandboxed path used for previews before advertising availability.
	if _, err := renderer.renderWithTimeout(context.Background(), "@startuml\nAlice -> Bob: Ready\n@enduml", 30*time.Second); err != nil {
		return nil
	}
	return renderer
}

func plantUMLSetup(platform string) string {
	switch platform {
	case "darwin":
		return "Install with brew install plantuml; add Homebrew openjdk/bin to PATH and set AICP_PLANTUML_JAR to the absolute Homebrew plantuml/libexec/plantuml.jar path. Restart aicp."
	case "windows":
		return "Install Java 11 or newer, download the official PlantUML JAR, add Java/bin to PATH, and set AICP_PLANTUML_JAR to the absolute JAR path. Restart aicp."
	default:
		return "Install a compatible Java runtime (11 or newer) and the official PlantUML JAR; add Java/bin to PATH and set AICP_PLANTUML_JAR to the absolute JAR path. Some diagrams require Graphviz. Restart aicp."
	}
}

type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	if b.Len()+len(data) > b.limit {
		return 0, fmt.Errorf("diagram output exceeds limit")
	}
	return b.Buffer.Write(data)
}

func (p *plantUMLRenderer) render(ctx context.Context, source string) ([]byte, error) {
	return p.renderWithTimeout(ctx, source, 10*time.Second)
}

func (p *plantUMLRenderer) renderWithTimeout(ctx context.Context, source string, timeout time.Duration) ([]byte, error) {
	if p == nil {
		return nil, &app.Error{Status: 503, Code: "renderer_unavailable", Message: "Local PlantUML rendering is unavailable or failed its startup check. " + plantUMLSetup(runtime.GOOS)}
	}
	if strings.TrimSpace(source) == "" || len(source) > 32<<10 {
		return nil, app.Invalid("diagram source must contain 1–32768 bytes")
	}
	// Initial syntax subset excludes external preprocessor resources, environment
	// functions, and image embedding. SANDBOX also denies filesystem/network reads.
	lower := strings.ToLower(source)
	for _, forbidden := range []string{"!include", "!import", "%", "<img", "!theme"} {
		if strings.Contains(lower, forbidden) {
			return nil, app.Invalid("PlantUML external resources, themes, and functions are unsupported")
		}
	}
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	default:
		return nil, &app.Error{Status: 503, Code: "renderer_busy", Message: "Diagram renderer is busy; retry shortly"}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, p.java, "-DPLANTUML_SECURITY_PROFILE=SANDBOX", "-Xmx128m", "-jar", p.jar, "-pipe", "-tpng", "-failfast2")
	command.Stdin = strings.NewReader(source)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "JAVA_TOOL_OPTIONS") && !strings.EqualFold(key, "JDK_JAVA_OPTIONS") && !strings.EqualFold(key, "_JAVA_OPTIONS") {
			command.Env = append(command.Env, entry)
		}
	}
	output := &boundedOutput{limit: 2 << 20}
	stderr := &boundedOutput{limit: 16 << 10}
	command.Stdout, command.Stderr = output, stderr
	if err := command.Run(); err != nil {
		return nil, app.Invalid("PlantUML rendering failed; check syntax, local dependencies, and rendering limits")
	}
	config, err := png.DecodeConfig(bytes.NewReader(output.Bytes()))
	if err != nil || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 16000000 {
		return nil, app.Invalid("PlantUML output is invalid or too large")
	}
	return output.Bytes(), nil
}

func reviewRoutes(mux *http.ServeMux, a *app.App) {
	renderer := newPlantUMLRenderer()
	mux.HandleFunc("GET /api/v1/review-capabilities", read(func(_ *http.Request) (any, error) {
		return map[string]any{"report_schema_versions": []int{1, 2}, "block_types": []string{"markdown", "image", "diagram", "review", "choice", "text_input", "actions"}, "diagram_languages": []string{"mermaid", "plantuml"}, "plantuml_available": renderer != nil, "plantuml_setup": plantUMLSetup(runtime.GOOS), "image_types": []string{"image/png", "image/jpeg"}, "image_max_bytes": 2 << 20, "diagram_max_bytes": 32 << 10}, nil
	}))
	mux.HandleFunc("GET /api/v1/review-formats", read(func(r *http.Request) (any, error) {
		formats, err := a.ReviewFormats(r.Context())
		return map[string]any{"items": formats}, err
	}))
	mux.HandleFunc("GET /api/v1/review-formats/{id}/{version}", read(func(r *http.Request) (any, error) {
		version, err := strconv.Atoi(r.PathValue("version"))
		if err != nil || version < 1 {
			return nil, app.Invalid("format version must be positive")
		}
		return a.ReviewFormat(r.Context(), app.FormatRef{ID: r.PathValue("id"), Version: version})
	}))
	mux.HandleFunc("POST /api/v1/review-formats", command(func(r *http.Request, input app.RegisterReviewFormat) (any, error) {
		return a.RegisterReviewFormat(r.Context(), input)
	}))
	mux.HandleFunc("POST /api/v1/review-artifacts", command(func(r *http.Request, input app.UploadReviewArtifact) (any, error) {
		return a.UploadReviewArtifact(r.Context(), input)
	}))
	mux.HandleFunc("GET /api/v1/review-artifacts/{id}", func(w http.ResponseWriter, r *http.Request) {
		media, data, err := a.ReviewArtifact(r.Context(), r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", media)
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("POST /api/v1/review-diagrams", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Source string `json:"source"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, err)
			return
		}
		data, err := renderer.render(r.Context(), input.Source)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})
	// Portal-only exposure reduces accidental agent submissions. Loopback/origin
	// checks are not authenticated proof of a human; see the specification.
	mux.HandleFunc("POST /api/v1/items/{id}/answers", command(func(r *http.Request, input app.SubmitReviewAnswer) (any, error) {
		if r.Header.Get("Origin") != "http://"+r.Host {
			return nil, &app.Error{Status: 403, Code: "portal_required", Message: "Submit answers through the local portal"}
		}
		return a.SubmitReviewAnswer(r.Context(), r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /api/v1/items/{id}/answers", read(func(r *http.Request) (any, error) {
		after := int64(0)
		if raw := r.URL.Query().Get("after"); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 0 {
				return nil, app.Invalid("after must be a nonnegative answer sequence")
			}
			after = value
		}
		return a.AnswerHistory(r.Context(), r.PathValue("id"), after)
	}))
}
