package autonomy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
)

func TestNewRunnerDefaults(t *testing.T) {
	r := NewRunner(nil, nil, domain.ModeAuto, nil, 0, 0, 0, 0, 0, false, "")
	if r.interval != 10*time.Second {
		t.Fatalf("unexpected default interval: %s", r.interval)
	}
	if r.syncTimeout != 15*time.Second {
		t.Fatalf("unexpected default sync timeout: %s", r.syncTimeout)
	}
	if r.orderTimeout != 15*time.Second {
		t.Fatalf("unexpected default order timeout: %s", r.orderTimeout)
	}
	if r.maxPerCycle != 1 {
		t.Fatalf("unexpected default maxPerCycle: %d", r.maxPerCycle)
	}
	if r.heartbeatInterval <= 0 {
		t.Fatalf("expected heartbeat interval default")
	}
}

func TestRun_NilAgent(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.agent = nil
	err := r.Run(context.Background())
	if err == nil {
		t.Fatalf("expected nil agent error")
	}
	if !strings.Contains(err.Error(), "requires an agent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleIntent_ManualAddsSkippedEvent(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeManual, false)
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "aapl",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err != nil {
		t.Fatalf("handleIntent failed: %v", err)
	}
	if broker.placeCalls != 0 {
		t.Fatalf("expected no place calls, got %d", broker.placeCalls)
	}
	if !hasEventType(r.engine.Snapshot().Events, "agent_intent_skipped") {
		t.Fatalf("expected agent_intent_skipped event")
	}
}

func TestHandleIntent_AssistAddsApprovalEvent(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeAssist, false)
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "aapl",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err != nil {
		t.Fatalf("handleIntent failed: %v", err)
	}
	if broker.placeCalls != 0 {
		t.Fatalf("expected no place calls, got %d", broker.placeCalls)
	}
	if !hasEventType(r.engine.Snapshot().Events, "agent_intent_needs_approval") {
		t.Fatalf("expected agent_intent_needs_approval event")
	}
}

func TestHandleIntent_AutoDryRunAddsEvent(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeAuto, true)
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "aapl",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err != nil {
		t.Fatalf("handleIntent failed: %v", err)
	}
	if broker.placeCalls != 0 {
		t.Fatalf("expected no place calls, got %d", broker.placeCalls)
	}
	if !hasEventType(r.engine.Snapshot().Events, "agent_intent_dry_run") {
		t.Fatalf("expected agent_intent_dry_run event")
	}
}

func TestHandleIntent_AutoExecutesOrder(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeAuto, false)
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "aapl",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err != nil {
		t.Fatalf("handleIntent failed: %v", err)
	}
	if broker.placeCalls != 1 {
		t.Fatalf("expected one place call, got %d", broker.placeCalls)
	}
	if len(broker.placedRequests) != 1 || broker.placedRequests[0].Type != domain.OrderTypeMarket {
		t.Fatalf("expected default market order request, got %#v", broker.placedRequests)
	}

	events := r.engine.Snapshot().Events
	if !hasEventType(events, "order_placed") {
		t.Fatalf("expected order_placed event")
	}
	if !hasEventType(events, "agent_intent_executed") {
		t.Fatalf("expected agent_intent_executed event")
	}
}

func TestHandleIntent_AutoCancelsExistingSameSideOpenOrdersBeforePlacing(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeAuto, false)
	existingLimit := 101.0
	newLimit := 102.0
	broker.openOrders = []domain.Order{
		{
			ID:         "ord-open-1",
			Symbol:     "AAPL",
			Side:       domain.SideBuy,
			Qty:        1,
			Type:       domain.OrderTypeLimit,
			LimitPrice: &existingLimit,
			Status:     domain.OrderStatusNew,
			CreatedAt:  time.Now().UTC().Add(-time.Minute),
			UpdatedAt:  time.Now().UTC().Add(-time.Minute),
		},
	}
	if err := r.engine.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol:     "aapl",
		Side:       domain.SideBuy,
		Qty:        1,
		OrderType:  domain.OrderTypeLimit,
		LimitPrice: &newLimit,
	})
	if err != nil {
		t.Fatalf("handleIntent failed: %v", err)
	}
	if broker.cancelCalls != 1 {
		t.Fatalf("expected one cancel call, got %d", broker.cancelCalls)
	}
	if len(broker.canceledOrderIDs) != 1 || broker.canceledOrderIDs[0] != "ord-open-1" {
		t.Fatalf("expected canceled open order ord-open-1, got %#v", broker.canceledOrderIDs)
	}
	if broker.placeCalls != 1 {
		t.Fatalf("expected one place call, got %d", broker.placeCalls)
	}
	events := r.engine.Snapshot().Events
	if !hasEventType(events, "order_canceled") {
		t.Fatalf("expected order_canceled event")
	}
	if !hasEventType(events, "agent_open_orders_replaced") {
		t.Fatalf("expected agent_open_orders_replaced event")
	}
}

