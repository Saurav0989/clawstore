package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/saurav0989/clawstore/config"
	mcpserver "github.com/saurav0989/clawstore/mcp"
	"github.com/saurav0989/clawstore/store"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func newServeCmd() *cobra.Command {
	var (
		port   int
		noAuth bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start clawstore MCP daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fatalErr(cmd, err)
			}
			if port > 0 {
				cfg.Port = port
			}
			if !noAuth {
				if _, err := config.EnsureToken(&cfg); err != nil {
					return fatalErr(cmd, err)
				}
			}
			if err := config.Save(cfg); err != nil {
				return fatalErr(cmd, err)
			}

			paths, err := resolvePathsFromConfig(cfg)
			if err != nil {
				return fatalErr(cmd, err)
			}
			if err := os.MkdirAll(paths.DataDir, 0o755); err != nil {
				return fatalErr(cmd, err)
			}

			logger, err := newDaemonLogger(paths.DataDir)
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer logger.Sync()

			db, err := store.Open(paths.DBPath, paths.DataDir, paths.ConfigDir, logger)
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer db.Close()

			pidPath := filepath.Join(paths.DataDir, "daemon.pid")
			releasePID, err := acquirePIDFile(pidPath)
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer releasePID()

			if noAuth {
				writeStderr(cmd, "WARNING: MCP auth is disabled (--no-auth). Do not use this mode in production.\n")
				logger.Warn("mcp auth disabled via --no-auth")
			}

			mcpSrv := mcpserver.New(mcpserver.ToolDeps{DB: db, Embedder: newRuntimeEmbedder(cfg), Logger: logger}, cfg.MCPToken, noAuth)
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port))
			httpSrv := &http.Server{
				Addr:              addr,
				Handler:           mcpSrv.Handler(),
				ReadHeaderTimeout: 5 * time.Second,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      60 * time.Second,
				IdleTimeout:       120 * time.Second,
				MaxHeaderBytes:    1 << 20,
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
			defer stop()

			errCh := make(chan error, 1)
			go func() {
				logger.Info("starting clawstore daemon", zap.String("addr", addr))
				err := httpSrv.ListenAndServe()
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- err
					return
				}
				errCh <- nil
			}()

			writeStdout(cmd, "MCP server listening on http://localhost:%d/mcp\n", cfg.Port)
			select {
			case err := <-errCh:
				if err != nil {
					logger.Error("daemon error", zap.Error(err))
					return fatalErr(cmd, err)
				}
				return nil
			case <-ctx.Done():
				logger.Info("shutting down clawstore daemon")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := httpSrv.Shutdown(shutdownCtx); err != nil {
					logger.Error("shutdown failed", zap.Error(err))
					return fatalErr(cmd, err)
				}
				logger.Info("shutdown complete")
				return nil
			}
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "MCP port (default: config value or 7433)")
	cmd.Flags().BoolVar(&noAuth, "no-auth", false, "disable MCP auth (development only)")
	return cmd
}

func acquirePIDFile(path string) (func(), error) {
	if buf, err := os.ReadFile(path); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(buf)))
		if pid > 0 && processAlive(pid) {
			return nil, fmt.Errorf("clawstore daemon already running (PID %d). Run 'clawstore status' to check", pid)
		}
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	pid := os.Getpid()
	if _, err := fmt.Fprintf(f, "%d\n", pid); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return func() {
		_ = os.Remove(path)
	}, nil
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func newDaemonLogger(dataDir string) (*zap.Logger, error) {
	logWriter := &lumberjack.Logger{
		Filename:   filepath.Join(dataDir, "daemon.log"),
		MaxSize:    10,
		MaxBackups: 3,
		Compress:   true,
	}
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(logWriter),
		zap.InfoLevel,
	)
	return zap.New(core), nil
}
