package store

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/saurav0989/clawstore/embed"
	"go.uber.org/zap"
)

type SearchMode string

const (
	SearchModeHybrid   SearchMode = "hybrid"
	SearchModeSemantic SearchMode = "semantic"
	SearchModeFTS      SearchMode = "fts"
)

func ParseSearchMode(mode string) SearchMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "semantic":
		return SearchModeSemantic
	case "fts":
		return SearchModeFTS
	default:
		return SearchModeHybrid
	}
}

func (d *DB) AddObservationWithEmbedding(
	ctx context.Context,
	emb embed.Embedder,
	entityID, content, source string,
	confidence float64,
) (Observation, error) {
	obs, err := d.AddObservation(ctx, entityID, content, source, confidence)
	if err != nil {
		return Observation{}, err
	}

	if emb == nil || !d.VecEnabled {
		return obs, nil
	}
	vector, err := emb.Embed(ctx, content)
	if err != nil {
		d.Logger.Warn("embedding failed, observation stored without vector", zap.Error(err))
		return obs, nil
	}
	if err := d.UpsertVector(ctx, obs.ID, vector); err != nil {
		d.Logger.Warn("vector insert failed, observation stored without vector", zap.Error(err))
	}
	return obs, nil
}

func (d *DB) UpsertVector(ctx context.Context, observationID int64, vector []float32) error {
	if !d.VecEnabled {
		return errors.New("vector table is unavailable")
	}
	blob, err := serializeVector(vector)
	if err != nil {
		return err
	}
	_, err = d.SQL.ExecContext(ctx, `
		INSERT OR REPLACE INTO observation_vectors(observation_id, embedding)
		VALUES(?, vec_f32(?))
	`, observationID, blob)
	if err != nil {
		return err
	}
	return nil
}

func (d *DB) Search(ctx context.Context, emb embed.Embedder, query string, mode SearchMode, limit int) ([]Observation, error) {
	if limit <= 0 {
		limit = 10
	}
	switch mode {
	case SearchModeFTS:
		return d.searchFTSScored(ctx, query, limit)
	case SearchModeSemantic:
		return d.SearchSemantic(ctx, emb, query, limit)
	default:
		return d.SearchHybrid(ctx, emb, query, limit)
	}
}

func (d *DB) SearchSemantic(ctx context.Context, emb embed.Embedder, query string, limit int) ([]Observation, error) {
	if limit <= 0 {
		limit = 10
	}
	if emb == nil {
		return nil, errors.New("embedder unavailable")
	}
	if !d.VecEnabled {
		return nil, errors.New("vector search unavailable")
	}

	queryVector, err := emb.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	queryBlob, err := serializeVector(queryVector)
	if err != nil {
		return nil, err
	}

	rows, err := d.SQL.QueryContext(ctx, `
		SELECT observation_id, distance
		FROM observation_vectors
		WHERE embedding MATCH vec_f32(?)
		ORDER BY distance
		LIMIT ?
	`, queryBlob, limit)
	if err != nil {
		return nil, fmt.Errorf("semantic KNN query failed: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		ID       int64
		Distance float64
	}
	candidates := make([]candidate, 0, limit)
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.ID, &c.Distance); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	results := make([]Observation, 0, len(candidates))
	for _, c := range candidates {
		obs, err := d.GetObservation(ctx, c.ID)
		if err != nil {
			continue
		}
		obs.Score = c.Distance
		results = append(results, obs)
	}
	return normalizeSemanticDistances(results), nil
}

