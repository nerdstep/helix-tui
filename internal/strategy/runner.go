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
	GetActivePlan() (*storage.StrategyPlanWithRecommendations, error)
	GetLatestPlan() (*storage.StrategyPlanWithRecommendations, error)
}

type eventHistoryStore interface {
	ListRecent(limit int) ([]domain.Event, error)
}

const defaultPromptVersion = "strategy-v1"

type Runner struct {
	engine      *engine.Engine
	analyst     Analyst
	store       planStore
	watchlist   []string
	interval    time.Duration
	syncTimeout time.Duration

	maxRecommendations int
	autoActivate       bool
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
	model string,
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
		model:              strings.TrimSpace(model),
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
	if err := r.runCycle(ctx, false, "startup"); err != nil {
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
			if err := r.runCycle(ctx, false, "interval"); err != nil {
				r.engine.AddEvent("strategy_cycle_error", err.Error())
			}
		case <-r.triggerCh:
			if err := r.runCycle(ctx, true, "manual"); err != nil {
				r.engine.AddEvent("strategy_cycle_error", err.Error())
			}
		}
	}
}

func (r *Runner) runCycle(ctx context.Context, force bool, reason string) error {
	store := r.getStore()
	if store == nil {
		return fmt.Errorf("strategy store not configured")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unknown"
	}

	latestPlan, err := store.GetLatestPlan()
	if err != nil {
		return fmt.Errorf("load latest strategy plan: %w", err)
	}
	if !force && latestPlan != nil {
		age := time.Since(latestPlan.Plan.GeneratedAt)
		if age < 0 {
			age = 0
		}
		if age < r.interval {
			r.engine.AddEvent(
				"strategy_cycle_skipped",
				fmt.Sprintf(
					"reason=fresh_plan trigger=%s id=%d status=%s age=%s interval=%s",
					reason,
					latestPlan.Plan.ID,
					latestPlan.Plan.Status,
					age.Round(time.Second),
					r.interval,
				),
			)
			return nil
		}
	}

	currentPlan, err := r.resolveCurrentPlan(store, latestPlan)
	if err != nil {
		return fmt.Errorf("load current strategy plan: %w", err)
	}

	watchlist := r.watchlistSnapshot()
	r.engine.AddEvent("strategy_cycle_start", fmt.Sprintf("watchlist=%d trigger=%s force=%t", len(watchlist), reason, force))

	syncCtx, cancel := context.WithTimeout(ctx, r.syncTimeout)
	defer cancel()
	if err := r.engine.SyncQuiet(syncCtx); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	snapshot := r.engine.Snapshot()
	input := Input{
		GeneratedAt:        time.Now().UTC(),
		MaxRecommendations: r.maxRecommendations,
		Watchlist:          watchlist,
		CurrentPlan:        toCurrentPlanInput(currentPlan),
		Snapshot:           snapshot,
		Quotes:             r.collectQuotes(syncCtx, watchlist),
		RecentEvents:       r.recentEvents(snapshot.Events),
	}
	plan, err := r.analyst.BuildPlan(ctx, input)
	if err != nil {
		return fmt.Errorf("build strategy plan: %w", err)
	}
	if plan.NoChange {
		if currentPlan == nil {
			r.engine.AddEvent("strategy_plan_empty", "analyst returned no_change but no existing plan")
			return nil
		}
		msg := fmt.Sprintf("id=%d status=%s", currentPlan.Plan.ID, currentPlan.Plan.Status)
		if reason := strings.TrimSpace(plan.Summary); reason != "" {
			msg += " reason=" + reason
		}
		r.engine.AddEvent("strategy_plan_unchanged", msg)
		return nil
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
		PromptVersion: defaultPromptVersion,
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

func (r *Runner) resolveCurrentPlan(store planStore, latest *storage.StrategyPlanWithRecommendations) (*storage.StrategyPlanWithRecommendations, error) {
	active, err := store.GetActivePlan()
	if err != nil {
		return nil, err
	}
	if active != nil {
		return active, nil
	}
	return latest, nil
}

func toCurrentPlanInput(bundle *storage.StrategyPlanWithRecommendations) *CurrentPlan {
	if bundle == nil {
		return nil
	}
	recs := make([]Recommendation, 0, len(bundle.Recommendations))
	for _, rec := range bundle.Recommendations {
		recs = append(recs, Recommendation{
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
			Priority:     rec.Priority,
		})
	}
	return &CurrentPlan{
		ID:              bundle.Plan.ID,
		GeneratedAt:     bundle.Plan.GeneratedAt,
		Status:          strings.TrimSpace(string(bundle.Plan.Status)),
		Summary:         bundle.Plan.Summary,
		Confidence:      bundle.Plan.Confidence,
		Recommendations: recs,
	}
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
