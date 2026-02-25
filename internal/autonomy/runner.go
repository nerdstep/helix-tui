package autonomy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
	"helix-tui/internal/eventmeta"
	"helix-tui/internal/markethours"
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
	lowPower         lowPowerConfig
	powerState       powerState

	eventHistory      eventHistoryStore
	eventHistorySize  int
	strategyPolicy    strategyPolicyProvider
	tradingDayChecker markethours.TradingDayChecker
	nowFn             func() time.Time
}

type contextLogMode string

type eventHistoryStore interface {
	ListRecent(limit int) ([]domain.Event, error)
}

type StrategyConstraint struct {
	Symbol      string
	Bias        string
	MaxNotional float64
}

type LowPowerConfig struct {
	Enabled            bool
	AllowAfterHours    bool
	ClosedPollInterval time.Duration
	PreOpenWarmup      time.Duration
}

type lowPowerConfig struct {
	Enabled            bool
	AllowAfterHours    bool
	ClosedPollInterval time.Duration
	PreOpenWarmup      time.Duration
}

type powerState string

type ActiveStrategyPolicy struct {
	PlanID          uint
	GeneratedAt     time.Time
	Recommendations []StrategyConstraint
}

type strategyPolicyProvider interface {
	GetActiveStrategyPolicy() (*ActiveStrategyPolicy, error)
}

const (
	contextLogOff             contextLogMode = "off"
	contextLogSummary         contextLogMode = "summary"
	contextLogFull            contextLogMode = "full"
	defaultEventHistorySize                  = 200
	defaultClosedPollInterval                = 2 * time.Minute
	defaultPreOpenWarmup                     = 15 * time.Minute

	powerStateActive powerState = "active"
	powerStateWarmup powerState = "warmup"
	powerStateIdle   powerState = "idle"
)

var errEquivalentOpenOrder = errors.New("equivalent open order already exists")

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
		lowPower: lowPowerConfig{
			Enabled: false,
		},
		eventHistorySize: defaultEventHistorySize,
		nowFn:            time.Now,
	}
}

func (r *Runner) SetLowPower(cfg LowPowerConfig) {
	r.mu.Lock()
	r.lowPower = normalizeLowPowerConfig(cfg)
	r.mu.Unlock()
}

func (r *Runner) SetEventHistory(store eventHistoryStore) {
	r.mu.Lock()
	r.eventHistory = store
	r.mu.Unlock()
}

func (r *Runner) SetStrategyPolicyProvider(provider strategyPolicyProvider) {
	r.mu.Lock()
	r.strategyPolicy = provider
	r.mu.Unlock()
}

func (r *Runner) SetTradingDayChecker(checker markethours.TradingDayChecker) {
	r.mu.Lock()
	r.tradingDayChecker = checker
	r.mu.Unlock()
}

func (r *Runner) tradingDayCheckerSnapshot() markethours.TradingDayChecker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tradingDayChecker
}

func (r *Runner) Run(ctx context.Context) error {
	if r.agent == nil {
		return fmt.Errorf("runner requires an agent")
	}
	lowPower := r.lowPowerSnapshot()
	r.engine.AddEvent(
		"agent_runner_start",
		fmt.Sprintf(
			"mode=%s interval=%s low_power=%t closed_poll=%s after_hours=%t warmup=%s",
			r.mode,
			r.interval,
			lowPower.Enabled,
			lowPower.ClosedPollInterval,
			lowPower.AllowAfterHours,
			lowPower.PreOpenWarmup,
		),
	)
	if err := r.runCycle(ctx); err != nil {
		r.engine.AddEvent("agent_cycle_error", err.Error())
	}

	timer := time.NewTimer(r.nextCycleInterval(r.currentTime()))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			r.engine.AddEvent("agent_runner_stop", "context canceled")
			return ctx.Err()
		case <-timer.C:
			if err := r.runCycle(ctx); err != nil {
				r.engine.AddEvent("agent_cycle_error", err.Error())
			}
			timer.Reset(r.nextCycleInterval(r.currentTime()))
		}
	}
}

