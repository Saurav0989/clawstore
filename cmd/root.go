package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/saurav0989/clawstore/config"
	"github.com/saurav0989/clawstore/embed"
	"github.com/saurav0989/clawstore/store"
	"go.uber.org/zap"
)

var (
	rootCmd = &cobra.Command{
		Use:   "clawstore",
		Short: "Long-term memory engine for OpenClaw",
	}
	overrideDBPath string
)

type runtimeDeps struct {
	Paths    store.Paths
	Config   config.Config
	DB       *store.DB
	Embedder embed.Embedder
	Logger   *zap.Logger
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&overrideDBPath, "db", "", "override sqlite db path")
	rootCmd.AddCommand(newWriteCmd())
	rootCmd.AddCommand(newReadCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newEntityCmd())
	rootCmd.AddCommand(newLogCmd())
	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newTokenCmd())
	rootCmd.AddCommand(newReembedCmd())
}

func newRuntimeDeps() (*runtimeDeps, error) {
	return newRuntimeDepsWithLogger(zap.NewNop())
}

func newRuntimeDepsWithLogger(logger *zap.Logger) (*runtimeDeps, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	paths, err := resolvePathsFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	db, err := store.Open(paths.DBPath, paths.DataDir, paths.ConfigDir, logger)
	if err != nil {
		return nil, err
	}

	emb := newRuntimeEmbedder(cfg)
	return &runtimeDeps{
		Paths:    paths,
		Config:   cfg,
		DB:       db,
		Embedder: emb,
		Logger:   logger,
	}, nil
}

func (r *runtimeDeps) Close() {
	if r == nil {
		return
	}
	if r.DB != nil {
		_ = r.DB.Close()
	}
	if r.Logger != nil {
		_ = r.Logger.Sync()
	}
}

func resolvePathsFromConfig(cfg config.Config) (store.Paths, error) {
	paths, err := store.ResolvePaths()
	if err != nil {
		return paths, err
	}
	dataDir := cfg.DataDir
	if overrideDBPath != "" {
		dataDir = filepath.Dir(overrideDBPath)
	}
	if dataDir == "" {
		dataDir = paths.DataDir
	}
	paths.DataDir = dataDir
	if overrideDBPath != "" {
		paths.DBPath = overrideDBPath
	} else {
		paths.DBPath = filepath.Join(paths.DataDir, "store.db")
	}
	return paths, nil
}

func newRuntimeEmbedder(cfg config.Config) embed.Embedder {
	return embed.NewOllamaEmbedder(cfg.OllamaURL, cfg.OllamaModel, 10*time.Second)
}

func fatalErr(cmd *cobra.Command, err error) error {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
	return err
}

func writeStdout(cmd *cobra.Command, msg string, args ...any) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), msg, args...)
}

func writeStderr(cmd *cobra.Command, msg string, args ...any) {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), msg, args...)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
