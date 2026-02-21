package runtime

import (
	"path/filepath"
	"testing"
	"time"

	"helix-tui/internal/engine"
	"helix-tui/internal/storage"
)

func TestComplianceStateAdapter_RoundTrip(t *testing.T) {
	store, err := storage.Open(storage.Config{Path: filepath.Join(t.TempDir(), "state.db")})
	if err != nil {
		t.Fatalf("open store failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	adapter := complianceStateAdapter{repo: store.ComplianceState()}
	now := time.Date(2026, time.February, 21, 10, 0, 0, 0, time.UTC)
	if err := adapter.AppendUnsettledSellProceeds(engine.UnsettledSellProceeds{
		Amount:    500,
		SettlesAt: now.Add(24 * time.Hour),
	}, now); err != nil {
		t.Fatalf("append unsettled proceeds failed: %v", err)
	}
	if err := adapter.PruneSettledSellProceeds(now); err != nil {
		t.Fatalf("prune settled proceeds failed: %v", err)
	}
	lots, err := adapter.LoadUnsettledSellProceeds(now)
	if err != nil {
		t.Fatalf("load unsettled proceeds failed: %v", err)
	}
	if len(lots) != 1 || lots[0].Amount != 500 {
		t.Fatalf("unexpected unsettled proceeds: %#v", lots)
	}
}

func TestComplianceStateAdapter_NilRepository(t *testing.T) {
	adapter := complianceStateAdapter{}
	now := time.Now().UTC()
	if _, err := adapter.LoadUnsettledSellProceeds(now); err == nil {
		t.Fatalf("expected load error for nil repo")
	}
	if err := adapter.AppendUnsettledSellProceeds(engine.UnsettledSellProceeds{
		Amount:    1,
		SettlesAt: now.Add(time.Hour),
	}, now); err == nil {
		t.Fatalf("expected append error for nil repo")
	}
	if err := adapter.PruneSettledSellProceeds(now); err == nil {
		t.Fatalf("expected prune error for nil repo")
	}
}
