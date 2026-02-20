package autonomy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
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
	minGainPct   float64
	dryRun       bool

	heartbeatInterval    time.Duration
	heartbeatWindowStart time.Time
	heartbeat            heartbeatStats

	lastDecisionHash string
	lastDecisionAt   time.Time
	forceInvokeAfter time.Duration
	contextLogMode   contextLogMode

	eventHistory     eventHistoryStore
	eventHistorySize int
}

type contextLogMode string

type eventHistoryStore interface {
	ListRecent(limit int) ([]domain.Event, error)
}

const (
	contextLogOff           contextLogMode = "off"
	contextLogSummary       contextLogMode = "summary"
	contextLogFull          contextLogMode = "full"
	defaultEventHistorySize                = 200
)

type heartbeatStats struct {
	cycles         int
	idleCycles     int
	generated      int
	executed       int
	rejected       int
	approvals      int
	dryRun         int
	skipped        int
	totalLatencyMs int64
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
	minGainPct float64,
	dryRun bool,
	contextLog string,
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
	if minGainPct < 0 {
		minGainPct = 0
	}
	return &Runner{
		engine:            engine,
		agent:             agent,
		mode:              mode,
		watchlist:         watchlist,
		interval:          interval,
		syncTimeout:       syncTimeout,
		orderTimeout:      orderTimeout,
		maxPerCycle:       maxPerCycle,
		minGainPct:        minGainPct,
		dryRun:            dryRun,
		heartbeatInterval: heartbeatIntervalForCycle(interval),
		forceInvokeAfter:  forceInvokeAfterForCycle(interval),
		contextLogMode:    normalizedContextLogMode(contextLog),
		eventHistorySize:  defaultEventHistorySize,
	}
}

