package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreOpenMigrateAppendListAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.EquityHistory()
	if repo == nil {
		t.Fatalf("expected equity repository")
	}
	var firstRunCount int64
	if err := store.db.Model(&schemaMigration{}).Count(&firstRunCount).Error; err != nil {
		t.Fatalf("count schema migrations failed: %v", err)
	}
	if firstRunCount == 0 {
		t.Fatalf("expected at least one applied migration")
	}

	p1 := EquityPoint{Time: time.Now().UTC(), Equity: 100000}
	p2 := EquityPoint{Time: time.Now().UTC().Add(time.Second), Equity: 100120.5}
	if err := repo.Append(p1); err != nil {
		t.Fatalf("append p1 failed: %v", err)
	}
	if err := repo.Append(p2); err != nil {
		t.Fatalf("append p2 failed: %v", err)
	}

	points, err := repo.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}

	_ = store.Close()

	reopened, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	t.Cleanup(func() {
		_ = reopened.Close()
	})
	points, err = reopened.EquityHistory().List()
	if err != nil {
		t.Fatalf("list after reopen failed: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected persisted points after reopen, got %d", len(points))
	}
	var secondRunCount int64
	if err := reopened.db.Model(&schemaMigration{}).Count(&secondRunCount).Error; err != nil {
		t.Fatalf("count schema migrations on reopen failed: %v", err)
	}
	if secondRunCount != firstRunCount {
		t.Fatalf("expected migration count to remain stable, got %d then %d", firstRunCount, secondRunCount)
	}
}

func TestStoreOpenInvalidPath(t *testing.T) {
	path := t.TempDir()
	_, err := Open(Config{Path: path})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "open sqlite database") {
		t.Fatalf("unexpected error: %v", err)
	}
}
