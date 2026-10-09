package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/zhuochun/control-plane/internal/app"
	"github.com/zhuochun/control-plane/internal/client"
	"github.com/zhuochun/control-plane/internal/httpapi"
	"github.com/zhuochun/control-plane/internal/mcpserver"
	"github.com/zhuochun/control-plane/internal/store"
	"github.com/zhuochun/control-plane/web"
)

var version = "0.1.0-dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := command().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCode(err))
	}
}

func exitCode(err error) int {
	var remote *client.Error
	if errors.As(err, &remote) {
		if remote.Code == "run_in_progress" {
			return 5
		}
		if remote.Code == "server_unavailable" {
			return 4
		}
		if remote.Status == 409 {
			return 3
		}
		if remote.Status == 400 || remote.Status == 413 || remote.Status == 422 {
			return 2
		}
		return 1
	}
	message := err.Error()
	if strings.Contains(message, "arg(s)") || strings.Contains(message, "unknown flag") || strings.Contains(message, "required flag") {
		return 2
	}
	return 1
}

func command() *cobra.Command {
	directory, _ := os.UserConfigDir()
	dataDir := env("AICP_DATA_DIR", filepath.Join(directory, "control-plane"))
	server := env("AICP_SERVER_URL", "http://127.0.0.1:7331")
	var jsonOutput bool
	root := &cobra.Command{Use: "aicp", Short: "A local control plane for agent work", Long: `aicp keeps the durable plan and results of agent work.

An Interest says why a finding matters. A Watcher names a bounded source to
inspect. A Run records one actual inspection. An Item retains a matter to
review or act on. Your external agent inspects sources; aicp stores the plan,
context, coverage, and findings. A scheduler is optional and external.

Start with init, serve, and doctor. Set owner context, an Interest, and an
applicable Watcher; then read brief without claiming a Run. See the packaged
GETTING_STARTED.md for the full agent-assisted path.`, SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&dataDir, "data-dir", dataDir, "Local data directory (init and serve)")
	root.PersistentFlags().StringVar(&server, "server", server, "Local server URL")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Print machine-readable JSON")
	printResult := func(cmd *cobra.Command, method, path string, body any) error {
		result, err := client.New(server).Do(cmd.Context(), method, path, body)
		if err != nil {
			return err
		}
		if jsonOutput {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(result))
			return err
		}
		var value any
		if err = json.Unmarshal(result, &value); err != nil {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(result))
			return err
		}
		shortenIDs(value)
		pretty, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(pretty))
		return err
	}
	var servePort int
	serve := &cobra.Command{Use: "serve", Short: "Serve the local portal", RunE: func(cmd *cobra.Command, args []string) error {
		if servePort < 1 || servePort > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
		s, err := store.Open(cmd.Context(), dataDir)
		if err != nil {
			return err
		}
		defer func() {
			if err := s.Close(); err != nil {
				slog.Error("close store", "error", err)
			}
		}()
		address := fmt.Sprintf("127.0.0.1:%d", servePort)
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("listen on %s (another service may be using the port): %w", address, err)
		}
		srv := &http.Server{Handler: httpapi.New(app.New(s), web.Handler(), version), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			<-cmd.Context().Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()
		slog.Info("aicp is ready", "url", "http://"+address)
		err = srv.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		<-stopped
		return nil
	}}
	serve.Flags().IntVar(&servePort, "port", 7331, "Local loopback port (1-65535)")
	root.AddCommand(serve)
	root.AddCommand(&cobra.Command{Use: "init", Args: cobra.NoArgs, Short: "Initialize local data and default agent guidance", RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Open(cmd.Context(), dataDir)
		if err != nil {
			return err
		}
		if err = s.Close(); err != nil {
			return err
		}
		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"status": "initialized", "data_dir": dataDir})
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "aicp is initialized.", "")
		if err == nil {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Next: express what matters as an Interest, attach a source with a Watch, and add your priorities and context in USER.md from Preferences.")
		}
		if err == nil {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Keep `aicp serve` running, register `aicp mcp --server http://127.0.0.1:7331` with your agent harness, and let your scheduler start the external AI heartbeat.")
		}
		return err
	}})
	root.AddCommand(&cobra.Command{Use: "version", Short: "Print the executable version", RunE: func(cmd *cobra.Command, args []string) error {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"version": version})
	}})
	root.AddCommand(&cobra.Command{Use: "doctor", Short: "Check the local server", RunE: func(cmd *cobra.Command, args []string) error { return printResult(cmd, "GET", "/status", nil) }})
	root.AddCommand(&cobra.Command{Use: "open", Short: "Open the portal", RunE: func(cmd *cobra.Command, args []string) error {
		var browser *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			browser = exec.Command("rundll32", "url.dll,FileProtocolHandler", server)
		case "darwin":
			browser = exec.Command("open", server)
		default:
			browser = exec.Command("xdg-open", server)
		}
		if err := browser.Run(); err != nil {
			if _, writeErr := fmt.Fprintln(cmd.OutOrStdout(), server); writeErr != nil {
				return writeErr
			}
		}
		return nil
	}})
	config := &cobra.Command{Use: "config", Short: "View or change local preferences"}
	config.AddCommand(&cobra.Command{Use: "get", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return printResult(cmd, "GET", "/settings", nil) }})
	config.AddCommand(&cobra.Command{Use: "set <timezone>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return printResult(cmd, "PATCH", "/settings", app.SetSettings{RequestID: uuid.NewString(), Timezone: args[0]})
	}})
	userContext := &cobra.Command{Use: "user-context", Short: "Read or save owner context"}
	userContext.AddCommand(&cobra.Command{Use: "get", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return printResult(cmd, "GET", "/settings", nil) }})
	var userContextFile string
	setUserContext := &cobra.Command{Use: "set", Args: cobra.NoArgs, Short: "Save an approved USER.md draft from a UTF-8 file", RunE: func(cmd *cobra.Command, args []string) error {
		body, err := os.ReadFile(userContextFile)
		if err != nil {
			return err
		}
		if !utf8.Valid(body) {
			return fmt.Errorf("USER.md file must be UTF-8")
		}
		value := string(body)
		return printResult(cmd, "PUT", "/settings/user-context", app.SetUserContext{RequestID: uuid.NewString(), UserMD: &value})
	}}
	setUserContext.Flags().StringVar(&userContextFile, "file", "", "Approved USER.md file")
	_ = setUserContext.MarkFlagRequired("file")
	userContext.AddCommand(setUserContext)
	config.AddCommand(userContext)
	var planFile, previewToken, planRequestID string
	var dryRun bool
	planCommand := &cobra.Command{Use: "plan", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		body, err := commandFile(cmd, planFile)
		if err != nil {
			return err
		}
		return printResult(cmd, "POST", "/config/plans/preview", body)
	}}
	planCommand.Flags().StringVar(&planFile, "file", "", "JSON change-set file")
	_ = planCommand.MarkFlagRequired("file")
	applyCommand := &cobra.Command{Use: "apply", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		body, err := commandFile(cmd, planFile)
		if err != nil {
			return err
		}
		if dryRun {
			return printResult(cmd, "POST", "/config/plans/preview", body)
		}
		if previewToken == "" {
			return fmt.Errorf("--preview-token is required; run config plan first")
		}
		return printResult(cmd, "POST", "/config/plans/apply", map[string]any{"request_id": planRequestID, "preview_token": previewToken, "plan": json.RawMessage(body)})
	}}
	applyCommand.Flags().StringVar(&planFile, "file", "", "JSON change-set file")
	applyCommand.Flags().StringVar(&previewToken, "preview-token", "", "Token from the reviewed preview")
	applyCommand.Flags().StringVar(&planRequestID, "request-id", "", "Idempotency key")
	applyCommand.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without applying")
	_ = applyCommand.MarkFlagRequired("file")
	config.AddCommand(planCommand, applyCommand)
	root.AddCommand(config)
	root.AddCommand(configurationCommand("interest", "/interests", printResult), configurationCommand("watch", "/watches", printResult))
	root.AddCommand(itemCommand(printResult))
	root.AddCommand(reviewCommand(printResult))
	root.AddCommand(runCommand(printResult))
	root.AddCommand(changesCommand(printResult))
	root.AddCommand(proposalCommand(printResult))
	var briefCursor string
	brief := &cobra.Command{Use: "brief", Args: cobra.NoArgs, Short: "Read a live overview without claiming source work", RunE: func(cmd *cobra.Command, args []string) error {
		path := "/brief"
		if briefCursor != "" {
			path += "?cursor=" + url.QueryEscape(briefCursor)
		}
		return printResult(cmd, "GET", path, nil)
	}}
	brief.Flags().StringVar(&briefCursor, "cursor", "", "Continuation cursor from the brief or Run packet")
	root.AddCommand(brief)
	root.AddCommand(&cobra.Command{Use: "mcp", Args: cobra.NoArgs, Short: "Serve MCP over stdio using the local HTTP server", RunE: func(cmd *cobra.Command, args []string) error {
		return mcpserver.New(server, version).Run(cmd.Context(), &mcp.StdioTransport{})
	}})
	return root
}

func shortenID(value string) string {
	if index := strings.IndexByte(value, '-'); index > 0 {
		return value[:index]
	}
	return value
}

func shortenIDs(value any) {
	keys := map[string]bool{"id": true, "run_id": true, "interest_id": true, "watch_id": true, "item_id": true, "proposal_id": true, "parent_id": true}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if keys[key] {
				if text, ok := child.(string); ok {
					typed[key] = shortenID(text)
				}
				continue
			}
			shortenIDs(child)
		}
	case []any:
		for _, child := range typed {
			shortenIDs(child)
		}
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