func (r *Runner) SetEventHistory(store eventHistoryStore) {
	r.mu.Lock()
	r.eventHistory = store
	r.mu.Unlock()
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
	snapshot.Events = r.eventsForAgent(snapshot.Events)
	quotes, quoteErrors := r.collectQuotes(syncCtx, watchlist)
	input := domain.AgentInput{
		Mode:        r.mode,
		Watchlist:   watchlist,
		Snapshot:    snapshot,
		Quotes:      quotes,
		QuoteErrors: quoteErrors,
	}
	contextHash, payloadBytes, err := hashDecisionContext(input)
	if err != nil {
		return fmt.Errorf("hash decision context: %w", err)
	}
	if r.shouldSkipAgentCall(contextHash, cycleStartedAt) {
		r.logContext(input, contextHash, payloadBytes, false)
		r.engine.AddEvent(
			"agent_context_unchanged",
			fmt.Sprintf(
				"hash=%s age=%s force_after=%s",
				shortHash(contextHash),
				time.Since(r.lastDecisionAt).Round(time.Second),
				r.forceInvokeAfter.Round(time.Second),
			),
		)
		r.engine.AddEvent("agent_cycle_complete", "generated=0 attempted=0 executed=0 rejected=0 approvals=0 dry_run=0 skipped=0 reason=context_unchanged")
		r.recordHeartbeat(0, 0, 0, 0, 0, 0, 0)
		return nil
	}
	r.logContext(input, contextHash, payloadBytes, true)
	r.engine.AddEvent("agent_context_changed", fmt.Sprintf("hash=%s", shortHash(contextHash)))

	intents, err := r.agent.ProposeTrades(ctx, input)
	if err != nil {
		return fmt.Errorf("propose trades: %w", err)
	}
	r.lastDecisionHash = contextHash
	r.lastDecisionAt = cycleStartedAt
	proposalLatencyMs := time.Since(cycleStartedAt).Milliseconds()
	generated := len(intents)
	r.engine.AddEvent(
		"agent_proposal",
		fmt.Sprintf(
			"generated=%d watchlist=%d latency_ms=%d",
			generated,
			len(watchlist),
			proposalLatencyMs,
		),
	)
	if len(intents) == 0 {
		r.engine.AddEvent("agent_cycle_complete", "generated=0 attempted=0 executed=0 rejected=0 approvals=0 dry_run=0 skipped=0")
		r.recordHeartbeat(proposalLatencyMs, generated, 0, 0, 0, 0, 0)
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
			r.engine.AddStructuredEvent(domain.Event{
				Type:            "agent_intent_rejected",
				Details:         summarizeRejectedIntent(intent),
				RejectionReason: strings.TrimSpace(err.Error()),
			})
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
	r.recordHeartbeat(proposalLatencyMs, generated, executed, rejected, approvals, dryRun, skipped)
	return nil
}

func (r *Runner) eventsForAgent(snapshotEvents []domain.Event) []domain.Event {
	r.mu.RLock()
	store := r.eventHistory
	limit := r.eventHistorySize
	r.mu.RUnlock()
	if store == nil {
		return snapshotEvents
	}
	events, err := store.ListRecent(limit)
	if err != nil {
		r.engine.AddEvent("database_error", fmt.Sprintf("list trade events: %v", err))
		return snapshotEvents
	}
	return events
}

func (r *Runner) handleIntent(ctx context.Context, intent domain.TradeIntent) error {
	if intent.Qty <= 0 {
		return fmt.Errorf("intent qty must be > 0")
	}
	intent.Symbol = strings.ToUpper(strings.TrimSpace(intent.Symbol))
	if intent.Symbol == "" {
		return fmt.Errorf("intent symbol is required")
	}
	switch intent.OrderType {
	case "", domain.OrderTypeMarket:
		intent.OrderType = domain.OrderTypeMarket
		intent.LimitPrice = nil
	case domain.OrderTypeLimit:
		if intent.LimitPrice == nil || *intent.LimitPrice <= 0 {
			return fmt.Errorf("limit order requires positive limit price")
		}
	default:
		intent.OrderType = domain.OrderTypeMarket
		intent.LimitPrice = nil
	}
	if err := r.enforceMinExpectedGain(ctx, intent); err != nil {
		return err
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
		"%s %s qty=%.2f type=%s conf=%.2f gain=%.2f%% rationale=%s",
		i.Side,
		i.Symbol,
		i.Qty,
		i.OrderType,
		i.Confidence,
		i.ExpectedGainPct,
		strings.TrimSpace(i.Rationale),
	)
}

func summarizeRejectedIntent(i domain.TradeIntent) string {
	return fmt.Sprintf(
		"%s %s qty=%.2f type=%s conf=%.2f gain=%.2f%%",
		i.Side,
		i.Symbol,
		i.Qty,
		i.OrderType,
		i.Confidence,
		i.ExpectedGainPct,
	)
}

type decisionContextDigest struct {
	Mode        domain.Mode          `json:"mode"`
	Watchlist   []string             `json:"watchlist"`
	Account     domain.Account       `json:"account"`
	Positions   []domain.Position    `json:"positions"`
	OpenOrders  []decisionOrderInput `json:"open_orders"`
	Quotes      []domain.Quote       `json:"quotes"`
	QuoteErrors []string             `json:"quote_errors"`
}

type decisionOrderInput struct {
	Symbol string             `json:"symbol"`
	Side   domain.Side        `json:"side"`
	Qty    float64            `json:"qty"`
	Status domain.OrderStatus `json:"status"`
	Type   domain.OrderType   `json:"type"`
}

func hashDecisionContext(input domain.AgentInput) (string, int, error) {
	digest := decisionContextDigest{
		Mode:        input.Mode,
		Watchlist:   append([]string{}, input.Watchlist...),
		Account:     input.Snapshot.Account,
		Positions:   append([]domain.Position{}, input.Snapshot.Positions...),
		OpenOrders:  make([]decisionOrderInput, 0, len(input.Snapshot.Orders)),
		Quotes:      append([]domain.Quote{}, input.Quotes...),
		QuoteErrors: append([]string{}, input.QuoteErrors...),
	}
	for _, o := range input.Snapshot.Orders {
		digest.OpenOrders = append(digest.OpenOrders, decisionOrderInput{
			Symbol: strings.ToUpper(strings.TrimSpace(o.Symbol)),
			Side:   o.Side,
			Qty:    o.Qty,
			Status: o.Status,
			Type:   o.Type,
		})
	}
	sort.Strings(digest.Watchlist)
	sort.Slice(digest.Positions, func(i, j int) bool {
		return digest.Positions[i].Symbol < digest.Positions[j].Symbol
	})
	sort.Slice(digest.OpenOrders, func(i, j int) bool {
		if digest.OpenOrders[i].Symbol == digest.OpenOrders[j].Symbol {
			if digest.OpenOrders[i].Side == digest.OpenOrders[j].Side {
				return digest.OpenOrders[i].Qty < digest.OpenOrders[j].Qty
			}
			return digest.OpenOrders[i].Side < digest.OpenOrders[j].Side
		}
		return digest.OpenOrders[i].Symbol < digest.OpenOrders[j].Symbol
	})
	sort.Slice(digest.Quotes, func(i, j int) bool {
		return digest.Quotes[i].Symbol < digest.Quotes[j].Symbol
	})
	sort.Strings(digest.QuoteErrors)

	payload, err := json.Marshal(digest)
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), len(payload), nil
}

