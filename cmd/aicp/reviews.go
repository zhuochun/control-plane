package main

import (
	"net/url"

	"github.com/spf13/cobra"
)

func reviewCommand(send request) *cobra.Command {
	root := &cobra.Command{Use: "review", Short: "Discover review capabilities, register formats, and upload evidence"}
	root.AddCommand(&cobra.Command{Use: "capabilities", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return send(cmd, "GET", "/review-capabilities", nil)
	}})
	root.AddCommand(&cobra.Command{Use: "formats", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return send(cmd, "GET", "/review-formats", nil)
	}})
	root.AddCommand(&cobra.Command{Use: "format <id> <version>", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		return send(cmd, "GET", "/review-formats/"+url.PathEscape(args[0])+"/"+url.PathEscape(args[1]), nil)
	}})
	for _, operation := range []struct{ name, path string }{{"register", "/review-formats"}, {"upload", "/review-artifacts"}} {
		var file string
		command := &cobra.Command{Use: operation.name, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			body, err := commandFile(cmd, file)
			if err != nil {
				return err
			}
			return send(cmd, "POST", operation.path, body)
		}}
		command.Flags().StringVar(&file, "file", "", "JSON command file, or - for stdin")
		_ = command.MarkFlagRequired("file")
		root.AddCommand(command)
	}
	return root
}
