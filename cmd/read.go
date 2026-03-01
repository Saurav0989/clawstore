package cmd

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newReadCmd() *cobra.Command {
	var (
		limit int
		since string
	)
	cmd := &cobra.Command{
		Use:   "read <entity-id>",
		Short: "Read observations for an entity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			entityID := strings.ToLower(strings.TrimSpace(args[0]))
			sinceTime, err := parseSince(since)
			if err != nil {
				return fatalErr(cmd, err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()

			entity, err := deps.DB.GetEntity(ctx, entityID)
			if err != nil {
				return fatalErr(cmd, err)
			}
			obs, err := deps.DB.ReadObservationsByEntity(ctx, entityID, limit, sinceTime)
			if err != nil {
				return fatalErr(cmd, err)
			}

			writeStdout(cmd, "Entity: %s (%s)\n", entity.Name, entity.Type)
			if len(obs) == 0 {
				writeStdout(cmd, "No observations found.\n")
				return nil
			}
			for _, item := range obs {
				src := strings.TrimSpace(item.Source)
				if src == "" {
					src = "agent"
				}
				writeStdout(cmd, "- [%s] (%s) %s\n", unixToLocal(item.CreatedAt), src, item.Content)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "max number of observations")
	cmd.Flags().StringVar(&since, "since", "", "time window: e.g. 7d, 30d, 1h")
	return cmd
}
