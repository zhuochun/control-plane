package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type request func(*cobra.Command, string, string, any) error

func configurationCommand(name, path string, send request) *cobra.Command {
	root := &cobra.Command{Use: name, Short: "Manage " + name + " configuration"}
	if name == "watch" {
		root.Aliases = []string{"watcher"}
	}
	var cursor, state string
	var all bool
	var limit int
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		query := url.Values{"limit": {fmt.Sprint(limit)}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		if state != "" {
			query.Set("state", state)
		}
		if all {
			query.Set("all", "1")
		}
		return send(cmd, "GET", path+"?"+query.Encode(), nil)
	}}
	list.Flags().StringVar(&cursor, "cursor", "", "Continuation cursor")
	list.Flags().IntVar(&limit, "limit", 50, "Page size (1–100)")
	list.Flags().StringVar(&state, "state", "", "Lifecycle state")
	list.Flags().BoolVar(&all, "all", false, "Include inactive records")
	root.AddCommand(list, &cobra.Command{Use: "get <slug-or-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return send(cmd, "GET", path+"/"+url.PathEscape(args[0]), nil)
	}})
	for _, operation := range []string{"create", "update"} {
		var file, slug, title, instructions, instructionsFile, sourceKind, sourceLocator, policy, validUntil, requestID string
		var interests []string
		var interval, lookback, revision int64
		var clearInterests, clearValidUntil bool
		use, args := "create", cobra.NoArgs
		if operation == "update" {
			use, args = "update <slug-or-id>", cobra.ExactArgs(1)
		}
		command := &cobra.Command{Use: use, Args: args}
		command.RunE = func(cmd *cobra.Command, args []string) error {
			method, target := "POST", path
			if operation == "update" {
				method, target = "PATCH", path+"/"+url.PathEscape(args[0])
			}
			if file != "" {
				body, err := commandFile(cmd, file)
				if err != nil {
					return err
				}
				return send(cmd, method, target, body)
			}
			if requestID == "" {
				requestID = uuid.NewString()
			}
			body := map[string]any{"request_id": requestID}
			if operation == "update" {
				if !cmd.Flags().Changed("revision") {
					return fmt.Errorf("--revision is required for update")
				}
				body["expected_revision"] = revision
			}
			put := func(flag, key string, value any) {
				if operation == "create" || cmd.Flags().Changed(flag) {
					body[key] = value
				}
			}
			put("slug", "slug", slug)
			if name == "interest" {
				put("title", "title", title)
			} else {
				if operation == "create" || cmd.Flags().Changed("source-kind") || cmd.Flags().Changed("source-locator") {
					if sourceKind == "" || sourceLocator == "" {
						return fmt.Errorf("changing a source requires both --source-kind and --source-locator")
					}
					body["source"] = map[string]string{"kind": sourceKind, "locator": sourceLocator}
				}
				put("matching-policy", "matching_policy", policy)
				if operation == "create" || cmd.Flags().Changed("interest") {
					body["interest_ids"] = interests
				}
				if clearInterests {
					body["interest_ids"] = []string{}
				}
				if cmd.Flags().Changed("interval") {
					body["interval_seconds"] = interval
				}
				if cmd.Flags().Changed("lookback") {
					body["lookback_seconds"] = lookback
				}
				if cmd.Flags().Changed("valid-until") {
					if clearValidUntil {
						return fmt.Errorf("--valid-until and --clear-valid-until cannot be combined")
					}
					parsed, err := time.Parse(time.RFC3339, validUntil)
					if err != nil {
						return fmt.Errorf("--valid-until must be RFC3339: %w", err)
					}
					body["valid_until"] = parsed
				}
				if clearValidUntil {
					body["clear_valid_until"] = true
				}
			}
			if instructionsFile != "" {
				data, err := os.ReadFile(instructionsFile)
				if err != nil {
					return err
				}
				if !utf8.Valid(data) {
					return fmt.Errorf("instructions file must be UTF-8")
				}
				instructions = string(data)
			}
			if operation == "create" || cmd.Flags().Changed("instructions") || cmd.Flags().Changed("instructions-file") {
				body["instructions_md"] = instructions
			}
			if cmd.Flags().Changed("state") {
				body["state"] = state
			}
			if operation == "create" {
				if name == "interest" && strings.TrimSpace(title) == "" {
					return fmt.Errorf("--title is required")
				}
				if name == "watch" && (sourceKind == "" || sourceLocator == "" || policy == "") {
					return fmt.Errorf("--source-kind, --source-locator, and --matching-policy are required")
				}
			}
			return send(cmd, method, target, body)
		}
		command.Flags().StringVar(&file, "file", "", "JSON command file, or - for stdin")
		command.Flags().StringVar(&requestID, "request-id", "", "Idempotency key (generated when omitted)")
		command.Flags().StringVar(&slug, "slug", "", "Stable human-readable slug")
		command.Flags().StringVar(&instructions, "instructions", "", "Interpretation or source-specific instructions")
		command.Flags().StringVar(&instructionsFile, "instructions-file", "", "Read instructions from a UTF-8 file")
		command.Flags().StringVar(&state, "state", "", "active, paused, or deprecated")
		command.Flags().Int64Var(&revision, "revision", 0, "Expected revision for update")
		if name == "interest" {
			command.Flags().StringVar(&title, "title", "", "Interest title")
		} else {
			command.Flags().StringVar(&sourceKind, "source-kind", "", "Source connector kind")
			command.Flags().StringVar(&sourceLocator, "source-locator", "", "Bounded source scope")
			command.Flags().StringVar(&policy, "matching-policy", "", "broad or explicit")
			command.Flags().StringSliceVar(&interests, "interest", nil, "Applicable Interest slug (repeatable)")
			command.Flags().BoolVar(&clearInterests, "clear-interests", false, "Remove explicit Interest links")
			command.Flags().Int64Var(&interval, "interval", 7200, "Inspection interval in seconds")
			command.Flags().Int64Var(&lookback, "lookback", 604800, "Lookback in seconds")
			command.Flags().StringVar(&validUntil, "valid-until", "", "Optional RFC3339 end of temporary Watcher")
			command.Flags().BoolVar(&clearValidUntil, "clear-valid-until", false, "Remove a Watcher's validity end")
		}
		root.AddCommand(command)
	}
	return root
}

func commandFile(cmd *cobra.Command, path string) (json.RawMessage, error) {
	source := cmd.InOrStdin()
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		source = file
	}
	data, err := io.ReadAll(io.LimitReader(source, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 || !json.Valid(data) {
		return nil, fmt.Errorf("file must contain valid JSON under 4 MiB")
	}
	return json.RawMessage(data), nil
}
