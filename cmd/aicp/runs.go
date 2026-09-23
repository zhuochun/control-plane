package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func runCommand(send request) *cobra.Command {
	root := &cobra.Command{Use: "run", Short: "Claim and record agent work"}
	var cursor string
	var limit int
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		values := url.Values{"limit": {fmt.Sprint(limit)}}
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		return send(cmd, "GET", "/runs?"+values.Encode(), nil)
	}}
	list.Flags().StringVar(&cursor, "cursor", "", "Continuation cursor")
	list.Flags().IntVar(&limit, "limit", 50, "Page size (1–100)")
	root.AddCommand(list)
	root.AddCommand(&cobra.Command{Use: "get <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return send(cmd, "GET", "/runs/"+url.PathEscape(args[0]), nil)
	}})
	var runnerLabel string
	var watchIDs []string
	var force bool
	start := &cobra.Command{Use: "start", Args: cobra.NoArgs, Short: "Check in and receive one heartbeat work packet", RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]any{"runner_label": runnerLabel, "force": force}
		if len(watchIDs) > 0 {
			body["watch_ids"] = watchIDs
		}
		return send(cmd, "POST", "/runs", body)
	}}
	start.Flags().StringVar(&runnerLabel, "runner-label", "", "Optional label for the external agent")
	start.Flags().StringSliceVar(&watchIDs, "watch", nil, "Select a Watch (repeatable; normally due Watches are selected)")
	start.Flags().BoolVar(&force, "force", false, "Run selected Watches even when not due")
	root.AddCommand(start)
	var summary string
	var ackThroughSeq int64
	finish := &cobra.Command{Use: "finish", Args: cobra.NoArgs, Short: "Check out after all selected Watches have coverage", RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]any{"summary": summary}
		if cmd.Flags().Changed("ack-through-seq") {
			body["ack_through_seq"] = ackThroughSeq
		}
		return send(cmd, "POST", "/runs/finish", body)
	}}
	finish.Flags().StringVar(&summary, "summary", "", "Run summary")
	finish.Flags().Int64Var(&ackThroughSeq, "ack-through-seq", 0, "Acknowledge the captured event range through this sequence")
	root.AddCommand(finish)
	var findingsFile string
	findings := &cobra.Command{Use: "submit <watch-id>", Aliases: []string{"findings"}, Args: cobra.ExactArgs(1), Short: "Submit one final Watch finding result", RunE: func(cmd *cobra.Command, args []string) error {
		body, err := commandFile(cmd, findingsFile)
		if err != nil {
			return err
		}
		return send(cmd, "PUT", "/runs/watches/"+url.PathEscape(strings.TrimSpace(args[0]))+"/findings", body)
	}}
	findings.Flags().StringVar(&findingsFile, "file", "", "JSON Watch findings payload, or - for stdin")
	_ = findings.MarkFlagRequired("file")
	root.AddCommand(findings)
	return root
}

func changesCommand(send request) *cobra.Command {
	var after, through int64
	var cursor string
	var limit int
	command := &cobra.Command{Use: "changes", Args: cobra.NoArgs, Short: "Read a captured event range", RunE: func(cmd *cobra.Command, args []string) error {
		values := url.Values{"after_seq": {fmt.Sprint(after)}, "through_seq": {fmt.Sprint(through)}, "limit": {fmt.Sprint(limit)}}
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		return send(cmd, "GET", "/changes?"+values.Encode(), nil)
	}}
	command.Flags().Int64Var(&after, "after-seq", 0, "Exclusive lower event sequence")
	command.Flags().Int64Var(&through, "through-seq", 0, "Inclusive captured upper event sequence")
	command.Flags().StringVar(&cursor, "cursor", "", "Continuation cursor")
	command.Flags().IntVar(&limit, "limit", 50, "Page size (1–100)")
	_ = command.MarkFlagRequired("through-seq")
	return command
}
