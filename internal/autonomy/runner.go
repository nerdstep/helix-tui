package autonomy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
	"helix-tui/internal/symbols"
)

type Runner struct {
	engine       *engine.Engine
	agent        domain.Agent
	mode         domain.Mode
	watchlist    []string
	mu           sync.RWMutex
	interval     time.Duration
	syncTimeout  time.Duration
	orderTimeout time.Duration
	maxPerCycle  int
	dryRun       bool
	objective    string
}

func (r *Runner) SetWatchlist(nextWatchlist []string) {
	r.mu.Lock()
	r.watchlist = symbols.Normalize(nextWatchlist)
	r.mu.Unlock()
}

func (r *Runner) watchlistSnapshot() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.watchlist))
	copy(out, r.watchlist)
	return out
}

func NewRunner(
	engine *engine.Engine,
	agent domain.Agent,
	mode domain.Mode,
	watchlist []string,
	interval time.Duration,
	syncTimeout time.Duration,
	orderTimeout time.Duration,
	maxPerCycle int,
	dryRun bool,
	objective string,
) *Runner {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if syncTimeout <= 0 {
		syncTimeout = 15 * time.Second
	}
	if orderTimeout <= 0 {
		orderTimeout = 15 * time.Second
	}
	if maxPerCycle <= 0 {
		maxPerCycle = 1
	}
	return &Runner{
		engine:       engine,
		agent:        agent,
		mode:         mode,
		watchlist:    watchlist,
		interval:     interval,
		syncTimeout:  syncTimeout,
		orderTimeout: orderTimeout,
		maxPerCycle:  maxPerCycle,
		dryRun:       dryRun,
		objective:    strings.TrimSpace(objective),
	}
}

func (r *Runner) Run(ctx context.Context) error {
	if r.agent == nil {
		return fmt.Errorf("runner requires an agent")
	}
	r.engine.AddEvent("agent_runner_start", fmt.Sprintf("mode=%s interval=%s", r.mode, r.interval))
	if err := r.runCycle(ctx); err != nil {
		r.engine.AddEvent("agent_cycle_error", err.Error())
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.engine.AddEvent("agent_runner_stop", "context canceled")
			return ctx.Err()
		case <-ticker.C:
			if err := r.runCycle(ctx); err != nil {
				r.engine.AddEvent("agent_cycle_error", err.Error())
			}
		}
	}
}

func (r *Runner) runCycle(ctx context.Context) error {
	cycleStartedAt := time.Now()
	watchlist := r.watchlistSnapshot()
	r.engine.AddEvent(
		"agent_cycle_start",
		fmt.Sprintf("mode=%s watchlist=%d", r.mode, len(watchlist)),
	)

	syncCtx, cancel := context.WithTimeout(ctx, r.syncTimeout)
	defer cancel()
	if err := r.engine.SyncQuiet(syncCtx); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	snapshot := r.engine.Snapshot()
	intents, err := r.agent.ProposeTrades(ctx, domain.AgentInput{
		Mode:      r.mode,
		Watchlist: watchlist,
		Snapshot:  snapshot,
		Objective: r.objective,
	})
	if err != nil {
		return fmt.Errorf("propose trades: %w", err)
	}
	generated := len(intents)
	r.engine.AddEvent(
		"agent_proposal",
		fmt.Sprintf(
			"generated=%d watchlist=%d latency_ms=%d",
			generated,
			len(watchlist),
			time.Since(cycleStartedAt).Milliseconds(),
		),
	)
	if len(intents) == 0 {
		r.engine.AddEvent("agent_cycle_complete", "generated=0 attempted=0 executed=0 rejected=0 approvals=0 dry_run=0 skipped=0")
		return nil
	}

	if len(intents) > r.maxPerCycle {
		intents = intents[:r.maxPerCycle]
	}

	executed := 0
	rejected := 0
	approvals := 0
	dryRun := 0
	skipped := 0

	for _, intent := range intents {
		if err := r.handleIntent(ctx, intent); err != nil {
			rejected++
			r.engine.AddEvent("agent_intent_rejected", fmt.Sprintf("%s: %v", summarizeIntent(intent), err))
			continue
		}
		switch r.mode {
		case domain.ModeManual:
			skipped++
		case domain.ModeAssist:
			approvals++
		case domain.ModeAuto:
			if r.dryRun {
				dryRun++
			} else {
				executed++
			}
		}
	}
	r.engine.AddEvent(
		"agent_cycle_complete",
		fmt.Sprintf(
			"generated=%d attempted=%d executed=%d rejected=%d approvals=%d dry_run=%d skipped=%d",
			generated,
			len(intents),
			executed,
			rejected,
			approvals,
			dryRun,
			skipped,
		),
	)
	return nil
}

func (r *Runner) handleIntent(ctx context.Context, intent domain.TradeIntent) error {
	if intent.Qty <= 0 {
		return fmt.Errorf("intent qty must be > 0")
	}
	intent.Symbol = strings.ToUpper(strings.TrimSpace(intent.Symbol))
	if intent.Symbol == "" {
		return fmt.Errorf("intent symbol is required")
	}
	if intent.OrderType == "" {
		intent.OrderType = domain.OrderTypeMarket
	}

	switch r.mode {
	case domain.ModeManual:
		r.engine.AddEvent("agent_intent_skipped", fmt.Sprintf("manual mode: %s", summarizeIntent(intent)))
		return nil
	case domain.ModeAssist:
		r.engine.AddEvent("agent_intent_needs_approval", summarizeIntent(intent))
		return nil
	case domain.ModeAuto:
		if r.dryRun {
			r.engine.AddEvent("agent_intent_dry_run", summarizeIntent(intent))
			return nil
		}
		execCtx, cancel := context.WithTimeout(ctx, r.orderTimeout)
		defer cancel()
		_, err := r.engine.PlaceOrder(execCtx, domain.OrderRequest{
			Symbol:     intent.Symbol,
			Side:       intent.Side,
			Qty:        intent.Qty,
			Type:       intent.OrderType,
			LimitPrice: intent.LimitPrice,
		})
		if err != nil {
			return err
		}
		r.engine.AddEvent("agent_intent_executed", summarizeIntent(intent))
		return nil
	default:
		return fmt.Errorf("unknown mode %q", r.mode)
	}
}

func summarizeIntent(i domain.TradeIntent) string {
	return fmt.Sprintf(
		"%s %s qty=%.2f type=%s conf=%.2f rationale=%s",
		i.Side,
		i.Symbol,
		i.Qty,
		i.OrderType,
		i.Confidence,
		strings.TrimSpace(i.Rationale),
	)
}
