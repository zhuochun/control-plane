package main

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func itemCommand(send request) *cobra.Command {
	root := &cobra.Command{Use: "item", Short: "Read and save items"}
	var view, kind, interestID, watchID, query, dedupeKey, cursor, delegationStatus, executor, externalRef string
	var limit int
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		values := url.Values{"limit": {fmt.Sprint(limit)}, "view": {view}}
		for key, value := range map[string]string{"kind": kind, "interest_id": interestID, "watch_id": watchID, "q": query, "dedupe_key": dedupeKey, "cursor": cursor, "delegation_status": delegationStatus, "executor": executor, "external_ref": externalRef} {
			if value != "" {
				values.Set(key, value)
			}
		}
		return send(cmd, "GET", "/items?"+values.Encode(), nil)
	}}
	list.Flags().StringVar(&view, "view", "all", "all, attention, todo, done, inbox, or archived")
	list.Flags().StringVar(&kind, "kind", "", "Filter by item kind")
	list.Flags().StringVar(&interestID, "interest", "", "Filter by Interest ID")
	list.Flags().StringVar(&watchID, "watch", "", "Filter by Watch ID")
	list.Flags().StringVar(&query, "query", "", "Literal title, summary, or context search")
	list.Flags().StringVar(&dedupeKey, "dedupe-key", "", "Match one exact deduplication key")
	list.Flags().StringVar(&cursor, "cursor", "", "Continuation cursor")
	list.Flags().StringVar(&delegationStatus, "delegation-status", "", "Comma-separated pending, blocked, or closed")
	list.Flags().StringVar(&executor, "executor", "", "Exact executing agent/system identity")
	list.Flags().StringVar(&externalRef, "external-ref", "", "Exact external continuation reference")
	list.Flags().IntVar(&limit, "limit", 50, "Page size (1–100)")
	root.AddCommand(list)
	root.AddCommand(&cobra.Command{Use: "get <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return send(cmd, "GET", "/items/"+url.PathEscape(args[0]), nil)
	}})
	for _, operation := range []string{"create", "update"} {
		operation := operation
		var file string
		use, args := operation, cobra.NoArgs
		if operation == "update" {
			use += " <id>"
			args = cobra.ExactArgs(1)
		}
		command := &cobra.Command{Use: use, Args: args, RunE: func(cmd *cobra.Command, args []string) error {
			body, err := commandFile(cmd, file)
			if err != nil {
				return err
			}
			method, target := "POST", "/items"
			if operation == "update" {
				method, target = "PUT", "/items/"+url.PathEscape(args[0])
			}
			return send(cmd, method, target, body)
		}}
		command.Flags().StringVar(&file, "file", "", "JSON command file, or - for stdin")
		_ = command.MarkFlagRequired("file")
		root.AddCommand(command)
	}
	var upsertFile string
	upsert := &cobra.Command{Use: "upsert", Args: cobra.NoArgs, Short: "Create or update an Interest-level Item", RunE: func(cmd *cobra.Command, args []string) error {
		body, err := commandFile(cmd, upsertFile)
		if err != nil {
			return err
		}
		return send(cmd, "POST", "/items/interest", body)
	}}
	upsert.Flags().StringVar(&upsertFile, "file", "", "JSON Item payload, or - for stdin")
	_ = upsert.MarkFlagRequired("file")
	root.AddCommand(upsert)
	var workFile string
	work := &cobra.Command{Use: "work <id>", Args: cobra.ExactArgs(1), Short: "Update existing Item work without a source Run", RunE: func(cmd *cobra.Command, args []string) error {
		body, err := commandFile(cmd, workFile)
		if err != nil {
			return err
		}
		return send(cmd, "PATCH", "/items/"+url.PathEscape(args[0])+"/work", body)
	}}
	work.Flags().StringVar(&workFile, "file", "", "JSON work update, or - for stdin")
	_ = work.MarkFlagRequired("file")
	root.AddCommand(work)
	var processFile string
	process := &cobra.Command{Use: "process-input <id>", Args: cobra.ExactArgs(1), Short: "Record captured user input handling, failure, or inbox archive", RunE: func(cmd *cobra.Command, args []string) error {
		body, err := commandFile(cmd, processFile)
		if err != nil {
			return err
		}
		return send(cmd, "POST", "/items/"+url.PathEscape(args[0])+"/inputs/process", body)
	}}
	process.Flags().StringVar(&processFile, "file", "", "JSON handling command, or - for stdin")
	_ = process.MarkFlagRequired("file")
	root.AddCommand(process)
	var inputOffset int
	var historyInputID string
	history := &cobra.Command{Use: "inputs <id>", Args: cobra.ExactArgs(1), Short: "Read original submissions and processing history", RunE: func(cmd *cobra.Command, args []string) error {
		target := "/items/" + url.PathEscape(args[0]) + "/inputs"
		if historyInputID != "" {
			target += "/" + url.PathEscape(historyInputID) + "/attempts"
		}
		return send(cmd, "GET", target+"?offset="+fmt.Sprint(inputOffset), nil)
	}}
	history.Flags().IntVar(&inputOffset, "offset", 0, "History offset returned as next_offset")
	history.Flags().StringVar(&historyInputID, "input", "", "Read paged attempts for this exact input ID")
	root.AddCommand(history)
	for _, operation := range []string{"action", "note"} {
		operation := operation
		var file string
		command := &cobra.Command{Use: operation + " <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			body, err := commandFile(cmd, file)
			if err != nil {
				return err
			}
			suffix, method := "/actions", "POST"
			if operation == "note" {
				suffix, method = "/note", "PUT"
			}
			return send(cmd, method, "/items/"+url.PathEscape(args[0])+suffix, body)
		}}
		command.Flags().StringVar(&file, "file", "", "JSON command file, or - for stdin")
		_ = command.MarkFlagRequired("file")
		root.AddCommand(command)
	}
	return root
}
