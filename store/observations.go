package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const MaxObservationLength = 10000

type Observation struct {
	ID         int64   `json:"id"`
	EntityID   string  `json:"entity_id"`
	EntityName string  `json:"entity_name,omitempty"`
	Content    string  `json:"content"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
	CreatedAt  int64   `json:"created_at"`
	Score      float64 `json:"score,omitempty"`
}

func (d *DB) AddObservation(ctx context.Context, entityID, content, source string, confidence float64) (Observation, error) {
	entityID = strings.TrimSpace(entityID)
	content = strings.TrimSpace(content)
	if content == "" {
		return Observation{}, fmt.Errorf("content cannot be empty")
	}
	if len([]rune(content)) > MaxObservationLength {
		return Observation{}, fmt.Errorf("observation too long: %d chars, max is %d", len([]rune(content)), MaxObservationLength)
	}
	if source == "" {
		source = "agent"
	}
	if confidence <= 0 {
		confidence = 1.0
	}

	if entityID != "" {
		entityID = strings.ToLower(strings.TrimSpace(entityID))
		if err := validateEntityID(entityID); err != nil {
			return Observation{}, err
		}
		if _, err := d.EnsureEntity(ctx, entityID); err != nil {
			return Observation{}, err
		}
	}

	now := UnixNow()
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return Observation{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO observations(entity_id, content, source, confidence, created_at) VALUES(?, ?, ?, ?, ?)`,
		nullableEntityID(entityID), content, source, confidence, now,
	)
	if err != nil {
		return Observation{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Observation{}, err
	}
	if entityID != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE entities SET updated_at = ? WHERE id = ?`, now, entityID); err != nil {
			return Observation{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Observation{}, err
	}
	return d.GetObservation(ctx, id)
}

func (d *DB) GetObservation(ctx context.Context, id int64) (Observation, error) {
	var out Observation
	err := d.SQL.QueryRowContext(ctx, `
		SELECT o.id, COALESCE(o.entity_id, ''), COALESCE(e.name, ''), o.content, o.source, o.confidence, o.created_at
		FROM observations o
		LEFT JOIN entities e ON e.id = o.entity_id
		WHERE o.id = ?
	`, id).Scan(&out.ID, &out.EntityID, &out.EntityName, &out.Content, &out.Source, &out.Confidence, &out.CreatedAt)
	if err != nil {
		return Observation{}, err
	}
	return out, nil
}

func (d *DB) ReadObservationsByEntity(ctx context.Context, entityID string, limit int, since *time.Time) ([]Observation, error) {
	if limit <= 0 {
		limit = 20
	}
	entityID = strings.ToLower(strings.TrimSpace(entityID))
	if err := validateEntityID(entityID); err != nil {
		return nil, err
	}
	if since != nil {
		rows, err := d.SQL.QueryContext(ctx, `
			SELECT o.id, COALESCE(o.entity_id, ''), COALESCE(e.name, ''), o.content, o.source, o.confidence, o.created_at
			FROM observations o
			LEFT JOIN entities e ON e.id = o.entity_id
			WHERE o.entity_id = ? AND o.created_at >= ?
			ORDER BY o.created_at DESC
			LIMIT ?
		`, entityID, since.Unix(), limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanObservations(rows)
	}

	rows, err := d.SQL.QueryContext(ctx, `
		SELECT o.id, COALESCE(o.entity_id, ''), COALESCE(e.name, ''), o.content, o.source, o.confidence, o.created_at
		FROM observations o
		LEFT JOIN entities e ON e.id = o.entity_id
		WHERE o.entity_id = ?
		ORDER BY o.created_at DESC
		LIMIT ?
	`, entityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObservations(rows)
}

func (d *DB) RecentObservations(ctx context.Context, limit int, since *time.Time) ([]Observation, error) {
	if limit <= 0 {
		limit = 20
	}
	if since != nil {
		rows, err := d.SQL.QueryContext(ctx, `
			SELECT o.id, COALESCE(o.entity_id, ''), COALESCE(e.name, ''), o.content, o.source, o.confidence, o.created_at
			FROM observations o
			LEFT JOIN entities e ON e.id = o.entity_id
			WHERE o.created_at >= ?
			ORDER BY o.created_at DESC
			LIMIT ?
		`, since.Unix(), limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanObservations(rows)
	}
	rows, err := d.SQL.QueryContext(ctx, `
		SELECT o.id, COALESCE(o.entity_id, ''), COALESCE(e.name, ''), o.content, o.source, o.confidence, o.created_at
		FROM observations o
		LEFT JOIN entities e ON e.id = o.entity_id
		ORDER BY o.created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObservations(rows)
}

