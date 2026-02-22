package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/broker/paper"
	"helix-tui/internal/domain"
)

func TestSync_Success(t *testing.T) {
	b := &engineStubBroker{
		account: domain.Account{Cash: 1000, BuyingPower: 1000, Equity: 1000},
		positions: []domain.Position{
			{Symbol: "aapl", Qty: 2},
		},
		orders: []domain.Order{
			{ID: "o1", Symbol: "AAPL", Status: domain.OrderStatusNew},
		},
	}
	e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))

	if err := e.Sync(context.Background()); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	s := e.Snapshot()
	if s.Account.Cash != 1000 || len(s.Positions) != 1 || len(s.Orders) != 1 {
		t.Fatalf("unexpected snapshot after sync: %#v", s)
	}
	if !hasEngineEvent(s.Events, "sync") {
		t.Fatalf("expected sync event")
	}
}

func TestSync_ComplianceReconciliationEmitsPostureAndDriftEvents(t *testing.T) {
	b := &engineStubBroker{
		account: domain.Account{
			Cash:          2000,
			BuyingPower:   1500,
			Equity:        2000,
			DayTradeCount: 2,
		},
	}
	e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
	gate := NewComplianceGate(CompliancePolicy{
		Enabled:         true,
		AccountType:     "cash",
		AvoidPDT:        true,
		MaxDayTrades5D:  3,
		MinEquityForPDT: 25000,
		AvoidGoodFaith:  true,
		SettlementDays:  1,
	})
	now := time.Date(2026, time.February, 21, 10, 0, 0, 0, time.UTC)
	gate.now = func() time.Time { return now }
	gate.unsettledSells = []UnsettledSellProceeds{
		{Amount: 900, SettlesAt: now.Add(24 * time.Hour)},
	}
	e.SetComplianceGate(gate)

	if err := e.Sync(context.Background()); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	s := e.Snapshot()
	if !hasEngineEvent(s.Events, "compliance_posture") {
		t.Fatalf("expected compliance_posture event")
	}
	if !hasEngineEvent(s.Events, "compliance_drift_detected") {
		t.Fatalf("expected compliance_drift_detected event")
	}
}

func TestComplianceStatusSnapshotAvailableAfterSync(t *testing.T) {
	b := &engineStubBroker{
		account: domain.Account{
			Cash:        1000,
			BuyingPower: 1000,
			Equity:      1000,
		},
	}
	e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
	e.SetComplianceGate(NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
	}))
	if err := e.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	status, ok := e.ComplianceStatus()
	if !ok || status == nil {
		t.Fatalf("expected compliance status snapshot")
	}
	if !status.Enabled {
		t.Fatalf("expected enabled compliance status")
	}
	if status.AccountType != "cash" {
		t.Fatalf("expected cash account type, got %#v", status)
	}
}

func TestSyncQuiet_SuccessWithoutSyncEvent(t *testing.T) {
	b := &engineStubBroker{
		account: domain.Account{Cash: 1000, BuyingPower: 1000, Equity: 1000},
	}
	e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))

	if err := e.SyncQuiet(context.Background()); err != nil {
		t.Fatalf("SyncQuiet failed: %v", err)
	}
	s := e.Snapshot()
	if hasEngineEvent(s.Events, "sync") {
		t.Fatalf("did not expect sync event from SyncQuiet")
	}
}

