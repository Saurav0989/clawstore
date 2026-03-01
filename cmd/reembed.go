package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newReembedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reembed",
		Short: "Re-embed observations missing vectors",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := newRuntimeDeps()
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer deps.Close()

			if !deps.DB.VecEnabled {
				return fatalErr(cmd, fmt.Errorf("vector search is not enabled in this build"))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			missing, err := deps.DB.MissingEmbeddings(ctx, 1_000_000)
			if err != nil {
				return fatalErr(cmd, err)
			}
			total := len(missing)
			if total == 0 {
				writeStdout(cmd, "No missing embeddings.\n")
				return nil
			}

			writeStdout(cmd, "re-embedding %d observations...\n", total)
			done := 0
			for _, obs := range missing {
				itemCtx, itemCancel := context.WithTimeout(cmd.Context(), 20*time.Second)
				err := deps.DB.ReembedObservation(itemCtx, deps.Embedder, obs.ID)
				itemCancel()
				if err == nil {
					done++
				}
				writeStdout(cmd, "\r[%s] %d/%d done", progressBar(done, total, 20), done, total)
			}
			writeStdout(cmd, "\n")
			writeStdout(cmd, "re-embedding complete: %d/%d done\n", done, total)
			return nil
		},
	}
}

func progressBar(done, total, width int) string {
	if total <= 0 {
		return strings.Repeat("-", width)
	}
	if width <= 0 {
		width = 20
	}
	filled := int(float64(done) / float64(total) * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("-", width-filled)
}