func TestRunCycle_AutoSkipsEquivalentOpenOrder(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeAuto, false)
	limit := 101.00
	broker.openOrders = []domain.Order{
		{
			ID:         "ord-open-1",
			Symbol:     "AAPL",
			Side:       domain.SideBuy,
			Qty:        10,
			Type:       domain.OrderTypeLimit,
			LimitPrice: &limit,
			Status:     domain.OrderStatusNew,
			CreatedAt:  time.Now().UTC().Add(-time.Minute),
			UpdatedAt:  time.Now().UTC().Add(-time.Minute),
		},
	}
	r.watchlist = []string{"AAPL"}
	r.agent = &fakeAgent{
		intents: []domain.TradeIntent{
			{Symbol: "AAPL", Side: domain.SideBuy, Qty: 10, OrderType: domain.OrderTypeLimit, LimitPrice: &limit, Confidence: 0.8},
		},
	}

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	if broker.cancelCalls != 0 {
		t.Fatalf("expected no cancels for equivalent order, got %d", broker.cancelCalls)
	}
	if broker.placeCalls != 0 {
		t.Fatalf("expected no new placement for equivalent order, got %d", broker.placeCalls)
	}
	events := r.engine.Snapshot().Events
	if !hasEventType(events, "agent_intent_skipped") {
		t.Fatalf("expected agent_intent_skipped event")
	}
	if hasEventType(events, "agent_intent_rejected") {
		t.Fatalf("did not expect agent_intent_rejected for equivalent order skip")
	}
	if !hasEventDetailContains(events, "agent_cycle_complete", "skipped=1") {
		t.Fatalf("expected cycle_complete skipped=1, events=%#v", events)
	}
}

func TestHandleIntent_MinGainRejectsMissingExpectedGainForBuy(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.minGainPct = 1
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err == nil {
		t.Fatalf("expected min gain rejection")
	}
	if !strings.Contains(err.Error(), "expected gain missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleIntent_MinGainAllowsWhenIntentProvidesExpectedGain(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeAuto, false)
	r.minGainPct = 1
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol:          "AAPL",
		Side:            domain.SideBuy,
		Qty:             1,
		ExpectedGainPct: 2,
	})
	if err != nil {
		t.Fatalf("handleIntent failed: %v", err)
	}
	if broker.placeCalls != 1 {
		t.Fatalf("expected one place call, got %d", broker.placeCalls)
	}
}