func TestSync_Errors(t *testing.T) {
	tests := []struct {
		name string
		b    *engineStubBroker
		want string
	}{
		{
			name: "account error",
			b:    &engineStubBroker{accountErr: errors.New("down")},
			want: "get account",
		},
		{
			name: "positions error",
			b: &engineStubBroker{
				account:      domain.Account{},
				positionsErr: errors.New("down"),
			},
			want: "get positions",
		},
		{
			name: "orders error",
			b: &engineStubBroker{
				account:   domain.Account{},
				ordersErr: errors.New("down"),
			},
			want: "get open orders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(tt.b, NewRiskGate(Policy{AllowMarketOrders: true}))
			err := e.Sync(context.Background())
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPlaceOrder_Success(t *testing.T) {
	b := &engineStubBroker{
		account: domain.Account{},
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 100},
		},
	}
	e := New(b, NewRiskGate(Policy{
		AllowMarketOrders:   true,
		MaxNotionalPerTrade: 1000,
		MaxNotionalPerDay:   2000,
	}))
	if err := e.Sync(context.Background()); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	order, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "aapl",
		Side:   domain.SideBuy,
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if order.ID == "" || len(b.placedRequests) != 1 {
		t.Fatalf("expected one placed order, got %+v", b.placedRequests)
	}
	if b.placedRequests[0].Symbol != "AAPL" {
		t.Fatalf("expected uppercase symbol, got %q", b.placedRequests[0].Symbol)
	}
	if !hasEngineEvent(e.Snapshot().Events, "order_placed") {
		t.Fatalf("expected order_placed event")
	}
}

func TestPlaceOrder_Errors(t *testing.T) {
	t.Run("quote error", func(t *testing.T) {
		b := &engineStubBroker{
			quoteErr: map[string]error{"AAPL": errors.New("quote down")},
		}
		e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
		_, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
			Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, Type: domain.OrderTypeMarket,
		})
		if err == nil || !strings.Contains(err.Error(), "get quote") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("risk error", func(t *testing.T) {
		b := &engineStubBroker{
			quotes: map[string]domain.Quote{"AAPL": {Symbol: "AAPL", Last: 100}},
		}
		e := New(b, NewRiskGate(Policy{
			AllowMarketOrders:   true,
			MaxNotionalPerTrade: 10,
		}))
		_, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
			Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, Type: domain.OrderTypeMarket,
		})
		if err == nil || !strings.Contains(err.Error(), "max per trade") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("broker error", func(t *testing.T) {
		b := &engineStubBroker{
			quotes:   map[string]domain.Quote{"AAPL": {Symbol: "AAPL", Last: 100}},
			placeErr: errors.New("place down"),
		}
		e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
		_, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
			Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, Type: domain.OrderTypeMarket,
		})
		if err == nil || !strings.Contains(err.Error(), "place down") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestCancelOrder(t *testing.T) {
	t.Run("empty id", func(t *testing.T) {
		e := New(&engineStubBroker{}, NewRiskGate(Policy{AllowMarketOrders: true}))
		err := e.CancelOrder(context.Background(), "")
		if err == nil || !strings.Contains(err.Error(), "required") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("broker error", func(t *testing.T) {
		b := &engineStubBroker{cancelErr: errors.New("cancel down")}
		e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
		err := e.CancelOrder(context.Background(), "o1")
		if err == nil || !strings.Contains(err.Error(), "cancel down") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		b := &engineStubBroker{
			account: domain.Account{},
			quotes:  map[string]domain.Quote{"AAPL": {Symbol: "AAPL", Last: 100}},
		}
		e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
		if err := e.Sync(context.Background()); err != nil {
			t.Fatalf("Sync failed: %v", err)
		}
		order, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
			Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, Type: domain.OrderTypeMarket,
		})
		if err != nil {
			t.Fatalf("PlaceOrder failed: %v", err)
		}

		if err := e.CancelOrder(context.Background(), order.ID); err != nil {
			t.Fatalf("CancelOrder failed: %v", err)
		}
		s := e.Snapshot()
		if len(s.Orders) == 0 || s.Orders[0].Status != domain.OrderStatusCanceled {
			t.Fatalf("expected canceled order, got %#v", s.Orders)
		}
		if !hasEngineEvent(s.Events, "order_canceled") {
			t.Fatalf("expected order_canceled event")
		}
	})
}

func TestPlaceOrder_ComplianceRejectedEmitsEvent(t *testing.T) {
	b := &engineStubBroker{
		account: domain.Account{
			Cash:          1000,
			BuyingPower:   2000,
			Equity:        1000,
			Multiplier:    2,
			DayTradeCount: 2,
		},
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 100},
		},
	}
	e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
	e.SetComplianceGate(NewComplianceGate(CompliancePolicy{
		Enabled:         true,
		AccountType:     "margin",
		AvoidPDT:        true,
		MaxDayTrades5D:  3,
		MinEquityForPDT: 25000,
	}))
	if err := e.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	_, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, Type: domain.OrderTypeMarket,
	})
	if err == nil {
		t.Fatalf("expected compliance rejection")
	}
	if !hasEngineEvent(e.Snapshot().Events, "compliance_rejected") {
		t.Fatalf("expected compliance_rejected event")
	}
}

