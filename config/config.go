package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultPort        = 7433
	defaultOllamaURL   = "http://localhost:11434"
	defaultOllamaModel = "nomic-embed-text"
)

type Config struct {
	Port        int    `json:"port"`
	MCPToken    string `json:"mcp_token"`
	OllamaURL   string `json:"ollama_url"`
	OllamaModel string `json:"ollama_model"`
	DataDir     string `json:"data_dir"`
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	ollamaURL := strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	if ollamaURL == "" {
		ollamaURL = defaultOllamaURL
	}
	dataDir := filepath.Join(home, ".clawstore")
	return Config{
		Port:        defaultPort,
		OllamaURL:   ollamaURL,
		OllamaModel: defaultOllamaModel,
		DataDir:     dataDir,
	}
}

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", errors.New("home directory is empty")
	}
	return filepath.Join(home, ".config", "clawstore", "config.json"), nil
}

func ConfigDir() (string, error) {
	path, err := ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

func Load() (Config, error) {
	cfg := DefaultConfig()
	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := Save(cfg); err != nil {
				return Config{}, err
			}
			return cfg, nil
		}
		return Config{}, err
	}
	if err := json.Unmarshal(buf, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	mergeDefaults(&cfg)
	return cfg, nil
}

func Save(cfg Config) error {
	mergeDefaults(&cfg)
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	return os.WriteFile(path, buf, 0o600)
}

func EnsureToken(cfg *Config) (bool, error) {
	if cfg == nil {
		return false, errors.New("nil config")
	}
	if strings.TrimSpace(cfg.MCPToken) != "" {
		return false, nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return false, err
	}
	cfg.MCPToken = "cs_" + hex.EncodeToString(raw)
	return true, nil
}

func mergeDefaults(cfg *Config) {
	if cfg.Port <= 0 {
		cfg.Port = defaultPort
	}
	if strings.TrimSpace(cfg.OllamaURL) == "" {
		cfg.OllamaURL = DefaultConfig().OllamaURL
	}
	if strings.TrimSpace(cfg.OllamaModel) == "" {
		cfg.OllamaModel = defaultOllamaModel
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = DefaultConfig().DataDir
	}
}