func TestHandleIntent_UnknownMode(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.Mode("mystery"), false)
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err == nil {
		t.Fatalf("expected unknown mode error")
	}
	if !strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleIntent_StrategyPolicyRejectsWithoutActivePlan(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.SetStrategyPolicyProvider(fakeStrategyPolicyProvider{})
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err == nil {
		t.Fatalf("expected strategy policy rejection")
	}
	if !strings.Contains(err.Error(), "requires an active strategy plan") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleIntent_StrategyPolicyRejectsUnknownSymbol(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.SetStrategyPolicyProvider(fakeStrategyPolicyProvider{
		policy: &ActiveStrategyPolicy{
			PlanID: 1,
			Recommendations: []StrategyConstraint{
				{Symbol: "MSFT", Bias: "buy", MaxNotional: 5000},
			},
		},
	})
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err == nil {
		t.Fatalf("expected strategy policy rejection")
	}
	if !strings.Contains(err.Error(), "no recommendation for symbol") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleIntent_StrategyPolicyRejectsSideMismatch(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.SetStrategyPolicyProvider(fakeStrategyPolicyProvider{
		policy: &ActiveStrategyPolicy{
			PlanID: 1,
			Recommendations: []StrategyConstraint{
				{Symbol: "AAPL", Bias: "sell", MaxNotional: 5000},
			},
		},
	})
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err == nil {
		t.Fatalf("expected strategy policy rejection")
	}
	if !strings.Contains(err.Error(), "rejects buy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleIntent_StrategyPolicyRejectsOverNotional(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.SetStrategyPolicyProvider(fakeStrategyPolicyProvider{
		policy: &ActiveStrategyPolicy{
			PlanID: 1,
			Recommendations: []StrategyConstraint{
				{Symbol: "AAPL", Bias: "buy", MaxNotional: 50},
			},
		},
	})
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err == nil {
		t.Fatalf("expected strategy policy rejection")
	}
	if !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleIntent_StrategyPolicyAllowsMatchingIntent(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeAuto, false)
	r.SetStrategyPolicyProvider(fakeStrategyPolicyProvider{
		policy: &ActiveStrategyPolicy{
			PlanID: 1,
			Recommendations: []StrategyConstraint{
				{Symbol: "AAPL", Bias: "buy", MaxNotional: 5000},
			},
		},
	})
	err := r.handleIntent(context.Background(), domain.TradeIntent{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    1,
	})
	if err != nil {
		t.Fatalf("expected intent to pass strategy policy: %v", err)
	}
	if broker.placeCalls != 1 {
		t.Fatalf("expected one placement, got %d", broker.placeCalls)
	}
}

func TestRunCycle_UsesMaxPerCycleAndPassesAgentInput(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeAuto, false)
	agent := &fakeAgent{
		intents: []domain.TradeIntent{
			{Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, OrderType: domain.OrderTypeMarket},
			{Symbol: "MSFT", Side: domain.SideBuy, Qty: 1, OrderType: domain.OrderTypeMarket},
		},
	}
	r.agent = agent
	r.maxPerCycle = 1
	r.watchlist = []string{"AAPL", "MSFT"}

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	if agent.calls != 1 {
		t.Fatalf("expected one agent call, got %d", agent.calls)
	}
	if agent.lastInput.Mode != domain.ModeAuto {
		t.Fatalf("unexpected mode in agent input: %q", agent.lastInput.Mode)
	}
	if len(agent.lastInput.Watchlist) != 2 {
		t.Fatalf("unexpected agent input: %#v", agent.lastInput)
	}
	if broker.placeCalls != 1 {
		t.Fatalf("expected one placement due to maxPerCycle, got %d", broker.placeCalls)
	}
	if !hasEventType(r.engine.Snapshot().Events, "agent_cycle_complete") {
		t.Fatalf("expected agent_cycle_complete event")
	}
}

func TestRunCycle_IntentErrorRecordedAsEvent(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.agent = &fakeAgent{
		intents: []domain.TradeIntent{
			{Symbol: "AAPL", Side: domain.SideBuy, Qty: 0},
		},
	}

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	if !hasEventType(r.engine.Snapshot().Events, "agent_intent_rejected") {
		t.Fatalf("expected agent_intent_rejected event")
	}
	if !hasEventType(r.engine.Snapshot().Events, "agent_cycle_complete") {
		t.Fatalf("expected agent_cycle_complete event")
	}
}

func TestRunCycle_EmitsHeartbeat(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.agent = &fakeAgent{}
	r.heartbeatInterval = time.Second
	r.heartbeatWindowStart = time.Now().Add(-time.Minute)

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	if !hasEventType(r.engine.Snapshot().Events, "agent_heartbeat") {
		t.Fatalf("expected agent_heartbeat event")
	}
}

