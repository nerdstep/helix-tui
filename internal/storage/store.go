package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

const DefaultPath = "logs/helix.db"

type Config struct {
	Path string
}

type Store struct {
	path   string
	db     *gorm.DB
	equity *EquityRepository
	events *TradeEventRepository
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
	}, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %q: %w", path, err)
	}
	if err := runMigrations(db); err != nil {
		return nil, err
	}

	return &Store{
		path:   path,
		db:     db,
		equity: &EquityRepository{db: db},
		events: &TradeEventRepository{db: db},
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

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
