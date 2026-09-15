package main

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func itemCommand(send request) *cobra.Command {
	root := &cobra.Command{Use: "item", Short: "Read and publish items"}
	var view, kind, interestID, watchID, query, dedupeKey, cursor string
	var limit int
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		values := url.Values{"limit": {fmt.Sprint(limit)}, "view": {view}}
		for key, value := range map[string]string{"kind": kind, "interest_id": interestID, "watch_id": watchID, "q": query, "dedupe_key": dedupeKey, "cursor": cursor} {
			if value != "" {
				values.Set(key, value)
			}
		}
		return send(cmd, "GET", "/items?"+values.Encode(), nil)
	}}
	list.Flags().StringVar(&view, "view", "all", "all, attention, todo, or done")
	list.Flags().StringVar(&kind, "kind", "", "Filter by item kind")
	list.Flags().StringVar(&interestID, "interest", "", "Filter by Interest ID")
	list.Flags().StringVar(&watchID, "watch", "", "Filter by Watch ID")
	list.Flags().StringVar(&query, "query", "", "Literal title, summary, or context search")
	list.Flags().StringVar(&dedupeKey, "dedupe-key", "", "Match one exact deduplication key")
	list.Flags().StringVar(&cursor, "cursor", "", "Continuation cursor")
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