func (r *Runner) collectQuotes(ctx context.Context, watchlist []string) ([]domain.Quote, []string) {
	quotes := make([]domain.Quote, 0, len(watchlist))
	quoteErrors := make([]string, 0)
	for _, symbol := range watchlist {
		q, err := r.engine.GetQuote(ctx, symbol)
		if err != nil {
			quoteErrors = append(quoteErrors, fmt.Sprintf("%s: %v", symbol, err))
			continue
		}
		quotes = append(quotes, q)
	}
	return quotes, quoteErrors
}

func (r *Runner) shouldSkipAgentCall(contextHash string, now time.Time) bool {
	if contextHash == "" {
		return false
	}
	if r.lastDecisionHash == "" {
		return false
	}
	if r.lastDecisionHash != contextHash {
		return false
	}
	if r.forceInvokeAfter <= 0 {
		return true
	}
	if r.lastDecisionAt.IsZero() {
		return false
	}
	return now.Sub(r.lastDecisionAt) < r.forceInvokeAfter
}

func (r *Runner) logContext(input domain.AgentInput, contextHash string, payloadBytes int, changed bool) {
	if r.contextLogMode == contextLogOff {
		return
	}
	action := "unchanged"
	if changed {
		action = "changed"
	}
	r.engine.AddEvent(
		"agent_context_summary",
		fmt.Sprintf(
			"hash=%s action=%s payload_bytes=%d watchlist=%d positions=%d open_orders=%d quotes=%d quote_errors=%d",
			shortHash(contextHash),
			action,
			payloadBytes,
			len(input.Watchlist),
			len(input.Snapshot.Positions),
			len(input.Snapshot.Orders),
			len(input.Quotes),
			len(input.QuoteErrors),
		),
	)
	if r.contextLogMode != contextLogFull {
		return
	}
	payload, err := json.Marshal(input)
	if err != nil {
		r.engine.AddEvent("agent_context_payload_error", err.Error())
		return
	}
	r.engine.AddEvent("agent_context_payload", string(payload))
}

func shortHash(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}

func heartbeatIntervalForCycle(cycleInterval time.Duration) time.Duration {
	const minHeartbeat = 30 * time.Second
	derived := cycleInterval * 6
	if derived > minHeartbeat {
		return derived
	}
	return minHeartbeat
}

func forceInvokeAfterForCycle(cycleInterval time.Duration) time.Duration {
	const minForceInvoke = 2 * time.Minute
	derived := cycleInterval * 12
	if derived > minForceInvoke {
		return derived
	}
	return minForceInvoke
}

func normalizedContextLogMode(raw string) contextLogMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(contextLogOff):
		return contextLogOff
	case string(contextLogSummary):
		return contextLogSummary
	case string(contextLogFull):
		return contextLogFull
	default:
		return contextLogOff
	}
}

