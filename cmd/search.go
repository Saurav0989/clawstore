package cmd

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/saurav0989/clawstore/store"
)

func newSearchCmd() *cobra.Command {
	var (
		mode  string
		limit int
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search memory by keyword or semantic meaning",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			query := strings.TrimSpace(strings.Join(args, " "))
			ctx, cancel := context.WithTimeout(cmd.Context(), 25*time.Second)
			defer cancel()

			results, err := deps.DB.Search(ctx, deps.Embedder, query, store.ParseSearchMode(mode), limit)
			if err != nil {
				return fatalErr(cmd, err)
			}
			_, _ = deps.DB.AppendActionLog(ctx, "clawstore", "tool_call", "search: "+query, map[string]any{
				"mode":  mode,
				"limit": limit,
			}, nil)
			if len(results) == 0 {
				writeStdout(cmd, "No results found.\n")
				return nil
			}
			for _, r := range results {
				entityLabel := r.EntityID
				if strings.TrimSpace(entityLabel) == "" {
					entityLabel = "general"
				}
				if strings.TrimSpace(r.EntityName) != "" {
					if strings.TrimSpace(r.EntityID) == "" {
						entityLabel = r.EntityName + " (general)"
					} else {
						entityLabel = r.EntityName + " (" + r.EntityID + ")"
					}
				}
				writeStdout(cmd, "- [%.3f] [%s] %s\n  %s\n", r.Score, unixToLocal(r.CreatedAt), entityLabel, r.Content)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "hybrid", "search mode: hybrid|semantic|fts")
	cmd.Flags().IntVar(&limit, "limit", 10, "max number of results")
	return cmd
}