func (d *DB) SearchFTS(ctx context.Context, query string, limit int) ([]Observation, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	ftsQuery := buildFTSQuery(query)
	if limit <= 0 {
		limit = 10
	}
	rows, err := d.SQL.QueryContext(ctx, `
		SELECT o.id, COALESCE(o.entity_id, ''), COALESCE(e.name, ''), o.content, o.source, o.confidence, o.created_at,
		       bm25(observations_fts) AS rank
		FROM observations_fts
		JOIN observations o ON o.id = observations_fts.rowid
		LEFT JOIN entities e ON e.id = o.entity_id
		WHERE observations_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Observation
	for rows.Next() {
		var o Observation
		var rank float64
		if err := rows.Scan(&o.ID, &o.EntityID, &o.EntityName, &o.Content, &o.Source, &o.Confidence, &o.CreatedAt, &rank); err != nil {
			return nil, err
		}
		o.Score = rank
		results = append(results, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return d.searchKeywordFallback(ctx, query, limit)
	}
	return results, nil
}

func buildFTSQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return query
	}
	lower := strings.ToLower(query)
	if strings.Contains(lower, " or ") || strings.Contains(lower, " and ") || strings.ContainsAny(lower, "\"*()") {
		return query
	}
	terms := strings.Fields(query)
	if len(terms) <= 1 {
		return query
	}
	return strings.Join(terms, " OR ")
}

func (d *DB) searchKeywordFallback(ctx context.Context, query string, limit int) ([]Observation, error) {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(terms) == 0 {
		return nil, nil
	}
	clauses := make([]string, 0, len(terms))
	args := make([]any, 0, len(terms)+1)
	for _, term := range terms {
		root := termRoot(term)
		if root == "" {
			continue
		}
		clauses = append(clauses, "lower(o.content) LIKE ?")
		args = append(args, "%"+root+"%")
	}
	if len(clauses) == 0 {
		return nil, nil
	}
	args = append(args, limit)
	q := fmt.Sprintf(`
		SELECT o.id, COALESCE(o.entity_id, ''), COALESCE(e.name, ''), o.content, o.source, o.confidence, o.created_at
		FROM observations o
		LEFT JOIN entities e ON e.id = o.entity_id
		WHERE %s
		ORDER BY o.created_at DESC
		LIMIT ?
	`, strings.Join(clauses, " OR "))
	rows, err := d.SQL.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanObservations(rows)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Score = 0
	}
	return out, nil
}

func termRoot(term string) string {
	term = strings.TrimSpace(term)
	term = strings.Trim(term, `"'.,:;!?()[]{}<>`)
	term = strings.ToLower(term)
	if len(term) <= 4 {
		return term
	}
	suffixes := []string{"ences", "ance", "ments", "ment", "ings", "ing", "ers", "er", "ed", "es", "s"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(term, suffix) && len(term)-len(suffix) >= 4 {
			return term[:len(term)-len(suffix)]
		}
	}
	return term
}

func scanObservations(rows *sql.Rows) ([]Observation, error) {
	var out []Observation
	for rows.Next() {
		var o Observation
		if err := rows.Scan(&o.ID, &o.EntityID, &o.EntityName, &o.Content, &o.Source, &o.Confidence, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func nullableEntityID(id string) any {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return id
}

func (d *DB) LastObservation(ctx context.Context) (Observation, error) {
	var out Observation
	err := d.SQL.QueryRowContext(ctx, `
		SELECT o.id, COALESCE(o.entity_id, ''), COALESCE(e.name, ''), o.content, o.source, o.confidence, o.created_at
		FROM observations o
		LEFT JOIN entities e ON e.id = o.entity_id
		ORDER BY o.created_at DESC, o.id DESC
		LIMIT 1
	`).Scan(&out.ID, &out.EntityID, &out.EntityName, &out.Content, &out.Source, &out.Confidence, &out.CreatedAt)
	if err != nil {
		return Observation{}, err
	}
	return out, nil
}
