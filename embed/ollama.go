package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxPromptChars = 8192

type OllamaEmbedder struct {
	BaseURL string
	Model   string
	Timeout time.Duration
	client  *http.Client
}

type embeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embeddingResponse struct {
	Embedding  []float64   `json:"embedding"`
	Embeddings [][]float64 `json:"embeddings"`
	Error      string      `json:"error"`
}

func NewOllamaEmbedder(baseURL, model string, timeout time.Duration) *OllamaEmbedder {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://localhost:11434"
	}
	if strings.TrimSpace(model) == "" {
		model = "nomic-embed-text"
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &OllamaEmbedder{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

func (e *OllamaEmbedder) ModelName() string {
	return e.Model
}

func (e *OllamaEmbedder) HealthCheck(ctx context.Context) error {
	u, err := url.Parse(e.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid ollama base url: %w", err)
	}
	u.Path = "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ollama health check failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	payload := embeddingRequest{
		Model:  e.Model,
		Prompt: truncateRunes(text, maxPromptChars),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	u, err := url.Parse(e.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid ollama base url: %w", err)
	}
	u.Path = "/api/embeddings"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama embedding error: %s %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var out embeddingResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("ollama: %s", out.Error)
	}

	vec64 := out.Embedding
	if len(vec64) == 0 && len(out.Embeddings) > 0 {
		vec64 = out.Embeddings[0]
	}
	if len(vec64) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding")
	}

	vec32 := make([]float32, len(vec64))
	for i, v := range vec64 {
		vec32[i] = float32(v)
	}
	return vec32, nil
}

func truncateRunes(s string, n int) string {
	if n <= 0 || len(s) == 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
