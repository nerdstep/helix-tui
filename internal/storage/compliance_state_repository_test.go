package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestComplianceStateRepository_AppendListPruneAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("open store failed: %v", err)
	}
	repo := store.ComplianceState()
	if repo == nil {
		t.Fatalf("expected compliance state repository")
	}

	now := time.Date(2026, time.February, 21, 12, 0, 0, 0, time.UTC)
	if err := repo.AppendUnsettledSell(ComplianceUnsettledSell{
		Amount:    1000,
		SettlesAt: now.Add(24 * time.Hour),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("append unsettled sell failed: %v", err)
	}
	if err := repo.AppendUnsettledSell(ComplianceUnsettledSell{
		Amount:    500,
		SettlesAt: now.Add(-24 * time.Hour),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("append settled sell failed: %v", err)
	}

	unsettled, err := repo.ListUnsettledSells(now)
	if err != nil {
		t.Fatalf("list unsettled failed: %v", err)
	}
	if len(unsettled) != 1 || unsettled[0].Amount != 1000 {
		t.Fatalf("unexpected unsettled rows: %#v", unsettled)
	}

	if err := repo.PruneSettledSells(now); err != nil {
		t.Fatalf("prune settled failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store failed: %v", err)
	}

	reopened, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("reopen store failed: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	unsettled, err = reopened.ComplianceState().ListUnsettledSells(now)
	if err != nil {
		t.Fatalf("list unsettled after reopen failed: %v", err)
	}
	if len(unsettled) != 1 || unsettled[0].Amount != 1000 {
		t.Fatalf("unexpected unsettled rows after reopen: %#v", unsettled)
	}
}
