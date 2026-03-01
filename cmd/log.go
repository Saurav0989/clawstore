package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Manage append-only action log",
	}
	cmd.AddCommand(newLogAppendCmd())
	cmd.AddCommand(newLogTailCmd())
	return cmd
}

func newLogAppendCmd() *cobra.Command {
	var (
		actionType string
		summary    string
		detailRaw  string
		entities   string
		agent      string
	)
	cmd := &cobra.Command{
		Use:   "append",
		Short: "Append an entry to action log",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			if _, ok := validActionTypes[actionType]; !ok {
				return fatalErr(cmd, fmt.Errorf("invalid action type %q (allowed: tool_call|decision|error|cron_run)", actionType))
			}
			var detail any
			if strings.TrimSpace(detailRaw) != "" {
				if err := json.Unmarshal([]byte(detailRaw), &detail); err != nil {
					return fatalErr(cmd, err)
				}
			}
			entityIDs := splitCSV(entities)
			id, err := deps.DB.AppendActionLog(ctx, agent, actionType, summary, detail, entityIDs)
			if err != nil {
				return fatalErr(cmd, err)
			}
			writeStdout(cmd, "id=%d\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&actionType, "type", "", "action type: tool_call|decision|error|cron_run")
	cmd.Flags().StringVar(&summary, "summary", "", "short summary")
	cmd.Flags().StringVar(&detailRaw, "detail", "", "JSON object detail")
	cmd.Flags().StringVar(&entities, "entities", "", "comma-separated entity ids")
	cmd.Flags().StringVar(&agent, "agent", "openclaw", "agent name")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("summary")
	return cmd
}

var validActionTypes = map[string]struct{}{
	"tool_call": {},
	"decision":  {},
	"error":     {},
	"cron_run":  {},
}

func newLogTailCmd() *cobra.Command {
	var (
		limit int
		since string
		agent string
	)
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Tail action log entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			sinceTime, err := parseSince(since)
			if err != nil {
				return fatalErr(cmd, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			entries, err := deps.DB.TailActionLog(ctx, limit, sinceTime, agent)
			if err != nil {
				return fatalErr(cmd, err)
			}
			if len(entries) == 0 {
				writeStdout(cmd, "No log entries found.\n")
				return nil
			}
			for _, e := range entries {
				writeStdout(cmd, "- [%s] %s %s: %s", unixToLocal(e.CreatedAt), e.Agent, e.ActionType, e.Summary)
				if strings.TrimSpace(e.Entities) != "" {
					writeStdout(cmd, " | entities=%s", e.Entities)
				}
				writeStdout(cmd, "\n")
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "max entries")
	cmd.Flags().StringVar(&since, "since", "", "time window e.g. 1h")
	cmd.Flags().StringVar(&agent, "agent", "", "filter by agent")
	return cmd
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
