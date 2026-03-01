package cmd

import (
	"github.com/spf13/cobra"
	"github.com/saurav0989/clawstore/config"
)

func newTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Print MCP authentication token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fatalErr(cmd, err)
			}
			if _, err := config.EnsureToken(&cfg); err != nil {
				return fatalErr(cmd, err)
			}
			if err := config.Save(cfg); err != nil {
				return fatalErr(cmd, err)
			}
			writeStdout(cmd, "%s\n", cfg.MCPToken)
			return nil
		},
	}
}