func TestPlaceOrder_ComplianceGFVRejectsBuyUsingUnsettledProceeds(t *testing.T) {
	b := paper.New(1000)
	b.SetPrice("AAPL", 10)

	e := New(b, NewRiskGate(Policy{
		AllowMarketOrders:   true,
		MaxNotionalPerTrade: 100000,
		MaxNotionalPerDay:   100000,
	}))
	e.SetComplianceGate(NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	}))
	if err := e.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Seed and then liquidate a position to create unsettled sell proceeds.
	if _, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL", Side: domain.SideBuy, Qty: 10, Type: domain.OrderTypeMarket,
	}); err != nil {
		t.Fatalf("seed buy failed: %v", err)
	}
	if _, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL", Side: domain.SideSell, Qty: 10, Type: domain.OrderTypeMarket,
	}); err != nil {
		t.Fatalf("seed sell failed: %v", err)
	}

	_, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL", Side: domain.SideBuy, Qty: 95, Type: domain.OrderTypeMarket,
	})
	if err == nil {
		t.Fatalf("expected gfv guard rejection")
	}
	if !strings.Contains(err.Error(), "gfv guard") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasEngineEvent(e.Snapshot().Events, "compliance_rejected") {
		t.Fatalf("expected compliance_rejected event")
	}
}

func TestSetComplianceSettlementStore_WiresStore(t *testing.T) {
	e := New(&engineStubBroker{}, NewRiskGate(Policy{AllowMarketOrders: true}))
	e.SetComplianceGate(NewComplianceGate(CompliancePolicy{Enabled: true, AvoidGoodFaith: true}))
	store := &stubComplianceSettlementStore{}
	if err := e.SetComplianceSettlementStore(store); err != nil {
		t.Fatalf("set compliance settlement store failed: %v", err)
	}
	if store.loadCalls != 1 {
		t.Fatalf("expected settlement store load call, got %d", store.loadCalls)
	}
}

func TestPlaceOrder_ComplianceSettlementPersistErrorEmitsDatabaseEvent(t *testing.T) {
	b := &engineStubBroker{
		account:     domain.Account{Cash: 10000, BuyingPower: 10000, Equity: 10000, Multiplier: 1},
		quotes:      map[string]domain.Quote{"AAPL": {Symbol: "AAPL", Last: 10, Bid: 9.99, Ask: 10.01}},
		fillOnPlace: true,
	}
	e := New(b, NewRiskGate(Policy{AllowMarketOrders: true, MaxNotionalPerTrade: 100000, MaxNotionalPerDay: 100000}))
	e.SetComplianceGate(NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	}))
	store := &stubComplianceSettlementStore{appendErr: errors.New("write failed")}
	if err := e.SetComplianceSettlementStore(store); err != nil {
		t.Fatalf("set compliance settlement store failed: %v", err)
	}
	if err := e.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	_, err := e.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL", Side: domain.SideSell, Qty: 1, Type: domain.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("sell failed: %v", err)
	}
	if !hasEngineEvent(e.Snapshot().Events, "database_error") {
		t.Fatalf("expected database_error event")
	}
}

