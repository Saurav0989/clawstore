package cmd

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newWriteCmd() *cobra.Command {
	var (
		entityID   string
		source     string
		confidence float64
	)
	cmd := &cobra.Command{
		Use:   "write <content>",
		Short: "Store an observation in memory",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			content := strings.TrimSpace(strings.Join(args, " "))
			resolvedEntity := strings.TrimSpace(entityID)
			if resolvedEntity != "" {
				resolvedEntity = strings.ToLower(strings.TrimSpace(resolvedEntity))
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			obs, err := deps.DB.AddObservationWithEmbedding(ctx, deps.Embedder, resolvedEntity, content, source, confidence)
			if err != nil {
				return fatalErr(cmd, err)
			}

			entityLabel := obs.EntityID
			if entityLabel == "" {
				entityLabel = "general"
			}
			writeStdout(cmd, "id=%d entity=%s created_at=%d\n", obs.ID, entityLabel, obs.CreatedAt)
			return nil
		},
	}

	cmd.Flags().StringVar(&entityID, "entity", "", "entity id (slug)")
	cmd.Flags().StringVar(&source, "source", "agent", "source label")
	cmd.Flags().Float64Var(&confidence, "confidence", 1.0, "confidence score [0.0-1.0]")
	return cmd
}