func (d *DB) SearchHybrid(ctx context.Context, emb embed.Embedder, query string, limit int) ([]Observation, error) {
	if limit <= 0 {
		limit = 10
	}
	ftsResults, ftsErr := d.searchFTSScored(ctx, query, limit*2)
	semanticResults, semErr := d.SearchSemantic(ctx, emb, query, limit*2)

	if semErr != nil {
		d.Logger.Warn("semantic search unavailable, falling back to fts", zap.Error(semErr))
	}
	if ftsErr != nil {
		return nil, ftsErr
	}
	if len(semanticResults) == 0 {
		if len(ftsResults) > limit {
			return ftsResults[:limit], nil
		}
		return ftsResults, nil
	}

	byID := map[int64]Observation{}
	vecScore := map[int64]float64{}
	ftsScore := map[int64]float64{}

	for _, item := range semanticResults {
		byID[item.ID] = item
		vecScore[item.ID] = item.Score
	}
	for _, item := range ftsResults {
		if _, exists := byID[item.ID]; !exists {
			byID[item.ID] = item
		}
		ftsScore[item.ID] = item.Score
	}

	score := map[int64]float64{}
	ids := make([]int64, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
		// Missing side defaults to 0.0
		score[id] = 0.6*vecScore[id] + 0.4*ftsScore[id]
	}
	sort.Slice(ids, func(i, j int) bool {
		return score[ids[i]] > score[ids[j]]
	})

	out := make([]Observation, 0, min(limit, len(ids)))
	for _, id := range ids {
		item := byID[id]
		item.Score = score[id]
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (d *DB) searchFTSScored(ctx context.Context, query string, limit int) ([]Observation, error) {
	results, err := d.SearchFTS(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return results, nil
	}

	minRank, maxRank := results[0].Score, results[0].Score
	for _, r := range results {
		minRank = math.Min(minRank, r.Score)
		maxRank = math.Max(maxRank, r.Score)
	}
	for i := range results {
		results[i].Score = normalizeFTSRank(results[i].Score, minRank, maxRank)
	}
	return results, nil
}

func normalizeSemanticDistances(results []Observation) []Observation {
	if len(results) == 0 {
		return results
	}
	maxDistance := results[0].Score
	for _, r := range results {
		maxDistance = math.Max(maxDistance, r.Score)
	}
	for i := range results {
		results[i].Score = normalizeVecDistance(results[i].Score, maxDistance)
	}
	return results
}

func normalizeFTSRank(rank, minRank, maxRank float64) float64 {
	if maxRank == minRank {
		return 1.0
	}
	score := (maxRank - rank) / (maxRank - minRank)
	return clamp01(score)
}

func normalizeVecDistance(distance, maxDistance float64) float64 {
	if maxDistance <= 0 {
		return 1.0
	}
	score := 1.0 - (distance / maxDistance)
	return clamp01(score)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func (d *DB) MissingEmbeddings(ctx context.Context, limit int) ([]Observation, error) {
	if !d.VecEnabled {
		return nil, errors.New("vector table unavailable")
	}
	if limit <= 0 {
		limit = 1000
	}
	rows, err := d.SQL.QueryContext(ctx, `
		SELECT o.id, COALESCE(o.entity_id, ''), COALESCE(e.name, ''), o.content, o.source, o.confidence, o.created_at
		FROM observations o
		LEFT JOIN observation_vectors v ON v.observation_id = o.id
		LEFT JOIN entities e ON e.id = o.entity_id
		WHERE v.observation_id IS NULL
		ORDER BY o.created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObservations(rows)
}

func (d *DB) ReembedObservation(ctx context.Context, emb embed.Embedder, obsID int64) error {
	if emb == nil {
		return errors.New("embedder unavailable")
	}
	if !d.VecEnabled {
		return errors.New("vector table unavailable")
	}
	var content string
	if err := d.SQL.QueryRowContext(ctx, `SELECT content FROM observations WHERE id = ?`, obsID).Scan(&content); err != nil {
		return err
	}
	vector, err := emb.Embed(ctx, content)
	if err != nil {
		return err
	}
	blob, err := serializeVector(vector)
	if err != nil {
		return err
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO observation_vectors(observation_id, embedding)
		VALUES(?, vec_f32(?))
	`, obsID, blob); err != nil {
		return err
	}
	return tx.Commit()
}

func serializeVector(vector []float32) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, vector); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