func TestFlatten(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		b := &engineStubBroker{
			account: domain.Account{},
			positions: []domain.Position{
				{Symbol: "AAPL", Qty: 2},
				{Symbol: "MSFT", Qty: 0},
			},
			quotes: map[string]domain.Quote{
				"AAPL": {Symbol: "AAPL", Last: 100},
				"MSFT": {Symbol: "MSFT", Last: 100},
			},
		}
		e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
		if err := e.Sync(context.Background()); err != nil {
			t.Fatalf("Sync failed: %v", err)
		}
		if err := e.Flatten(context.Background()); err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}
		if len(b.placedRequests) != 1 || b.placedRequests[0].Side != domain.SideSell {
			t.Fatalf("expected one sell flatten order, got %#v", b.placedRequests)
		}
	})

	t.Run("error", func(t *testing.T) {
		b := &engineStubBroker{
			account: domain.Account{},
			positions: []domain.Position{
				{Symbol: "AAPL", Qty: 2},
			},
			quotes: map[string]domain.Quote{
				"AAPL": {Symbol: "AAPL", Last: 100},
			},
			placeErr: errors.New("place failed"),
		}
		e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
		if err := e.Sync(context.Background()); err != nil {
			t.Fatalf("Sync failed: %v", err)
		}
		err := e.Flatten(context.Background())
		if err == nil || !strings.Contains(err.Error(), "flatten AAPL") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestStartTradeUpdateLoop(t *testing.T) {
	t.Run("stream error", func(t *testing.T) {
		b := &engineStubBroker{streamErr: errors.New("stream down")}
		e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
		err := e.StartTradeUpdateLoop(context.Background())
		if err == nil || !strings.Contains(err.Error(), "stream down") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("applies updates", func(t *testing.T) {
		updateCh := make(chan domain.TradeUpdate, 2)
		b := &engineStubBroker{
			account: domain.Account{},
			orders: []domain.Order{
				{ID: "o1", Symbol: "AAPL", Status: domain.OrderStatusNew, UpdatedAt: time.Now().Add(-time.Minute)},
			},
			streamCh: updateCh,
		}
		e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
		if err := e.Sync(context.Background()); err != nil {
			t.Fatalf("Sync failed: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := e.StartTradeUpdateLoop(ctx); err != nil {
			t.Fatalf("StartTradeUpdateLoop failed: %v", err)
		}

		updateCh <- domain.TradeUpdate{
			OrderID: "o1",
			Status:  domain.OrderStatusFilled,
			FillQty: 1,
			Time:    time.Now().UTC(),
		}
		updateCh <- domain.TradeUpdate{
			OrderID: "missing",
			Status:  domain.OrderStatusFilled,
			FillQty: 1,
			Time:    time.Now().UTC(),
		}

		waitForEngine(t, time.Second, func() bool {
			s := e.Snapshot()
			return hasEngineEvent(s.Events, "trade_update") && hasEngineEvent(s.Events, "trade_update_unknown_order")
		})
	})
}

func TestSnapshot_TrimsEventsAndSortsPositions(t *testing.T) {
	e := New(&engineStubBroker{}, NewRiskGate(Policy{AllowMarketOrders: true}))
	e.positions["MSFT"] = domain.Position{Symbol: "MSFT"}
	e.positions["AAPL"] = domain.Position{Symbol: "AAPL"}
	now := time.Now()
	e.orders["1"] = domain.Order{ID: "1", UpdatedAt: now.Add(-time.Minute)}
	e.orders["2"] = domain.Order{ID: "2", UpdatedAt: now}
	for i := 0; i < 600; i++ {
		e.AddEvent("evt", fmt.Sprintf("%d", i))
	}

	s := e.Snapshot()
	if len(s.Events) != maxSnapshotEvents {
		t.Fatalf("expected %d events, got %d", maxSnapshotEvents, len(s.Events))
	}
	if s.Events[0].Details != "100" {
		t.Fatalf("expected oldest retained event to be 100, got %q", s.Events[0].Details)
	}
	if s.Events[len(s.Events)-1].Details != "599" {
		t.Fatalf("expected newest retained event to be 599, got %q", s.Events[len(s.Events)-1].Details)
	}
	if len(s.Positions) != 2 || s.Positions[0].Symbol != "AAPL" {
		t.Fatalf("expected sorted positions, got %#v", s.Positions)
	}
	if len(s.Orders) != 2 || s.Orders[0].ID != "2" {
		t.Fatalf("expected newest order first, got %#v", s.Orders)
	}
}

func TestGetQuote_UsesCacheAndUpsert(t *testing.T) {
	b := &engineStubBroker{
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 100, Time: time.Now().UTC()},
		},
	}
	e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))

	q1, err := e.GetQuote(context.Background(), "aapl")
	if err != nil {
		t.Fatalf("GetQuote failed: %v", err)
	}
	if q1.Symbol != "AAPL" || q1.Last != 100 {
		t.Fatalf("unexpected first quote: %#v", q1)
	}
	if b.quoteCalls != 1 {
		t.Fatalf("expected first call to hit broker, got %d", b.quoteCalls)
	}

	b.quotes["AAPL"] = domain.Quote{Symbol: "AAPL", Last: 101, Time: time.Now().UTC()}
	q2, err := e.GetQuote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetQuote failed: %v", err)
	}
	if q2.Last != 100 {
		t.Fatalf("expected cached quote last=100, got %#v", q2)
	}
	if b.quoteCalls != 1 {
		t.Fatalf("expected cached quote to avoid broker call, got %d", b.quoteCalls)
	}

	e.UpsertQuote(domain.Quote{Symbol: "aapl", Last: 102, Time: time.Now().UTC()})
	q3, err := e.GetQuote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetQuote failed: %v", err)
	}
	if q3.Last != 102 {
		t.Fatalf("expected upserted quote last=102, got %#v", q3)
	}
	if b.quoteCalls != 1 {
		t.Fatalf("expected no extra broker calls after upsert, got %d", b.quoteCalls)
	}
}

