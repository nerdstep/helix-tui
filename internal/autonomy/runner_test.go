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

func TestRunCycle_PersistsRelevantTradeEvents(t *testing.T) {
	r, _ := newRunnerTestHarness(domain.ModeAuto, false)
	agent := &fakeAgent{}
	r.agent = agent
	store := &fakeEventHistoryStore{}
	r.SetEventHistory(store)

	r.engine.AddEvent("order_placed", "buy AAPL 1")
	r.engine.AddEvent("agent_cycle_start", "mode=auto watchlist=1")
	r.engine.AddEvent("trade_update", "abc status=filled filled=1.00")

	if err := r.runCycle(context.Background()); err != nil {
		t.Fatalf("runCycle failed: %v", err)
	}
	if !hasEventType(store.appended, "order_placed") {
		t.Fatalf("expected order_placed to be persisted, got %#v", store.appended)
	}
	if !hasEventType(store.appended, "trade_update") {
		t.Fatalf("expected trade_update to be persisted, got %#v", store.appended)
	}
	if hasEventType(store.appended, "agent_cycle_start") {
		t.Fatalf("did not expect agent_cycle_start to be persisted: %#v", store.appended)
	}
}

type fakeAgent struct {
	intents   []domain.TradeIntent
	err       error
	calls     int
	lastInput domain.AgentInput
}

type fakeEventHistoryStore struct {
	appended      []domain.Event
	appendErr     error
	listRecentOut []domain.Event
	listRecentErr error
}

func (f *fakeEventHistoryStore) AppendMany(events []domain.Event) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.appended = append(f.appended, events...)
	return nil
}

func (f *fakeEventHistoryStore) ListRecent(_ int) ([]domain.Event, error) {
	if f.listRecentErr != nil {
		return nil, f.listRecentErr
	}
	out := make([]domain.Event, len(f.listRecentOut))
	copy(out, f.listRecentOut)
	return out, nil
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
	account        domain.Account
	positions      []domain.Position
	openOrders     []domain.Order
	quotes         map[string]domain.Quote
	quoteErr       map[string]error
	accountErr     error
	positionsErr   error
	ordersErr      error
	placeErr       error
	placeCalls     int
	placedRequests []domain.OrderRequest
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

func (b *fakeBroker) CancelOrder(context.Context, string) error { return nil }

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