func (r *Runner) recordHeartbeat(proposalLatencyMs int64, generated, executed, rejected, approvals, dryRun, skipped int) {
	if r.heartbeatInterval <= 0 {
		return
	}
	now := time.Now().UTC()
	if r.heartbeatWindowStart.IsZero() {
		r.heartbeatWindowStart = now
	}
	r.heartbeat.cycles++
	if generated == 0 {
		r.heartbeat.idleCycles++
	}
	r.heartbeat.generated += generated
	r.heartbeat.executed += executed
	r.heartbeat.rejected += rejected
	r.heartbeat.approvals += approvals
	r.heartbeat.dryRun += dryRun
	r.heartbeat.skipped += skipped
	r.heartbeat.totalLatencyMs += proposalLatencyMs

	if now.Sub(r.heartbeatWindowStart) < r.heartbeatInterval {
		return
	}
	avgLatencyMs := int64(0)
	if r.heartbeat.cycles > 0 {
		avgLatencyMs = r.heartbeat.totalLatencyMs / int64(r.heartbeat.cycles)
	}
	r.engine.AddEvent(
		"agent_heartbeat",
		fmt.Sprintf(
			"window=%s cycles=%d idle=%d generated=%d executed=%d rejected=%d approvals=%d dry_run=%d skipped=%d avg_latency_ms=%d",
			r.heartbeatInterval.Round(time.Second),
			r.heartbeat.cycles,
			r.heartbeat.idleCycles,
			r.heartbeat.generated,
			r.heartbeat.executed,
			r.heartbeat.rejected,
			r.heartbeat.approvals,
			r.heartbeat.dryRun,
			r.heartbeat.skipped,
			avgLatencyMs,
		),
	)
	r.heartbeatWindowStart = now
	r.heartbeat = heartbeatStats{}
}

func (r *Runner) enforceMinExpectedGain(ctx context.Context, intent domain.TradeIntent) error {
	if r.minGainPct <= 0 {
		return nil
	}
	gainPct, source, err := r.resolveExpectedGainPct(ctx, intent)
	if err != nil {
		return err
	}
	if gainPct < r.minGainPct {
		return fmt.Errorf("expected gain %.2f%% below minimum %.2f%% (%s)", gainPct, r.minGainPct, source)
	}
	return nil
}

func (r *Runner) resolveExpectedGainPct(ctx context.Context, intent domain.TradeIntent) (float64, string, error) {
	if intent.ExpectedGainPct > 0 {
		return intent.ExpectedGainPct, "intent", nil
	}
	if intent.Side != domain.SideSell {
		return 0, "intent", fmt.Errorf("expected gain missing for %s intent", intent.Side)
	}
	snapshot := r.engine.Snapshot()
	avgCost := 0.0
	for _, pos := range snapshot.Positions {
		if strings.EqualFold(pos.Symbol, intent.Symbol) {
			avgCost = pos.AvgCost
			break
		}
	}
	if avgCost <= 0 {
		return 0, "position", fmt.Errorf("expected gain missing: no avg cost for %s", intent.Symbol)
	}

	exitPrice := 0.0
	if intent.OrderType == domain.OrderTypeLimit && intent.LimitPrice != nil {
		exitPrice = *intent.LimitPrice
	} else {
		quote, err := r.engine.GetQuote(ctx, intent.Symbol)
		if err != nil {
			return 0, "quote", fmt.Errorf("get quote for gain check: %w", err)
		}
		if quote.Bid > 0 {
			exitPrice = quote.Bid
		} else if quote.Last > 0 {
			exitPrice = quote.Last
		} else {
			exitPrice = quote.Ask
		}
	}
	if exitPrice <= 0 {
		return 0, "quote", fmt.Errorf("expected gain missing: no reference price for %s", intent.Symbol)
	}

	return ((exitPrice - avgCost) / avgCost) * 100, "position", nil
}
