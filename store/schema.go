package store

const (
	MigrationCore = `
CREATE TABLE IF NOT EXISTS entities (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'general',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS observations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_id   TEXT REFERENCES entities(id) ON DELETE CASCADE,
    content     TEXT NOT NULL,
    source      TEXT DEFAULT 'agent',
    confidence  REAL DEFAULT 1.0,
    created_at  INTEGER NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
    content,
    entity_id UNINDEXED,
    content='observations',
    content_rowid='id',
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS obs_fts_insert AFTER INSERT ON observations BEGIN
    INSERT INTO observations_fts(rowid, content, entity_id) VALUES (new.id, new.content, new.entity_id);
END;

CREATE TRIGGER IF NOT EXISTS obs_fts_delete AFTER DELETE ON observations BEGIN
    INSERT INTO observations_fts(observations_fts, rowid, content, entity_id) VALUES ('delete', old.id, old.content, old.entity_id);
END;

CREATE TABLE IF NOT EXISTS action_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent       TEXT NOT NULL DEFAULT 'openclaw',
    action_type TEXT NOT NULL,
    summary     TEXT NOT NULL,
    detail      TEXT,
    entities    TEXT,
    created_at  INTEGER NOT NULL
);
`

	MigrationVectors = `
CREATE VIRTUAL TABLE IF NOT EXISTS observation_vectors USING vec0(
    observation_id INTEGER PRIMARY KEY,
    embedding float[768]
);
`
)
