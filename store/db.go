package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"go.uber.org/zap"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	sqlite_vec.Auto()
}

const (
	defaultDataDirName   = ".clawstore"
	defaultConfigDirName = ".config/clawstore"
	defaultDBFileName    = "store.db"
)

type DB struct {
	SQL         *sql.DB
	Logger      *zap.Logger
	DBPath      string
	DataDir     string
	ConfigDir   string
	VecEnabled  bool
	VecTableErr error
}

type Stats struct {
	EntityCount      int
	ObservationCount int
	VectorCount      int
	ActionLogCount   int
	MissingVectors   int
	LastWriteTS      int64
}

func OpenDefault(logger *zap.Logger) (*DB, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return nil, err
	}
	return Open(paths.DBPath, paths.DataDir, paths.ConfigDir, logger)
}

func Open(dbPath, dataDir, configDir string, logger *zap.Logger) (*DB, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	dsn := fmt.Sprintf("%s?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL", dbPath)
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	db := &DB{
		SQL:       sqlDB,
		Logger:    logger,
		DBPath:    dbPath,
		DataDir:   dataDir,
		ConfigDir: configDir,
	}

	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return db, nil
}

func (d *DB) Close() error {
	if d == nil || d.SQL == nil {
		return nil
	}
	return d.SQL.Close()
}

func (d *DB) migrate() error {
	if _, err := d.SQL.Exec(MigrationCore); err != nil {
		return fmt.Errorf("run core migrations: %w", err)
	}

	if _, err := d.SQL.Exec(MigrationVectors); err != nil {
		d.VecEnabled = false
		d.VecTableErr = err
		d.Logger.Warn("vec0 table unavailable; vectors disabled", zap.Error(err))
		return nil
	}
	d.VecEnabled = true
	return nil
}

func (d *DB) Stats(ctx context.Context) (Stats, error) {
	out := Stats{}
	if err := d.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM entities").Scan(&out.EntityCount); err != nil {
		return out, err
	}
	if err := d.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM observations").Scan(&out.ObservationCount); err != nil {
		return out, err
	}
	if err := d.SQL.QueryRowContext(ctx, "SELECT COALESCE(MAX(created_at), 0) FROM observations").Scan(&out.LastWriteTS); err != nil {
		return out, err
	}
	if d.VecEnabled {
		if err := d.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM observation_vectors").Scan(&out.VectorCount); err != nil {
			return out, err
		}
	} else {
		out.VectorCount = 0
	}
	if err := d.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM action_log").Scan(&out.ActionLogCount); err != nil {
		return out, err
	}
	out.MissingVectors = out.ObservationCount - out.VectorCount
	if out.MissingVectors < 0 {
		out.MissingVectors = 0
	}
	return out, nil
}

type Paths struct {
	Home      string
	DataDir   string
	ConfigDir string
	DBPath    string
}

func ResolvePaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return Paths{}, errors.New("home directory is empty")
	}
	dataDir := filepath.Join(home, defaultDataDirName)
	configDir := filepath.Join(home, defaultConfigDirName)
	return Paths{
		Home:      home,
		DataDir:   dataDir,
		ConfigDir: configDir,
		DBPath:    filepath.Join(dataDir, defaultDBFileName),
	}, nil
}

func UnixNow() int64 {
	return time.Now().Unix()
}
