package main

import (
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

func proposalCommand(send request) *cobra.Command {
	root := &cobra.Command{Use: "proposal", Short: "Review agent configuration proposals"}
	var state, cursor string
	var limit int
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		values := url.Values{"limit": {fmt.Sprint(limit)}}
		if state != "" {
			values.Set("state", state)
		}
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		return send(cmd, "GET", "/proposals?"+values.Encode(), nil)
	}}
	list.Flags().StringVar(&state, "state", "", "Filter by pending, accepted, or rejected")
	list.Flags().StringVar(&cursor, "cursor", "", "Continuation cursor")
	list.Flags().IntVar(&limit, "limit", 50, "Page size (1–100)")
	root.AddCommand(list)
	var createFile string
	create := &cobra.Command{Use: "create", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		body, err := commandFile(cmd, createFile)
		if err != nil {
			return err
		}
		return send(cmd, "POST", "/proposals", body)
	}}
	create.Flags().StringVar(&createFile, "file", "", "JSON proposal file, or - for stdin")
	_ = create.MarkFlagRequired("file")
	root.AddCommand(create)
	var resolveFile,resolution,snoozeUntil,mergeInto,requestID string
	resolve := &cobra.Command{Use: "resolve <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var body any
		if resolveFile!=""{
			data,err:=commandFile(cmd,resolveFile);if err!=nil{return err};body=data
		}else{
			if resolution==""{return fmt.Errorf("--resolution is required")}
			payload:=map[string]any{"resolution":resolution,"request_id":requestID}
			if snoozeUntil!=""{value,err:=time.Parse(time.RFC3339,snoozeUntil);if err!=nil{return err};payload["snooze_until"]=value}
			if mergeInto!=""{payload["merge_into"]=mergeInto}
			body=payload
		}
		return send(cmd, "POST", "/proposals/"+url.PathEscape(args[0])+"/resolve", body)
	}}
	resolve.Flags().StringVar(&resolveFile, "file", "", "JSON resolution file, or - for stdin")
	resolve.Flags().StringVar(&resolution,"resolution","","accepted, rejected, snoozed, or merged")
	resolve.Flags().StringVar(&snoozeUntil,"snooze-until","","RFC3339 time for snoozed proposals")
	resolve.Flags().StringVar(&mergeInto,"merge-into","","Proposal ID to merge into")
	resolve.Flags().StringVar(&requestID,"request-id","","Optional idempotency key")
	root.AddCommand(resolve)
	return root
}