func (r *Runner) runCycle(ctx context.Context) error {
	cycleStartedAt := r.currentTime()
	watchlist := r.watchlistSnapshot()
	state, reason := r.powerStateForTime(cycleStartedAt)
	r.transitionPowerState(state, reason)
	r.engine.AddEvent(
		"agent_cycle_start",
		fmt.Sprintf("mode=%s watchlist=%d power_state=%s", r.mode, len(watchlist), state),
	)

	syncCtx, cancel := context.WithTimeout(ctx, r.syncTimeout)
	defer cancel()
	if err := r.engine.SyncQuiet(syncCtx); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if state == powerStateIdle {
		r.engine.AddEvent(
			"agent_cycle_idle",
			fmt.Sprintf("state=%s reason=%s", state, reason),
		)
		r.engine.AddEvent("agent_cycle_complete", "generated=0 attempted=0 executed=0 rejected=0 approvals=0 dry_run=0 skipped=0 reason=low_power_state")
		r.recordHeartbeat(0, 0, 0, 0, 0, 0, 0)
		return nil
	}

	snapshot := r.engine.Snapshot()
	snapshot.Events = r.eventsForAgent(snapshot.Events)
	quotes, quoteErrors := r.collectQuotes(syncCtx, watchlist)
	input := domain.AgentInput{
		Mode:        r.mode,
		Watchlist:   watchlist,
		Snapshot:    snapshot,
		Compliance:  r.complianceStatusSnapshot(),
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
			if errors.Is(err, errEquivalentOpenOrder) {
				skipped++
				r.engine.AddEvent("agent_intent_skipped", fmt.Sprintf("auto mode: %s reason=equivalent_open_order", summarizeIntent(intent)))
				continue
			}
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
	if err := r.enforceStrategyPolicy(ctx, intent); err != nil {
		return err
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
		if err := r.cancelOpenOrdersForIntent(execCtx, intent); err != nil {
			return err
		}
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

func (r *Runner) enforceStrategyPolicy(ctx context.Context, intent domain.TradeIntent) error {
	provider := r.getStrategyPolicyProvider()
	if provider == nil {
		return nil
	}
	policy, err := provider.GetActiveStrategyPolicy()
	if err != nil {
		return fmt.Errorf("strategy policy lookup: %w", err)
	}
	if policy == nil {
		return fmt.Errorf("strategy policy requires an active strategy plan")
	}
	constraint, ok := matchStrategyConstraint(policy, intent.Symbol)
	if !ok {
		return fmt.Errorf("strategy policy plan=%d has no recommendation for symbol %s", policy.PlanID, intent.Symbol)
	}
	bias := strings.ToLower(strings.TrimSpace(constraint.Bias))
	switch intent.Side {
	case domain.SideBuy:
		if bias != "buy" {
			return fmt.Errorf("strategy policy plan=%d rejects buy %s (bias=%s)", policy.PlanID, intent.Symbol, bias)
		}
	case domain.SideSell:
		if bias != "sell" {
			return fmt.Errorf("strategy policy plan=%d rejects sell %s (bias=%s)", policy.PlanID, intent.Symbol, bias)
		}
	default:
		return fmt.Errorf("strategy policy plan=%d unsupported side %s", policy.PlanID, intent.Side)
	}
	if constraint.MaxNotional > 0 {
		referencePrice, err := intentReferencePrice(ctx, r.engine, intent)
		if err != nil {
			return fmt.Errorf("strategy policy price lookup: %w", err)
		}
		notional := intent.Qty * referencePrice
		if notional > constraint.MaxNotional {
			return fmt.Errorf(
				"strategy policy plan=%d notional %.2f exceeds max %.2f for %s",
				policy.PlanID,
				notional,
				constraint.MaxNotional,
				intent.Symbol,
			)
		}
	}
	return nil
}

func matchStrategyConstraint(policy *ActiveStrategyPolicy, symbol string) (StrategyConstraint, bool) {
	if policy == nil {
		return StrategyConstraint{}, false
	}
	for _, rec := range policy.Recommendations {
		if strings.EqualFold(strings.TrimSpace(rec.Symbol), strings.TrimSpace(symbol)) {
			return rec, true
		}
	}
	return StrategyConstraint{}, false
}

func intentReferencePrice(ctx context.Context, eng *engine.Engine, intent domain.TradeIntent) (float64, error) {
	if intent.OrderType == domain.OrderTypeLimit && intent.LimitPrice != nil && *intent.LimitPrice > 0 {
		return *intent.LimitPrice, nil
	}
	quote, err := eng.GetQuote(ctx, intent.Symbol)
	if err != nil {
		return 0, err
	}
	switch intent.Side {
	case domain.SideBuy:
		if quote.Ask > 0 {
			return quote.Ask, nil
		}
		if quote.Last > 0 {
			return quote.Last, nil
		}
		if quote.Bid > 0 {
			return quote.Bid, nil
		}
	case domain.SideSell:
		if quote.Bid > 0 {
			return quote.Bid, nil
		}
		if quote.Last > 0 {
			return quote.Last, nil
		}
		if quote.Ask > 0 {
			return quote.Ask, nil
		}
	}
	return 0, fmt.Errorf("no reference quote for %s", intent.Symbol)
}

func (r *Runner) getStrategyPolicyProvider() strategyPolicyProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.strategyPolicy
}

func (r *Runner) cancelOpenOrdersForIntent(ctx context.Context, intent domain.TradeIntent) error {
	snapshot := r.engine.Snapshot()
	canceled := 0
	foundEquivalent := false
	for _, order := range snapshot.Orders {
		if !isActiveOpenOrder(order.Status) {
			continue
		}
		if !strings.EqualFold(order.Symbol, intent.Symbol) {
			continue
		}
		if order.Side != intent.Side {
			continue
		}
		if isEquivalentOpenOrder(order, intent) {
			foundEquivalent = true
			continue
		}
		if err := r.engine.CancelOrder(ctx, order.ID); err != nil {
			if isIgnorableCancelError(err) {
				continue
			}
			return fmt.Errorf("cancel existing open order %s: %w", order.ID, err)
		}
		canceled++
	}
	if canceled > 0 {
		r.engine.AddEvent(
			"agent_open_orders_replaced",
			fmt.Sprintf("symbol=%s side=%s canceled=%d", intent.Symbol, intent.Side, canceled),
		)
	}
	if foundEquivalent {
		return errEquivalentOpenOrder
	}
	return nil
}

func isActiveOpenOrder(status domain.OrderStatus) bool {
	switch status {
	case domain.OrderStatusNew, domain.OrderStatusAccepted, domain.OrderStatusPartially:
		return true
	default:
		return false
	}
}

func isIgnorableCancelError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "already filled")
}

