package strategy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
	"helix-tui/internal/storage"
	"helix-tui/internal/symbols"
)

type planStore interface {
	CreatePlan(plan storage.StrategyPlan, recommendations []storage.StrategyRecommendation) (storage.StrategyPlanWithRecommendations, error)
	SetPlanStatus(planID uint, status storage.StrategyPlanStatus) error
}

type eventHistoryStore interface {
	ListRecent(limit int) ([]domain.Event, error)
}

type Runner struct {
	engine      *engine.Engine
	analyst     Analyst
	store       planStore
	watchlist   []string
	interval    time.Duration
	syncTimeout time.Duration
	objective   string

	maxRecommendations int
	autoActivate       bool
	promptVersion      string
	model              string

	eventHistory eventHistoryStore

	triggerCh chan struct{}
	mu        sync.RWMutex
}

func NewRunner(
	engine *engine.Engine,
	analyst Analyst,
	watchlist []string,
	interval time.Duration,
	syncTimeout time.Duration,
	objective string,
	model string,
	promptVersion string,
	maxRecommendations int,
	autoActivate bool,
) *Runner {
	if interval <= 0 {
		interval = 4 * time.Hour
	}
	if syncTimeout <= 0 {
		syncTimeout = 15 * time.Second
	}
	if maxRecommendations <= 0 {
		maxRecommendations = 8
	}
	return &Runner{
		engine:             engine,
		analyst:            analyst,
		watchlist:          symbols.Normalize(watchlist),
		interval:           interval,
		syncTimeout:        syncTimeout,
		objective:          strings.TrimSpace(objective),
		model:              strings.TrimSpace(model),
		promptVersion:      strings.TrimSpace(promptVersion),
		maxRecommendations: maxRecommendations,
		autoActivate:       autoActivate,
		triggerCh:          make(chan struct{}, 1),
	}
}

func (r *Runner) SetStore(store planStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store = store
}

func (r *Runner) SetEventHistory(store eventHistoryStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.eventHistory = store
}

func (r *Runner) SetWatchlist(next []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.watchlist = symbols.Normalize(next)
}

func (r *Runner) TriggerNow(reason string) bool {
	if r == nil {
		return false
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual"
	}
	if r.engine != nil {
		r.engine.AddEvent("strategy_cycle_requested", fmt.Sprintf("reason=%s", reason))
	}
	select {
	case r.triggerCh <- struct{}{}:
		return true
	default:
		return false
	}
}

func (r *Runner) Run(ctx context.Context) error {
	if r.engine == nil {
		return fmt.Errorf("strategy runner requires engine")
	}
	if r.analyst == nil {
		return fmt.Errorf("strategy runner requires analyst")
	}
	if r.getStore() == nil {
		r.engine.AddEvent("strategy_runner_disabled", "database strategy store unavailable")
		return nil
	}

	r.engine.AddEvent("strategy_runner_start", fmt.Sprintf("interval=%s max_recommendations=%d", r.interval, r.maxRecommendations))
	if err := r.runCycle(ctx); err != nil {
		r.engine.AddEvent("strategy_cycle_error", err.Error())
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.engine.AddEvent("strategy_runner_stop", "context canceled")
			return ctx.Err()
		case <-ticker.C:
			if err := r.runCycle(ctx); err != nil {
				r.engine.AddEvent("strategy_cycle_error", err.Error())
			}
		case <-r.triggerCh:
			if err := r.runCycle(ctx); err != nil {
				r.engine.AddEvent("strategy_cycle_error", err.Error())
			}
		}
	}
}

