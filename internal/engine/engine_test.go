package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
	for i := 0; i < 60; i++ {
		e.AddEvent("evt", fmt.Sprintf("%d", i))
	}

	s := e.Snapshot()
	if len(s.Events) != 50 {
		t.Fatalf("expected 50 events, got %d", len(s.Events))
	}
	if len(s.Positions) != 2 || s.Positions[0].Symbol != "AAPL" {
		t.Fatalf("expected sorted positions, got %#v", s.Positions)
	}
	if len(s.Orders) != 2 || s.Orders[0].ID != "2" {
		t.Fatalf("expected newest order first, got %#v", s.Orders)
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

	placedRequests []domain.OrderRequest
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
	return domain.Order{
		ID:        fmt.Sprintf("ord-%d", len(b.placedRequests)),
		Symbol:    req.Symbol,
		Side:      req.Side,
		Qty:       req.Qty,
		Type:      req.Type,
		Status:    domain.OrderStatusNew,
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