func isEquivalentOpenOrder(order domain.Order, intent domain.TradeIntent) bool {
	if !strings.EqualFold(order.Symbol, intent.Symbol) || order.Side != intent.Side || order.Type != intent.OrderType {
		return false
	}
	if !withinQtyTolerance(order.Qty, intent.Qty) {
		return false
	}
	if intent.OrderType != domain.OrderTypeLimit {
		return true
	}
	if order.LimitPrice == nil || intent.LimitPrice == nil || *order.LimitPrice <= 0 || *intent.LimitPrice <= 0 {
		return false
	}
	return withinLimitPriceTolerance(*order.LimitPrice, *intent.LimitPrice)
}

func withinQtyTolerance(a, b float64) bool {
	diff := math.Abs(a - b)
	tol := math.Max(0.01, math.Abs(b)*0.005) // 0.5% or 0.01 share minimum
	return diff <= tol
}

func withinLimitPriceTolerance(a, b float64) bool {
	diff := math.Abs(a - b)
	tol := math.Max(0.01, math.Abs(b)*0.0025) // 0.25% or 1 cent minimum
	return diff <= tol
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
	Mode        domain.Mode              `json:"mode"`
	Watchlist   []string                 `json:"watchlist"`
	Account     domain.Account           `json:"account"`
	Compliance  *domain.ComplianceStatus `json:"compliance,omitempty"`
	Positions   []domain.Position        `json:"positions"`
	OpenOrders  []decisionOrderInput     `json:"open_orders"`
	Quotes      []domain.Quote           `json:"quotes"`
	QuoteErrors []string                 `json:"quote_errors"`
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
		Compliance:  complianceForDecisionDigest(input.Compliance),
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

func (r *Runner) complianceStatusSnapshot() *domain.ComplianceStatus {
	if r == nil || r.engine == nil {
		return nil
	}
	status, ok := r.engine.ComplianceStatus()
	if !ok || status == nil {
		return nil
	}
	return cloneComplianceStatus(status)
}

func cloneComplianceStatus(in *domain.ComplianceStatus) *domain.ComplianceStatus {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func complianceForDecisionDigest(in *domain.ComplianceStatus) *domain.ComplianceStatus {
	out := cloneComplianceStatus(in)
	if out == nil {
		return nil
	}
	// Timestamp churn is not a material decision input.
	out.LastReconciledAt = time.Time{}
	return out
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

func (r *Runner) currentTime() time.Time {
	if r.nowFn == nil {
		return time.Now()
	}
	return r.nowFn()
}

func normalizeLowPowerConfig(cfg LowPowerConfig) lowPowerConfig {
	closedPoll := cfg.ClosedPollInterval
	if closedPoll <= 0 {
		closedPoll = defaultClosedPollInterval
	}
	preOpenWarmup := cfg.PreOpenWarmup
	if preOpenWarmup < 0 {
		preOpenWarmup = 0
	}
	if preOpenWarmup == 0 && cfg.Enabled {
		preOpenWarmup = defaultPreOpenWarmup
	}
	return lowPowerConfig{
		Enabled:            cfg.Enabled,
		AllowAfterHours:    cfg.AllowAfterHours,
		ClosedPollInterval: closedPoll,
		PreOpenWarmup:      preOpenWarmup,
	}
}

func (r *Runner) lowPowerSnapshot() lowPowerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lowPower
}

func (r *Runner) powerStateSnapshot() powerState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.powerState
}

func (r *Runner) nextCycleInterval(now time.Time) time.Duration {
	state := r.powerStateSnapshot()
	if state == "" {
		computed, _ := r.powerStateForTime(now)
		state = computed
	}
	return r.intervalForPowerState(state)
}

func (r *Runner) intervalForPowerState(state powerState) time.Duration {
	lowPower := r.lowPowerSnapshot()
	if state != powerStateIdle || !lowPower.Enabled {
		return r.interval
	}
	if lowPower.ClosedPollInterval > r.interval {
		return lowPower.ClosedPollInterval
	}
	return r.interval
}

func (r *Runner) transitionPowerState(next powerState, reason string) {
	r.mu.Lock()
	prev := r.powerState
	if prev == next {
		r.mu.Unlock()
		return
	}
	r.powerState = next
	r.mu.Unlock()
	if next == "" {
		return
	}
	details := eventmeta.EncodeAgentPowerState(eventmeta.AgentPowerState{
		State:        string(next),
		Prev:         string(normalizePowerState(prev)),
		Reason:       normalizePowerReason(reason),
		NextInterval: r.intervalForPowerState(next).Round(time.Second).String(),
	})
	r.engine.AddEvent("agent_power_state", details)
}

func normalizePowerState(state powerState) powerState {
	if state == "" {
		return "unknown"
	}
	return state
}

func normalizePowerReason(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return "unspecified"
	}
	return strings.ReplaceAll(reason, " ", "_")
}

func (r *Runner) powerStateForTime(now time.Time) (powerState, string) {
	lowPower := r.lowPowerSnapshot()
	if !lowPower.Enabled {
		return powerStateActive, "disabled"
	}
	checker := r.tradingDayCheckerSnapshot()
	phase := markethours.PhaseAt(now, checker)
	switch phase {
	case markethours.PhaseRegular:
		return powerStateActive, "market_open"
	case markethours.PhasePremarket:
		if lowPower.AllowAfterHours {
			return powerStateActive, "after_hours_allowed"
		}
		if lowPower.PreOpenWarmup > 0 && markethours.InPreOpenWarmup(now, lowPower.PreOpenWarmup, checker) {
			return powerStateWarmup, "pre_open_warmup"
		}
		return powerStateIdle, "outside_market_hours"
	case markethours.PhaseAfterHours:
		if lowPower.AllowAfterHours {
			return powerStateActive, "after_hours_allowed"
		}
		return powerStateIdle, "outside_market_hours"
	default:
		return powerStateIdle, "outside_market_hours"
	}
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