func (r *Runner) runCycle(ctx context.Context) error {
	store := r.getStore()
	if store == nil {
		return fmt.Errorf("strategy store not configured")
	}
	watchlist := r.watchlistSnapshot()
	r.engine.AddEvent("strategy_cycle_start", fmt.Sprintf("watchlist=%d", len(watchlist)))

	syncCtx, cancel := context.WithTimeout(ctx, r.syncTimeout)
	defer cancel()
	if err := r.engine.SyncQuiet(syncCtx); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	snapshot := r.engine.Snapshot()
	input := Input{
		GeneratedAt:        time.Now().UTC(),
		Objective:          r.objective,
		MaxRecommendations: r.maxRecommendations,
		Watchlist:          watchlist,
		Snapshot:           snapshot,
		Quotes:             r.collectQuotes(syncCtx, watchlist),
		RecentEvents:       r.recentEvents(snapshot.Events),
	}
	plan, err := r.analyst.BuildPlan(ctx, input)
	if err != nil {
		return fmt.Errorf("build strategy plan: %w", err)
	}
	if strings.TrimSpace(plan.Summary) == "" && len(plan.Recommendations) == 0 {
		r.engine.AddEvent("strategy_plan_empty", "analyst returned no strategy output")
		return nil
	}

	recs := make([]storage.StrategyRecommendation, 0, len(plan.Recommendations))
	for i, rec := range plan.Recommendations {
		if strings.TrimSpace(rec.Symbol) == "" {
			continue
		}
		priority := rec.Priority
		if priority <= 0 {
			priority = i + 1
		}
		recs = append(recs, storage.StrategyRecommendation{
			Symbol:       rec.Symbol,
			Bias:         rec.Bias,
			Confidence:   rec.Confidence,
			EntryMin:     rec.EntryMin,
			EntryMax:     rec.EntryMax,
			TargetPrice:  rec.TargetPrice,
			StopPrice:    rec.StopPrice,
			MaxNotional:  rec.MaxNotional,
			Thesis:       rec.Thesis,
			Invalidation: rec.Invalidation,
			Priority:     priority,
		})
	}

	bundle, err := store.CreatePlan(storage.StrategyPlan{
		GeneratedAt:   input.GeneratedAt,
		Status:        storage.StrategyPlanStatusDraft,
		AnalystModel:  r.model,
		PromptVersion: r.promptVersion,
		Objective:     r.objective,
		Watchlist:     watchlist,
		Summary:       plan.Summary,
		Confidence:    plan.Confidence,
	}, recs)
	if err != nil {
		return fmt.Errorf("persist strategy plan: %w", err)
	}

	status := bundle.Plan.Status
	if r.autoActivate {
		if err := store.SetPlanStatus(bundle.Plan.ID, storage.StrategyPlanStatusActive); err != nil {
			return fmt.Errorf("activate strategy plan: %w", err)
		}
		status = storage.StrategyPlanStatusActive
	}
	r.engine.AddEvent(
		"strategy_plan_created",
		fmt.Sprintf("id=%d status=%s recs=%d conf=%.2f model=%s", bundle.Plan.ID, status, len(bundle.Recommendations), bundle.Plan.Confidence, bundle.Plan.AnalystModel),
	)
	return nil
}

func (r *Runner) collectQuotes(ctx context.Context, watchlist []string) []domain.Quote {
	out := make([]domain.Quote, 0, len(watchlist))
	for _, symbol := range watchlist {
		q, err := r.engine.GetQuote(ctx, symbol)
		if err != nil {
			continue
		}
		out = append(out, q)
	}
	return out
}

func (r *Runner) recentEvents(snapshotEvents []domain.Event) []domain.Event {
	r.mu.RLock()
	store := r.eventHistory
	r.mu.RUnlock()
	if store == nil {
		return snapshotEvents
	}
	events, err := store.ListRecent(100)
	if err != nil {
		r.engine.AddEvent("database_error", fmt.Sprintf("list events for strategy: %v", err))
		return snapshotEvents
	}
	return events
}

func (r *Runner) watchlistSnapshot() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.watchlist))
	copy(out, r.watchlist)
	return out
}

func (r *Runner) getStore() planStore {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.store
}
