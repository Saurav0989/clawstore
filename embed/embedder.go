package embed

import "context"

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	HealthCheck(ctx context.Context) error
	ModelName() string
}
