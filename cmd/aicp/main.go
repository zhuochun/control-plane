package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

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
	root := &cobra.Command{Use: "aicp", Short: "A little space for what matters", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&dataDir, "data-dir", dataDir, "Local data directory (serve only)")
	root.PersistentFlags().StringVar(&server, "server", server, "Local server URL")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Print machine-readable JSON")
	printResult := func(cmd *cobra.Command, method, path string, body any) error {
		result, err := client.New(server).Do(cmd.Context(), method, path, body)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), string(result))
		return err
	}
	serve := &cobra.Command{Use: "serve", Short: "Serve the local portal", RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Open(cmd.Context(), dataDir)
		if err != nil {
			return err
		}
		defer s.Close()
		listener, err := net.Listen("tcp", "127.0.0.1:7331")
		if err != nil {
			return fmt.Errorf("listen on 127.0.0.1:7331 (another service may be using the port): %w", err)
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
		slog.Info("aicp is ready", "url", "http://127.0.0.1:7331")
		err = srv.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		<-stopped
		return nil
	}}
	root.AddCommand(serve)
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
			fmt.Fprintln(cmd.OutOrStdout(), server)
		}
		return nil
	}})
	config := &cobra.Command{Use: "config", Short: "View or change local preferences"}
	config.AddCommand(&cobra.Command{Use: "get", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return printResult(cmd, "GET", "/settings", nil) }})
	config.AddCommand(&cobra.Command{Use: "set <timezone>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return printResult(cmd, "PATCH", "/settings", app.SetSettings{RequestID: uuid.NewString(), Timezone: args[0]})
	}})
	root.AddCommand(config)
	root.AddCommand(configurationCommand("interest", "/interests", printResult), configurationCommand("watch", "/watches", printResult))
	root.AddCommand(itemCommand(printResult))
	root.AddCommand(runCommand(printResult))
	root.AddCommand(changesCommand(printResult))
	root.AddCommand(proposalCommand(printResult))
	root.AddCommand(&cobra.Command{Use: "brief", Args: cobra.NoArgs, Short: "Read due work and human changes", RunE: func(cmd *cobra.Command, args []string) error { return printResult(cmd, "GET", "/brief", nil) }})
	root.AddCommand(&cobra.Command{Use: "mcp", Args: cobra.NoArgs, Short: "Serve MCP over stdio using the local HTTP server", RunE: func(cmd *cobra.Command, args []string) error {
		return mcpserver.New(server, version).Run(cmd.Context(), &mcp.StdioTransport{})
	}})
	return root
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
