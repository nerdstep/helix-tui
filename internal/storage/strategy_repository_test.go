package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStrategyRepositoryCreateActivateAndList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()
	if repo == nil {
		t.Fatalf("expected strategy repository")
	}

	created, err := repo.CreatePlan(
		StrategyPlan{
			GeneratedAt:   time.Now().UTC(),
			Status:        StrategyPlanStatusDraft,
			AnalystModel:  "gpt-5",
			PromptVersion: "strategy-v1",
			Objective:     "Prioritize momentum names with defined risk.",
			Watchlist:     []string{"AAPL", "MSFT", "NVDA"},
			Summary:       "Lean long on semis, neutral mega-cap software.",
			Confidence:    0.71,
		},
		[]StrategyRecommendation{
			{
				Symbol:       "nvda",
				Bias:         "buy",
				Confidence:   0.74,
				EntryMin:     920.0,
				EntryMax:     940.0,
				TargetPrice:  980.0,
				StopPrice:    900.0,
				MaxNotional:  5000.0,
				Thesis:       "Relative strength + earnings momentum.",
				Invalidation: "Breakdown below 900 on expanding volume.",
				Priority:     1,
			},
			{
				Symbol:       "aapl",
				Bias:         "hold",
				Confidence:   0.55,
				EntryMin:     0,
				EntryMax:     0,
				TargetPrice:  0,
				StopPrice:    0,
				MaxNotional:  0,
				Thesis:       "Range-bound while awaiting catalyst.",
				Invalidation: "Confirmed trend break from range.",
				Priority:     2,
			},
		},
	)
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}
	if created.Plan.ID == 0 {
		t.Fatalf("expected persisted strategy plan ID")
	}
	if got := len(created.Recommendations); got != 2 {
		t.Fatalf("expected 2 recommendations, got %d", got)
	}
	if created.Recommendations[0].PlanID != created.Plan.ID {
		t.Fatalf("expected recommendation plan id %d, got %d", created.Plan.ID, created.Recommendations[0].PlanID)
	}

	recs, err := repo.ListRecommendations(created.Plan.ID)
	if err != nil {
		t.Fatalf("ListRecommendations failed: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 persisted recommendations, got %d", len(recs))
	}
	if recs[0].Symbol != "NVDA" {
		t.Fatalf("expected normalized symbol NVDA, got %q", recs[0].Symbol)
	}

	if err := repo.SetPlanStatus(created.Plan.ID, StrategyPlanStatusActive); err != nil {
		t.Fatalf("SetPlanStatus(active) failed: %v", err)
	}
	active, err := repo.GetActivePlan()
	if err != nil {
		t.Fatalf("GetActivePlan failed: %v", err)
	}
	if active == nil {
		t.Fatalf("expected active plan")
	}
	if active.Plan.ID != created.Plan.ID {
		t.Fatalf("expected active plan id %d, got %d", created.Plan.ID, active.Plan.ID)
	}
	if active.Plan.Status != StrategyPlanStatusActive {
		t.Fatalf("expected status active, got %q", active.Plan.Status)
	}
	if len(active.Recommendations) != 2 {
		t.Fatalf("expected active plan recommendations, got %d", len(active.Recommendations))
	}

	plans, err := repo.ListRecentPlans(10)
	if err != nil {
		t.Fatalf("ListRecentPlans failed: %v", err)
	}
	if len(plans) == 0 {
		t.Fatalf("expected recent plans")
	}
	if plans[0].ID != created.Plan.ID {
		t.Fatalf("expected most recent plan id %d, got %d", created.Plan.ID, plans[0].ID)
	}
}

func TestStrategyRepositoryActivatingPlanSupersedesPriorActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-activate.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()
	first, err := repo.CreatePlan(StrategyPlan{
		GeneratedAt: time.Now().UTC().Add(-time.Hour),
		Status:      StrategyPlanStatusDraft,
		Objective:   "first",
	}, nil)
	if err != nil {
		t.Fatalf("CreatePlan first failed: %v", err)
	}
	second, err := repo.CreatePlan(StrategyPlan{
		GeneratedAt: time.Now().UTC(),
		Status:      StrategyPlanStatusDraft,
		Objective:   "second",
	}, nil)
	if err != nil {
		t.Fatalf("CreatePlan second failed: %v", err)
	}

	if err := repo.SetPlanStatus(first.Plan.ID, StrategyPlanStatusActive); err != nil {
		t.Fatalf("activate first failed: %v", err)
	}
	if err := repo.SetPlanStatus(second.Plan.ID, StrategyPlanStatusActive); err != nil {
		t.Fatalf("activate second failed: %v", err)
	}

	active, err := repo.GetActivePlan()
	if err != nil {
		t.Fatalf("GetActivePlan failed: %v", err)
	}
	if active == nil || active.Plan.ID != second.Plan.ID {
		t.Fatalf("expected second plan active, got %#v", active)
	}

	plans, err := repo.ListRecentPlans(10)
	if err != nil {
		t.Fatalf("ListRecentPlans failed: %v", err)
	}
	if len(plans) < 2 {
		t.Fatalf("expected at least 2 plans, got %d", len(plans))
	}
	var firstStatus StrategyPlanStatus
	for _, p := range plans {
		if p.ID == first.Plan.ID {
			firstStatus = p.Status
			break
		}
	}
	if firstStatus != StrategyPlanStatusSuperseded {
		t.Fatalf("expected first plan to be superseded, got %q", firstStatus)
	}
}

func TestStrategyRepositoryRequiresInitializedDB(t *testing.T) {
	var repo *StrategyRepository
	if _, err := repo.ListRecentPlans(1); err == nil {
		t.Fatalf("expected uninitialized repository error")
	}
}

func TestStrategyRepositoryGetActivePlan_NoActivePlanReturnsNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-empty-active.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()
	active, err := repo.GetActivePlan()
	if err != nil {
		t.Fatalf("GetActivePlan failed: %v", err)
	}
	if active != nil {
		t.Fatalf("expected nil active plan when none exists, got %#v", active)
	}
}
