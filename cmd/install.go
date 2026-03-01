package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/saurav0989/clawstore/config"
	"github.com/saurav0989/clawstore/launchd"
)

func newInstallCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install and load launchd daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fatalErr(cmd, err)
			}
			if port > 0 {
				cfg.Port = port
			}
			createdToken, err := config.EnsureToken(&cfg)
			if err != nil {
				return fatalErr(cmd, err)
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

			home := paths.Home
			binaryPath := "/usr/local/bin/clawstore"
			if !exists(binaryPath) {
				exe, err := os.Executable()
				if err == nil {
					if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil {
						exe = resolved
					}
					binaryPath = exe
				}
			}

			plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.saurav0989.clawstore.plist")
			if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
				return fatalErr(cmd, err)
			}
			content := launchd.RenderPlist(home, binaryPath, cfg.Port, cfg.OllamaURL)
			if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
				return fatalErr(cmd, err)
			}

			_ = exec.Command("launchctl", "unload", plistPath).Run()
			loadOut, err := exec.Command("launchctl", "load", plistPath).CombinedOutput()
			if err != nil {
				return fatalErr(cmd, fmt.Errorf("launchctl load failed: %w (%s)", err, string(loadOut)))
			}

			writeStdout(cmd, "Installed launchd plist at %s\n", plistPath)
			writeStdout(cmd, "Loaded service com.saurav0989.clawstore on port %d\n", cfg.Port)
			if createdToken {
				writeStdout(cmd, "Generated MCP token.\n")
			}
			writeStdout(cmd, "Add this to your OpenClaw MCP config:\n")
			writeStdout(cmd, "{\n")
			writeStdout(cmd, "  \"clawstore\": {\n")
			writeStdout(cmd, "    \"url\": \"http://localhost:%d/mcp\",\n", cfg.Port)
			writeStdout(cmd, "    \"headers\": { \"Authorization\": \"Bearer %s\" }\n", cfg.MCPToken)
			writeStdout(cmd, "  }\n")
			writeStdout(cmd, "}\n")
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "override daemon port in config")
	return cmd
}
