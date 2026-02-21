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
	analyst := &fakeAnalyst{
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
	}
	runner := NewRunner(
		e,
		analyst,
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
	if analyst.calls == 0 {
		t.Fatalf("expected analyst to be called")
	}
	if store.lastStatus != storage.StrategyPlanStatusActive {
		t.Fatalf("expected active status, got %q", store.lastStatus)
	}
}

func TestRunnerSkipsCycleWhenLatestPlanIsFresh(t *testing.T) {
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

	store := &fakePlanStore{
		latest: &storage.StrategyPlanWithRecommendations{
			Plan: storage.StrategyPlan{
				ID:          42,
				GeneratedAt: time.Now().UTC().Add(-10 * time.Minute),
				Status:      storage.StrategyPlanStatusActive,
			},
		},
	}
	analyst := &fakeAnalyst{
		plan: Plan{
			Summary: "should not be used",
		},
	}
	runner := NewRunner(
		e,
		analyst,
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

	if err := runner.runCycle(context.Background(), false, "startup"); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	if analyst.calls != 0 {
		t.Fatalf("expected analyst to not be called for fresh plan, got %d calls", analyst.calls)
	}
	if store.created != 0 {
		t.Fatalf("expected no new plan persisted, got %d", store.created)
	}
	if !hasEventType(e.Snapshot().Events, "strategy_cycle_skipped") {
		t.Fatalf("expected strategy_cycle_skipped event")
	}
}

func TestRunnerNoChangePlanDoesNotPersistNewPlan(t *testing.T) {
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

	store := &fakePlanStore{
		active: &storage.StrategyPlanWithRecommendations{
			Plan: storage.StrategyPlan{
				ID:          7,
				GeneratedAt: time.Now().UTC().Add(-3 * time.Hour),
				Status:      storage.StrategyPlanStatusActive,
				Summary:     "existing plan",
			},
			Recommendations: []storage.StrategyRecommendation{
				{Symbol: "AAPL", Bias: "buy", Priority: 1},
			},
		},
	}
	store.latest = store.active
	analyst := &fakeAnalyst{
		plan: Plan{
			NoChange: true,
			Summary:  "still valid",
		},
	}
	runner := NewRunner(
		e,
		analyst,
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

	if err := runner.runCycle(context.Background(), true, "manual"); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	if analyst.calls != 1 {
		t.Fatalf("expected analyst to be called once, got %d", analyst.calls)
	}
	if store.created != 0 {
		t.Fatalf("expected no new plan persisted on no-change, got %d", store.created)
	}
	if !hasEventType(e.Snapshot().Events, "strategy_plan_unchanged") {
		t.Fatalf("expected strategy_plan_unchanged event")
	}
}

type fakeAnalyst struct {
	plan  Plan
	err   error
	calls int
}

func (f *fakeAnalyst) BuildPlan(context.Context, Input) (Plan, error) {
	f.calls++
	return f.plan, f.err
}

type fakePlanStore struct {
	created    int
	lastStatus storage.StrategyPlanStatus
	active     *storage.StrategyPlanWithRecommendations
	latest     *storage.StrategyPlanWithRecommendations
}

func (s *fakePlanStore) CreatePlan(plan storage.StrategyPlan, recommendations []storage.StrategyRecommendation) (storage.StrategyPlanWithRecommendations, error) {
	s.created++
	plan.ID = uint(s.created)
	out := storage.StrategyPlanWithRecommendations{
		Plan:            plan,
		Recommendations: recommendations,
	}
	s.latest = &out
	return out, nil
}

func (s *fakePlanStore) SetPlanStatus(planID uint, status storage.StrategyPlanStatus) error {
	s.lastStatus = status
	if status == storage.StrategyPlanStatusActive && s.latest != nil && s.latest.Plan.ID == planID {
		s.active = s.latest
	}
	return nil
}

func (s *fakePlanStore) GetActivePlan() (*storage.StrategyPlanWithRecommendations, error) {
	return s.active, nil
}

func (s *fakePlanStore) GetLatestPlan() (*storage.StrategyPlanWithRecommendations, error) {
	return s.latest, nil
}

func hasEventType(events []domain.Event, eventType string) bool {
	for _, e := range events {
		if e.Type == eventType {
			return true
		}
	}
	return false
}
