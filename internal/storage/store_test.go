package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/domain"
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
	eventRepo := store.Events()
	if eventRepo == nil {
		t.Fatalf("expected trade event repository")
	}
	stateRepo := store.ComplianceState()
	if stateRepo == nil {
		t.Fatalf("expected compliance state repository")
	}
	strategyRepo := store.Strategy()
	if strategyRepo == nil {
		t.Fatalf("expected strategy repository")
	}

	p1 := EquityPoint{Time: time.Now().UTC(), Equity: 100000}
	p2 := EquityPoint{Time: time.Now().UTC().Add(time.Second), Equity: 100120.5}
	if err := repo.Append(p1); err != nil {
		t.Fatalf("append p1 failed: %v", err)
	}
	if err := repo.Append(p2); err != nil {
		t.Fatalf("append p2 failed: %v", err)
	}
	if err := eventRepo.Append(domain.Event{
		Time:    time.Now().UTC(),
		Type:    "order_placed",
		Details: "buy AAPL 1.00 (abc123)",
	}); err != nil {
		t.Fatalf("append trade event failed: %v", err)
	}
	if err := stateRepo.AppendUnsettledSell(ComplianceUnsettledSell{
		Amount:    250,
		SettlesAt: time.Now().UTC().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("append compliance state failed: %v", err)
	}
	planBundle, err := strategyRepo.CreatePlan(StrategyPlan{
		GeneratedAt: time.Now().UTC(),
		Status:      StrategyPlanStatusDraft,
		Objective:   "smoke test strategy persistence",
		Watchlist:   []string{"AAPL"},
	}, []StrategyRecommendation{
		{
			Symbol:      "AAPL",
			Bias:        "buy",
			Confidence:  0.6,
			MaxNotional: 1000,
			Priority:    1,
		},
	})
	if err != nil {
		t.Fatalf("append strategy plan failed: %v", err)
	}
	if err := strategyRepo.SetPlanStatus(planBundle.Plan.ID, StrategyPlanStatusActive); err != nil {
		t.Fatalf("activate strategy plan failed: %v", err)
	}

	points, err := repo.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
	events, err := eventRepo.ListRecent(10)
	if err != nil {
		t.Fatalf("list recent events failed: %v", err)
	}
	if len(events) != 1 || events[0].Type != "order_placed" {
		t.Fatalf("unexpected persisted events: %#v", events)
	}
	unsettled, err := stateRepo.ListUnsettledSells(time.Now().UTC())
	if err != nil {
		t.Fatalf("list unsettled sells failed: %v", err)
	}
	if len(unsettled) != 1 {
		t.Fatalf("expected 1 unsettled sell, got %d", len(unsettled))
	}
	activePlan, err := strategyRepo.GetActivePlan()
	if err != nil {
		t.Fatalf("get active strategy plan failed: %v", err)
	}
	if activePlan == nil || len(activePlan.Recommendations) != 1 {
		t.Fatalf("expected active strategy plan with one recommendation, got %#v", activePlan)
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
	events, err = reopened.Events().ListRecent(10)
	if err != nil {
		t.Fatalf("list recent events after reopen failed: %v", err)
	}
	if len(events) != 1 || events[0].Type != "order_placed" {
		t.Fatalf("unexpected persisted events after reopen: %#v", events)
	}
	unsettled, err = reopened.ComplianceState().ListUnsettledSells(time.Now().UTC())
	if err != nil {
		t.Fatalf("list unsettled sells after reopen failed: %v", err)
	}
	if len(unsettled) != 1 {
		t.Fatalf("expected 1 unsettled sell after reopen, got %d", len(unsettled))
	}
	activePlan, err = reopened.Strategy().GetActivePlan()
	if err != nil {
		t.Fatalf("get active strategy plan after reopen failed: %v", err)
	}
	if activePlan == nil || activePlan.Plan.Status != StrategyPlanStatusActive {
		t.Fatalf("expected persisted active strategy plan after reopen, got %#v", activePlan)
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
