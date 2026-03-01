package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ActionLogEntry struct {
	ID         int64  `json:"id"`
	Agent      string `json:"agent"`
	ActionType string `json:"action_type"`
	Summary    string `json:"summary"`
	Detail     string `json:"detail,omitempty"`
	Entities   string `json:"entities,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

func (d *DB) AppendActionLog(ctx context.Context, agent, actionType, summary string, detail any, entityIDs []string) (int64, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		agent = "openclaw"
	}
	actionType = strings.TrimSpace(actionType)
	if actionType == "" {
		return 0, fmt.Errorf("action_type is required")
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return 0, fmt.Errorf("summary is required")
	}

	var detailStr string
	if detail != nil {
		buf, err := json.Marshal(detail)
		if err != nil {
			return 0, fmt.Errorf("marshal detail: %w", err)
		}
		if !json.Valid(buf) {
			return 0, fmt.Errorf("detail must be valid JSON")
		}
		detailStr = string(buf)
	}
	normalizedEntityIDs := make([]string, 0, len(entityIDs))
	for _, rawID := range entityIDs {
		id := strings.TrimSpace(strings.ToLower(rawID))
		if id == "" {
			return 0, fmt.Errorf("entity_ids contains an empty value")
		}
		if err := validateEntityID(id); err != nil {
			return 0, err
		}
		normalizedEntityIDs = append(normalizedEntityIDs, id)
	}
	entitiesStr := strings.Join(normalizedEntityIDs, ",")

	res, err := d.SQL.ExecContext(ctx, `
		INSERT INTO action_log(agent, action_type, summary, detail, entities, created_at)
		VALUES(?, ?, ?, ?, ?, ?)
	`, agent, actionType, summary, detailStr, entitiesStr, UnixNow())
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (d *DB) TailActionLog(ctx context.Context, limit int, since *time.Time, agent string) ([]ActionLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	args := []any{}
	clauses := []string{"1=1"}
	if since != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, since.Unix())
	}
	if strings.TrimSpace(agent) != "" {
		clauses = append(clauses, "agent = ?")
		args = append(args, agent)
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT id, agent, action_type, summary, COALESCE(detail, ''), COALESCE(entities, ''), created_at
		FROM action_log
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, strings.Join(clauses, " AND "))

	rows, err := d.SQL.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActionLogEntry
	for rows.Next() {
		var e ActionLogEntry
		if err := rows.Scan(&e.ID, &e.Agent, &e.ActionType, &e.Summary, &e.Detail, &e.Entities, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) LastActionLogTime(ctx context.Context) (int64, error) {
	var ts sql.NullInt64
	if err := d.SQL.QueryRowContext(ctx, `SELECT MAX(created_at) FROM action_log`).Scan(&ts); err != nil {
		return 0, err
	}
	if !ts.Valid {
		return 0, nil
	}
	return ts.Int64, nil
}

func (d *DB) LastSearchLog(ctx context.Context) (ActionLogEntry, error) {
	var out ActionLogEntry
	err := d.SQL.QueryRowContext(ctx, `
		SELECT id, agent, action_type, summary, COALESCE(detail, ''), COALESCE(entities, ''), created_at
		FROM action_log
		WHERE summary LIKE 'search:%'
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`).Scan(&out.ID, &out.Agent, &out.ActionType, &out.Summary, &out.Detail, &out.Entities, &out.CreatedAt)
	if err != nil {
		return ActionLogEntry{}, err
	}
	return out, nil
}
