package main

import (
	"fmt"
	"net/url"

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
	for _, operation := range []string{"start", "renew", "finish"} {
		operation := operation
		var file string
		use := operation
		if operation != "start" {
			use += " <run-id>"
		}
		command := &cobra.Command{Use: use, Args: func() cobra.PositionalArgs {
			if operation == "start" {
				return cobra.NoArgs
			}
			return cobra.ExactArgs(1)
		}(), RunE: func(cmd *cobra.Command, args []string) error {
			body, err := commandFile(cmd, file)
			if err != nil {
				return err
			}
			target := "/runs"
			if operation != "start" {
				target += "/" + url.PathEscape(args[0]) + "/" + operation
			}
			return send(cmd, "POST", target, body)
		}}
		command.Flags().StringVar(&file, "file", "", "JSON command file, or - for stdin")
		_ = command.MarkFlagRequired("file")
		root.AddCommand(command)
	}
	var publishFile string
	publish := &cobra.Command{Use: "publish <run-id> <watch-id>", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		body, err := commandFile(cmd, publishFile)
		if err != nil {
			return err
		}
		return send(cmd, "PUT", "/runs/"+url.PathEscape(args[0])+"/watches/"+url.PathEscape(args[1])+"/result", body)
	}}
	publish.Flags().StringVar(&publishFile, "file", "", "JSON Watch-result file, or - for stdin")
	_ = publish.MarkFlagRequired("file")
	root.AddCommand(publish)
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
