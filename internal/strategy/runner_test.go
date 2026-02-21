package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	"helix-tui/internal/broker/paper"
	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
	"helix-tui/internal/storage"
)

func TestRunnerPersistsAndActivatesPlan(t *testing.T) {
	risk := engine.NewRiskGate(engine.Policy{
		AllowMarketOrders: true,
		AllowSymbols: map[string]struct{}{
			"AAPL": {},
		},
	})
	e := engine.New(paper.New(100000), risk)
	if err := e.Sync(context.Background()); err != nil {
		t.Fatalf("sync engine: %v", err)
	}

	store := &fakePlanStore{}
	runner := NewRunner(
		e,
		fakeAnalyst{
			plan: Plan{
				Summary:    "Lean long on quality large caps.",
				Confidence: 0.72,
				Recommendations: []Recommendation{
					{
						Symbol:      "AAPL",
						Bias:        "buy",
						Confidence:  0.71,
						MaxNotional: 2500,
						Priority:    1,
					},
				},
			},
		},
		[]string{"AAPL"},
		time.Hour,
		2*time.Second,
		"test objective",
		"test-model",
		"strategy-v1",
		4,
		true,
	)
	runner.SetStore(store)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := runner.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if store.created == 0 {
		t.Fatalf("expected at least one persisted strategy plan")
	}
	if store.lastStatus != storage.StrategyPlanStatusActive {
		t.Fatalf("expected active status, got %q", store.lastStatus)
	}
}

type fakeAnalyst struct {
	plan Plan
	err  error
}

func (f fakeAnalyst) BuildPlan(context.Context, Input) (Plan, error) {
	return f.plan, f.err
}

type fakePlanStore struct {
	created    int
	lastStatus storage.StrategyPlanStatus
}

func (s *fakePlanStore) CreatePlan(plan storage.StrategyPlan, recommendations []storage.StrategyRecommendation) (storage.StrategyPlanWithRecommendations, error) {
	s.created++
	plan.ID = uint(s.created)
	return storage.StrategyPlanWithRecommendations{
		Plan:            plan,
		Recommendations: recommendations,
	}, nil
}

func (s *fakePlanStore) SetPlanStatus(planID uint, status storage.StrategyPlanStatus) error {
	s.lastStatus = status
	return nil
}

func (s *fakePlanStore) ListRecent(limit int) ([]domain.Event, error) {
	return nil, nil
}