func TestRunCycle_SyncError(t *testing.T) {
	r, broker := newRunnerTestHarness(domain.ModeAuto, false)
	r.agent = &fakeAgent{}
	broker.accountErr = errors.New("account down")

	err := r.runCycle(context.Background())
	if err == nil {
		t.Fatalf("expected sync error")
	}
	if !strings.Contains(err.Error(), "sync:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCycle_ProposeError(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.agent = &fakeAgent{err: errors.New("proposal failed")}

	err := r.runCycle(context.Background())
	if err == nil {
		t.Fatalf("expected propose error")
	}
	if !strings.Contains(err.Error(), "propose trades:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCycle_SkipsWhenContextUnchanged(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	agent := &fakeAgent{}
	r.agent = agent
	r.forceInvokeAfter = time.Minute

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("first runCycle failed: %v", err)
	}
	r.lastDecisionAt = r.lastDecisionAt.Add(-time.Second)
	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("second runCycle failed: %v", err)
	}
	if agent.calls != 1 {
		t.Fatalf("expected second cycle to skip propose call, got %d calls", agent.calls)
	}
	if !hasEventType(r.engine.Snapshot().Events, "agent_context_unchanged") {
		t.Fatalf("expected agent_context_unchanged event")
	}
}

func TestRunCycle_ForceInvokesAfterWindow(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	agent := &fakeAgent{}
	r.agent = agent
	r.forceInvokeAfter = time.Nanosecond

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("first runCycle failed: %v", err)
	}
	r.lastDecisionAt = r.lastDecisionAt.Add(-time.Second)
	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("second runCycle failed: %v", err)
	}
	if agent.calls != 2 {
		t.Fatalf("expected force window to trigger second propose call, got %d calls", agent.calls)
	}
}

func TestRunCycle_ContextSummaryLog(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.agent = &fakeAgent{}
	r.contextLogMode = contextLogSummary

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	if !hasEventType(r.engine.Snapshot().Events, "agent_context_summary") {
		t.Fatalf("expected agent_context_summary event")
	}
}

func TestSetWatchlist(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.SetWatchlist([]string{"aapl", " AAPL ", "msft"})
	got := r.watchlistSnapshot()
	if len(got) != 2 || got[0] != "AAPL" || got[1] != "MSFT" {
		t.Fatalf("unexpected watchlist snapshot: %#v", got)
	}
}

func TestRunCycle_UsesEventHistoryStoreForAgentContext(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	agent := &fakeAgent{}
	r.agent = agent
	store := &fakeEventHistoryStore{
		listRecentOut: []domain.Event{
			{Time: time.Now().UTC(), Type: "order_placed", Details: "buy AAPL 1"},
		},
	}
	r.SetEventHistory(store)

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	if len(agent.lastInput.Snapshot.Events) != 1 || agent.lastInput.Snapshot.Events[0].Type != "order_placed" {
		t.Fatalf("expected DB-backed events in agent input, got %#v", agent.lastInput.Snapshot.Events)
	}
}

func TestRunCycle_RejectedIntentIncludesRejectionReason(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	r.agent = &fakeAgent{
		intents: []domain.TradeIntent{
			{Symbol: "AAPL", Side: domain.SideBuy, Qty: 0},
		},
	}

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	events := r.engine.Snapshot().Events
	for _, e := range events {
		if e.Type != "agent_intent_rejected" {
			continue
		}
		if strings.TrimSpace(e.RejectionReason) == "" {
			t.Fatalf("expected rejection reason on rejected event: %#v", e)
		}
		if strings.Contains(e.Details, "rationale=") {
			t.Fatalf("did not expect rationale in rejected details: %#v", e.Details)
		}
		return
	}
	t.Fatalf("expected agent_intent_rejected event")
}

type fakeAgent struct {
	intents   []domain.TradeIntent
	err       error
	calls     int
	lastInput domain.AgentInput
}

type fakeEventHistoryStore struct {
	listRecentOut []domain.Event
	listRecentErr error
}

type fakeStrategyPolicyProvider struct {
	policy *ActiveStrategyPolicy
	err    error
}

func (f *fakeEventHistoryStore) ListRecent(_ int) ([]domain.Event, error) {
	if f.listRecentErr != nil {
		return nil, f.listRecentErr
	}
	out := make([]domain.Event, len(f.listRecentOut))
	copy(out, f.listRecentOut)
	return out, nil
}

func (f fakeStrategyPolicyProvider) GetActiveStrategyPolicy() (*ActiveStrategyPolicy, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.policy, nil
}