func TestGetQuote_StaleCacheFallsBackToBroker(t *testing.T) {
	b := &engineStubBroker{
		quotes: map[string]domain.Quote{
			"AAPL": {Symbol: "AAPL", Last: 101, Time: time.Now().UTC()},
		},
	}
	e := New(b, NewRiskGate(Policy{AllowMarketOrders: true}))
	e.UpsertQuote(domain.Quote{Symbol: "AAPL", Last: 100, Time: time.Now().UTC()})
	e.quoteSeen["AAPL"] = time.Now().UTC().Add(-(cachedQuoteFreshFor + 100*time.Millisecond))

	q, err := e.GetQuote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetQuote failed: %v", err)
	}
	if q.Last != 101 {
		t.Fatalf("expected broker quote to replace stale cache, got %#v", q)
	}
	if b.quoteCalls != 1 {
		t.Fatalf("expected broker to be called once, got %d", b.quoteCalls)
	}
}

type engineStubBroker struct {
	account      domain.Account
	positions    []domain.Position
	orders       []domain.Order
	quotes       map[string]domain.Quote
	quoteErr     map[string]error
	accountErr   error
	positionsErr error
	ordersErr    error
	placeErr     error
	cancelErr    error
	streamErr    error
	streamCh     chan domain.TradeUpdate
	quoteCalls   int
	fillOnPlace  bool

	placedRequests []domain.OrderRequest
}

type stubComplianceSettlementStore struct {
	loadCalls int
	loadLots  []UnsettledSellProceeds
	loadErr   error
	appendErr error
	pruneErr  error
}

func (s *stubComplianceSettlementStore) LoadUnsettledSellProceeds(_ time.Time) ([]UnsettledSellProceeds, error) {
	s.loadCalls++
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	out := make([]UnsettledSellProceeds, len(s.loadLots))
	copy(out, s.loadLots)
	return out, nil
}

func (s *stubComplianceSettlementStore) AppendUnsettledSellProceeds(UnsettledSellProceeds, time.Time) error {
	return s.appendErr
}

func (s *stubComplianceSettlementStore) PruneSettledSellProceeds(time.Time) error {
	return s.pruneErr
}

func (b *engineStubBroker) GetAccount(context.Context) (domain.Account, error) {
	if b.accountErr != nil {
		return domain.Account{}, b.accountErr
	}
	return b.account, nil
}

func (b *engineStubBroker) GetPositions(context.Context) ([]domain.Position, error) {
	if b.positionsErr != nil {
		return nil, b.positionsErr
	}
	out := make([]domain.Position, len(b.positions))
	copy(out, b.positions)
	return out, nil
}

func (b *engineStubBroker) GetOpenOrders(context.Context) ([]domain.Order, error) {
	if b.ordersErr != nil {
		return nil, b.ordersErr
	}
	out := make([]domain.Order, len(b.orders))
	copy(out, b.orders)
	return out, nil
}

func (b *engineStubBroker) GetQuote(_ context.Context, symbol string) (domain.Quote, error) {
	b.quoteCalls++
	if err := b.quoteErr[symbol]; err != nil {
		return domain.Quote{}, err
	}
	if q, ok := b.quotes[symbol]; ok {
		return q, nil
	}
	return domain.Quote{}, fmt.Errorf("no quote for %s", symbol)
}

func (b *engineStubBroker) PlaceOrder(_ context.Context, req domain.OrderRequest) (domain.Order, error) {
	if b.placeErr != nil {
		return domain.Order{}, b.placeErr
	}
	b.placedRequests = append(b.placedRequests, req)
	status := domain.OrderStatusNew
	filledQty := 0.0
	if b.fillOnPlace {
		status = domain.OrderStatusFilled
		filledQty = req.Qty
	}
	return domain.Order{
		ID:        fmt.Sprintf("ord-%d", len(b.placedRequests)),
		Symbol:    req.Symbol,
		Side:      req.Side,
		Qty:       req.Qty,
		FilledQty: filledQty,
		Type:      req.Type,
		Status:    status,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (b *engineStubBroker) CancelOrder(context.Context, string) error {
	return b.cancelErr
}

func (b *engineStubBroker) StreamTradeUpdates(context.Context) (<-chan domain.TradeUpdate, error) {
	if b.streamErr != nil {
		return nil, b.streamErr
	}
	if b.streamCh == nil {
		ch := make(chan domain.TradeUpdate)
		close(ch)
		return ch, nil
	}
	return b.streamCh, nil
}

func hasEngineEvent(events []domain.Event, want string) bool {
	for _, e := range events {
		if e.Type == want {
			return true
		}
	}
	return false
}

func waitForEngine(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
