package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

var slugCleanupRE = regexp.MustCompile(`[^a-z0-9]+`)
var validEntityIDRE = regexp.MustCompile(`^[a-z0-9_-]+$`)

var validEntityTypes = map[string]struct{}{
	"person":     {},
	"project":    {},
	"place":      {},
	"preference": {},
	"concept":    {},
	"general":    {},
}

type Entity struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
	ObservationCount int    `json:"observation_count,omitempty"`
}

func validateEntityID(id string) error {
	if len(id) == 0 {
		return errors.New("entity_id cannot be empty")
	}
	if len(id) > 100 {
		return fmt.Errorf("entity_id too long (max 100 chars)")
	}
	if !validEntityIDRE.MatchString(id) {
		return errors.New("entity_id must be lowercase alphanumeric with hyphens/underscores only")
	}
	return nil
}

func Slugify(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	s = slugCleanupRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "entity-" + strings.ToLower(strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
	}
	return s
}

func HumanizeSlug(slug string) string {
	if strings.TrimSpace(slug) == "" {
		return "Unknown"
	}
	parts := strings.Split(strings.ReplaceAll(slug, "_", "-"), "-")
	for i := range parts {
		parts[i] = titleWord(parts[i])
	}
	return strings.Join(parts, " ")
}

func titleWord(s string) string {
	r := []rune(strings.ToLower(s))
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func normalizeEntityType(typ string) (string, error) {
	t := strings.TrimSpace(strings.ToLower(typ))
	if t == "" {
		t = "general"
	}
	if _, ok := validEntityTypes[t]; !ok {
		return "", fmt.Errorf("invalid entity type %q", typ)
	}
	return t, nil
}

func (d *DB) CreateEntity(ctx context.Context, id, name, typ string) (Entity, error) {
	if strings.TrimSpace(id) == "" {
		id = Slugify(name)
	}
	if strings.TrimSpace(name) == "" {
		name = HumanizeSlug(id)
	}
	id = strings.ToLower(id)
	if err := validateEntityID(id); err != nil {
		return Entity{}, err
	}
	t, err := normalizeEntityType(typ)
	if err != nil {
		return Entity{}, err
	}
	now := UnixNow()
	_, err = d.SQL.ExecContext(ctx,
		`INSERT INTO entities(id, name, type, created_at, updated_at) VALUES(?, ?, ?, ?, ?)`,
		id, name, t, now, now,
	)
	if err != nil {
		return Entity{}, err
	}
	return d.GetEntity(ctx, id)
}

func (d *DB) UpsertEntity(ctx context.Context, id, name, typ string) (Entity, error) {
	if strings.TrimSpace(id) == "" {
		id = Slugify(name)
	}
	if strings.TrimSpace(name) == "" {
		name = HumanizeSlug(id)
	}
	id = strings.ToLower(id)
	if err := validateEntityID(id); err != nil {
		return Entity{}, err
	}
	t, err := normalizeEntityType(typ)
	if err != nil {
		return Entity{}, err
	}
	now := UnixNow()
	_, err = d.SQL.ExecContext(ctx, `
		INSERT INTO entities(id, name, type, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			type = excluded.type,
			updated_at = excluded.updated_at
	`, id, name, t, now, now)
	if err != nil {
		return Entity{}, err
	}
	return d.GetEntity(ctx, id)
}

func (d *DB) GetEntity(ctx context.Context, id string) (Entity, error) {
	var e Entity
	id = strings.TrimSpace(strings.ToLower(id))
	if err := validateEntityID(id); err != nil {
		return Entity{}, err
	}
	err := d.SQL.QueryRowContext(ctx,
		`SELECT id, name, type, created_at, updated_at FROM entities WHERE id = ?`,
		id,
	).Scan(&e.ID, &e.Name, &e.Type, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entity{}, fmt.Errorf("entity %q not found", id)
		}
		return Entity{}, err
	}
	return e, nil
}

func (d *DB) EnsureEntity(ctx context.Context, id string) (Entity, error) {
	id = Slugify(id)
	if err := validateEntityID(id); err != nil {
		return Entity{}, err
	}
	existing, err := d.GetEntity(ctx, id)
	if err == nil {
		return existing, nil
	}
	return d.CreateEntity(ctx, id, HumanizeSlug(id), "general")
}

func (d *DB) DeleteEntity(ctx context.Context, id string) error {
	id = strings.TrimSpace(strings.ToLower(id))
	if err := validateEntityID(id); err != nil {
		return err
	}
	res, err := d.SQL.ExecContext(ctx, `DELETE FROM entities WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil
	}
	if n == 0 {
		return fmt.Errorf("entity %q not found", id)
	}
	return nil
}

func (d *DB) ListEntities(ctx context.Context, typeFilter string) ([]Entity, error) {
	query := `
		SELECT e.id, e.name, e.type, e.created_at, e.updated_at,
		       (SELECT COUNT(*) FROM observations o WHERE o.entity_id = e.id) as observation_count
		FROM entities e
		WHERE (? = '' OR e.type = ?)
		ORDER BY e.updated_at DESC
	`
	filter := strings.TrimSpace(typeFilter)
	if filter != "" {
		t, err := normalizeEntityType(filter)
		if err != nil {
			return nil, err
		}
		filter = t
	}
	rows, err := d.SQL.QueryContext(ctx, query, filter, filter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		var e Entity
		if err := rows.Scan(&e.ID, &e.Name, &e.Type, &e.CreatedAt, &e.UpdatedAt, &e.ObservationCount); err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entities, nil
}