func (a *fakeAgent) ProposeTrades(_ context.Context, input domain.AgentInput) ([]domain.TradeIntent, error) {
	a.calls++
	a.lastInput = input
	if a.err != nil {
		return nil, a.err
	}
	out := make([]domain.TradeIntent, len(a.intents))
	copy(out, a.intents)
	return out, nil
}

type fakeBroker struct {
	account          domain.Account
	positions        []domain.Position
	openOrders       []domain.Order
	quotes           map[string]domain.Quote
	quoteErr         map[string]error
	accountErr       error
	positionsErr     error
	ordersErr        error
	placeErr         error
	cancelErr        error
	placeCalls       int
	cancelCalls      int
	canceledOrderIDs []string
	placedRequests   []domain.OrderRequest
}

func newRunnerTestHarness(mode domain.Mode, dryRun bool) (*Runner, *fakeBroker) {
	b := &fakeBroker{
		account: domain.Account{Cash: 10000, Equity: 10000, BuyingPower: 10000},
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 100},
			"MSFT": {Symbol: "MSFT", Last: 200},
		},
	}
	gate := engine.NewRiskGate(engine.Policy{
		AllowMarketOrders: true,
	})
	e := engine.New(b, gate)
	r := NewRunner(
		e,
		&fakeAgent{},
		mode,
		[]string{"AAPL"},
		time.Second,
		2*time.Second,
		3*time.Second,
		2,
		0,
		dryRun,
		"",
	)
	return r, b
}

func (b *fakeBroker) GetAccount(context.Context) (domain.Account, error) {
	if b.accountErr != nil {
		return domain.Account{}, b.accountErr
	}
	return b.account, nil
}

func (b *fakeBroker) GetPositions(context.Context) ([]domain.Position, error) {
	if b.positionsErr != nil {
		return nil, b.positionsErr
	}
	out := make([]domain.Position, len(b.positions))
	copy(out, b.positions)
	return out, nil
}

func (b *fakeBroker) GetOpenOrders(context.Context) ([]domain.Order, error) {
	if b.ordersErr != nil {
		return nil, b.ordersErr
	}
	out := make([]domain.Order, len(b.openOrders))
	copy(out, b.openOrders)
	return out, nil
}

func (b *fakeBroker) GetQuote(_ context.Context, symbol string) (domain.Quote, error) {
	if err := b.quoteErr[symbol]; err != nil {
		return domain.Quote{}, err
	}
	if q, ok := b.quotes[symbol]; ok {
		return q, nil
	}
	return domain.Quote{}, fmt.Errorf("quote not found for %s", symbol)
}

func (b *fakeBroker) PlaceOrder(_ context.Context, req domain.OrderRequest) (domain.Order, error) {
	if b.placeErr != nil {
		return domain.Order{}, b.placeErr
	}
	b.placeCalls++
	b.placedRequests = append(b.placedRequests, req)
	return domain.Order{
		ID:        fmt.Sprintf("ord-%d", b.placeCalls),
		Symbol:    req.Symbol,
		Side:      req.Side,
		Qty:       req.Qty,
		Type:      req.Type,
		Status:    domain.OrderStatusNew,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (b *fakeBroker) CancelOrder(_ context.Context, orderID string) error {
	if b.cancelErr != nil {
		return b.cancelErr
	}
	b.cancelCalls++
	b.canceledOrderIDs = append(b.canceledOrderIDs, orderID)
	for i := range b.openOrders {
		if b.openOrders[i].ID == orderID {
			b.openOrders[i].Status = domain.OrderStatusCanceled
		}
	}
	return nil
}

func (b *fakeBroker) StreamTradeUpdates(context.Context) (<-chan domain.TradeUpdate, error) {
	ch := make(chan domain.TradeUpdate)
	close(ch)
	return ch, nil
}

func hasEventType(events []domain.Event, want string) bool {
	for _, e := range events {
		if e.Type == want {
			return true
		}
	}
	return false
}

func hasEventDetailContains(events []domain.Event, eventType, contains string) bool {
	for _, e := range events {
		if e.Type != eventType {
			continue
		}
		if strings.Contains(e.Details, contains) {
			return true
		}
	}
	return false
}
