package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

type request func(*cobra.Command, string, string, any) error

func configurationCommand(name, path string, send request) *cobra.Command {
	root := &cobra.Command{Use: name, Short: "Manage " + name + " configuration"}
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
	list.Flags().StringVar(&cursor, "cursor", "", "Continuation cursor from a previous page")
	list.Flags().IntVar(&limit, "limit", 50, "Page size (1–100)")
	list.Flags().StringVar(&state, "state", "", "active, paused, deprecated, or all (default: active)")
	list.Flags().BoolVar(&all, "all", false, "Include inactive records")
	root.AddCommand(list, &cobra.Command{Use: "get <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return send(cmd, "GET", path+"/"+url.PathEscape(args[0]), nil)
	}})
	for _, operation := range []string{"create", "update"} {
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
			method, target := "POST", path
			if operation == "update" {
				method, target = "PATCH", path+"/"+url.PathEscape(args[0])
			}
			return send(cmd, method, target, body)
		}}
		command.Flags().StringVar(&file, "file", "", "JSON command file, or - for stdin; request_id is optional")
		_ = command.MarkFlagRequired("file")
		root.AddCommand(command)
	}
	return root
}

func commandFile(cmd *cobra.Command, path string) (json.RawMessage, error) {
	var source io.Reader = cmd.InOrStdin()
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
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
