package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/saurav0989/clawstore/config"
	"github.com/saurav0989/clawstore/embed"
	"github.com/saurav0989/clawstore/store"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show clawstore daemon and database health",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fatalErr(cmd, err)
			}
			paths, err := resolvePathsFromConfig(cfg)
			if err != nil {
				return fatalErr(cmd, err)
			}

			db, err := store.Open(paths.DBPath, paths.DataDir, paths.ConfigDir, nil)
			if err != nil {
				return fatalErr(cmd, err)
			}
			defer db.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Second)
			defer cancel()
			stats, err := db.Stats(ctx)
			if err != nil {
				return fatalErr(cmd, err)
			}

			running, pid, uptime := daemonState(filepath.Join(paths.DataDir, "daemon.pid"))
			dataSize := fileSize(paths.DBPath)
			authEnabled := strings.TrimSpace(cfg.MCPToken) != ""

			ollama := embed.NewOllamaEmbedder(cfg.OllamaURL, cfg.OllamaModel, 5*time.Second)
			ollamaConnected, latency := checkOllama(ctx, ollama)
			modelPulled := false
			if ollamaConnected {
				modelPulled = ollamaModelPulled(ctx, cfg.OllamaURL, cfg.OllamaModel)
			}

			lastWriteLine := "never"
			if lastObs, err := db.LastObservation(ctx); err == nil {
				entity := lastObs.EntityID
				if strings.TrimSpace(entity) == "" {
					entity = "general"
				}
				lastWriteLine = fmt.Sprintf("%s (%q -> entity: %s)", ago(lastObs.CreatedAt), truncate(lastObs.Content, 80), entity)
			}

			lastSearchLine := "never"
			if lastSearch, err := db.LastSearchLog(ctx); err == nil {
				query := strings.TrimSpace(strings.TrimPrefix(lastSearch.Summary, "search:"))
				lastSearchLine = fmt.Sprintf("%s (query: %q)", ago(lastSearch.CreatedAt), query)
			}

			writeStdout(cmd, "daemon:          ")
			if running {
				writeStdout(cmd, "running (PID %d, uptime %s)\n", pid, uptime.Round(time.Minute))
			} else {
				writeStdout(cmd, "not running\n")
			}
			writeStdout(cmd, "port:            %d\n", cfg.Port)
			writeStdout(cmd, "auth:            %s\n", onOff(authEnabled))
			writeStdout(cmd, "vector_search:   %s\n", vectorMode(db.VecEnabled))
			writeStdout(cmd, "data:            %s (%s)\n", paths.DBPath, formatBytes(dataSize))
			writeStdout(cmd, "\n")

			writeStdout(cmd, "store:\n")
			writeStdout(cmd, "  entities:      %d\n", stats.EntityCount)
			writeStdout(cmd, "  observations:  %d\n", stats.ObservationCount)
			writeStdout(cmd, "  vectors:       %d (%d missing embeddings)\n", stats.VectorCount, stats.MissingVectors)
			writeStdout(cmd, "  action_log:    %d entries\n", stats.ActionLogCount)
			writeStdout(cmd, "\n")

			writeStdout(cmd, "ollama:\n")
			if ollamaConnected {
				writeStdout(cmd, "  status:        connected (%s)\n", cfg.OllamaURL)
			} else {
				writeStdout(cmd, "  status:        disconnected (%s)\n", cfg.OllamaURL)
			}
			if modelPulled {
				writeStdout(cmd, "  model:         %s ✓ pulled\n", cfg.OllamaModel)
			} else {
				writeStdout(cmd, "  model:         %s (not found)\n", cfg.OllamaModel)
			}
			if latency > 0 {
				writeStdout(cmd, "  latency:       %s\n", latency)
			}
			writeStdout(cmd, "\n")
			writeStdout(cmd, "last write:      %s\n", lastWriteLine)
			writeStdout(cmd, "last search:     %s\n", lastSearchLine)

			return nil
		},
	}
}

func onOff(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func vectorMode(enabled bool) string {
	if enabled {
		return "enabled (vec0)"
	}
	return "fallback (fts only)"
}

func daemonState(pidPath string) (bool, int, time.Duration) {
	buf, err := os.ReadFile(pidPath)
	if err != nil {
		return false, 0, 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf)))
	if err != nil || pid <= 0 || !processAlive(pid) {
		return false, 0, 0
	}
	st, err := os.Stat(pidPath)
	if err != nil {
		return true, pid, 0
	}
	return true, pid, time.Since(st.ModTime())
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

func checkOllama(ctx context.Context, e *embed.OllamaEmbedder) (bool, time.Duration) {
	start := time.Now()
	if err := e.HealthCheck(ctx); err != nil {
		return false, 0
	}
	return true, time.Since(start).Round(time.Millisecond)
}

func ollamaModelPulled(ctx context.Context, baseURL, model string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return false
	}
	buf, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return false
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(buf, &payload); err != nil {
		return false
	}
	for _, item := range payload.Models {
		name := strings.TrimSpace(item.Name)
		if name == model || strings.HasPrefix(name, model+":") {
			return true
		}
	}
	return false
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	kb := float64(n) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%.1f KB", kb)
	}
	mb := kb / 1024
	if mb < 1024 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	gb := mb / 1024
	return fmt.Sprintf("%.2f GB", gb)
}

func ago(ts int64) string {
	if ts <= 0 {
		return "never"
	}
	d := time.Since(time.Unix(ts, 0))
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

func truncate(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}
