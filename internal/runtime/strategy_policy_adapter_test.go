package runtime

import (
	"path/filepath"
	"testing"
	"time"

	"helix-tui/internal/storage"
)

func TestStrategyPolicyAdapterGetActiveStrategyPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.db")
	store, err := storage.Open(storage.Config{Path: path})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()
	bundle, err := repo.CreatePlan(storage.StrategyPlan{
		GeneratedAt: time.Now().UTC(),
		Status:      storage.StrategyPlanStatusDraft,
	}, []storage.StrategyRecommendation{
		{
			Symbol:      "AAPL",
			Bias:        "buy",
			MaxNotional: 1234,
			Priority:    1,
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if err := repo.SetPlanStatus(bundle.Plan.ID, storage.StrategyPlanStatusActive); err != nil {
		t.Fatalf("activate plan: %v", err)
	}

	adapter := strategyPolicyAdapter{repo: repo}
	policy, err := adapter.GetActiveStrategyPolicy()
	if err != nil {
		t.Fatalf("get active policy: %v", err)
	}
	if policy == nil {
		t.Fatalf("expected active policy")
	}
	if policy.PlanID != bundle.Plan.ID {
		t.Fatalf("unexpected plan id: got %d want %d", policy.PlanID, bundle.Plan.ID)
	}
	if len(policy.Recommendations) != 1 {
		t.Fatalf("expected one recommendation, got %d", len(policy.Recommendations))
	}
	if policy.Recommendations[0].Symbol != "AAPL" || policy.Recommendations[0].Bias != "buy" {
		t.Fatalf("unexpected recommendation: %#v", policy.Recommendations[0])
	}
}
