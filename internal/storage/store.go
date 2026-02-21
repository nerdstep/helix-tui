package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	_ "modernc.org/sqlite"
)

const DefaultPath = "logs/helix.db"

const (
	sqliteBusyTimeoutMillis = 5000
)

type Config struct {
	Path string
}

type Store struct {
	path     string
	db       *gorm.DB
	equity   *EquityRepository
	events   *TradeEventRepository
	state    *ComplianceStateRepository
	strategy *StrategyRepository
}

func Open(cfg Config) (*Store, error) {
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		path = DefaultPath
	}
	if err := ensureParentDir(path); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        path,
	}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %q: %w", path, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %q sql handle: %w", path, err)
	}
	if err := configureSQLite(sqlDB); err != nil {
		return nil, fmt.Errorf("configure sqlite database %q: %w", path, err)
	}
	if err := runMigrations(db); err != nil {
		return nil, err
	}

	return &Store{
		path:     path,
		db:       db,
		equity:   &EquityRepository{db: db},
		events:   &TradeEventRepository{db: db},
		state:    &ComplianceStateRepository{db: db},
		strategy: &StrategyRepository{db: db},
	}, nil
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) EquityHistory() *EquityRepository {
	if s == nil {
		return nil
	}
	return s.equity
}

func (s *Store) Events() *TradeEventRepository {
	if s == nil {
		return nil
	}
	return s.events
}

func (s *Store) ComplianceState() *ComplianceStateRepository {
	if s == nil {
		return nil
	}
	return s.state
}

func (s *Store) Strategy() *StrategyRepository {
	if s == nil {
		return nil
	}
	return s.strategy
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func configureSQLite(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("sqlite database handle is nil")
	}

	// Keep one writer connection to avoid cross-connection write lock churn.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	pragmas := []string{
		fmt.Sprintf("PRAGMA busy_timeout = %d;", sqliteBusyTimeoutMillis),
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA foreign_keys = ON;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return err
		}
	}
	return nil
}
