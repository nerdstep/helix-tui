package paper

import (
	"context"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/domain"
)

func TestNew_InitialAccount(t *testing.T) {
	b := New(1000)
	acct, err := b.GetAccount(context.Background())
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acct.Cash != 1000 || acct.BuyingPower != 1000 || acct.Equity != 1000 {
		t.Fatalf("unexpected initial account: %#v", acct)
	}
}

func TestSetPriceAndGetQuote(t *testing.T) {
	b := New(1000)
	b.SetPrice("aapl", 123.45)
	b.SetPrice("aapl", -1) // ignored

	q, err := b.GetQuote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetQuote failed: %v", err)
	}
	if q.Last != 123.45 {
		t.Fatalf("unexpected quote last: %f", q.Last)
	}

	q2, err := b.GetQuote(context.Background(), "UNKNOWN")
	if err != nil {
		t.Fatalf("GetQuote failed: %v", err)
	}
	if q2.Last != 100 {
		t.Fatalf("expected default quote of 100, got %f", q2.Last)
	}
}

func TestPlaceOrder_BuyUpdatesAccountAndPosition(t *testing.T) {
	b := New(1000)
	b.SetPrice("AAPL", 100)

	order, err := b.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "aapl",
		Side:   domain.SideBuy,
		Qty:    2,
		Type:   domain.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if order.Status != domain.OrderStatusFilled {
		t.Fatalf("unexpected order status: %q", order.Status)
	}

	acct, err := b.GetAccount(context.Background())
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acct.Cash != 800 {
		t.Fatalf("unexpected cash after buy: %f", acct.Cash)
	}

	pos, err := b.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("GetPositions failed: %v", err)
	}
	if len(pos) != 1 || pos[0].Symbol != "AAPL" || pos[0].Qty != 2 {
		t.Fatalf("unexpected positions: %#v", pos)
	}

	openOrders, err := b.GetOpenOrders(context.Background())
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}
	if len(openOrders) != 0 {
		t.Fatalf("expected no open orders for instant fills, got %d", len(openOrders))
	}
}

func TestPlaceOrder_LimitUsesLimitPrice(t *testing.T) {
	b := New(1000)
	b.SetPrice("AAPL", 100)
	limit := 90.0
	_, err := b.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol:     "AAPL",
		Side:       domain.SideBuy,
		Qty:        1,
		Type:       domain.OrderTypeLimit,
		LimitPrice: &limit,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	acct, err := b.GetAccount(context.Background())
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acct.Cash != 910 {
		t.Fatalf("expected cash to use limit price, got %f", acct.Cash)
	}
}

func TestPlaceOrder_Errors(t *testing.T) {
	b := New(100)

	_, err := b.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    0,
		Type:   domain.OrderTypeMarket,
	})
	if err == nil || !strings.Contains(err.Error(), "qty") {
		t.Fatalf("expected qty error, got %v", err)
	}

	_, err = b.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "",
		Side:   domain.SideBuy,
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	})
	if err == nil || !strings.Contains(err.Error(), "symbol") {
		t.Fatalf("expected symbol error, got %v", err)
	}

	_, err = b.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    2,
		Type:   domain.OrderTypeMarket,
	})
	if err == nil || !strings.Contains(err.Error(), "insufficient cash") {
		t.Fatalf("expected cash error, got %v", err)
	}

	_, err = b.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL",
		Side:   domain.SideSell,
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	})
	if err == nil || !strings.Contains(err.Error(), "insufficient position") {
		t.Fatalf("expected position error, got %v", err)
	}
}

func TestCancelOrder_ErrorsForMissingAndFilled(t *testing.T) {
	b := New(1000)
	err := b.CancelOrder(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}

	b.SetPrice("AAPL", 100)
	order, err := b.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	err = b.CancelOrder(context.Background(), order.ID)
	if err == nil || !strings.Contains(err.Error(), "already filled") {
		t.Fatalf("expected already filled error, got %v", err)
	}
}

func TestStreamTradeUpdates_ReceivesFillUpdate(t *testing.T) {
	b := New(1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updates, err := b.StreamTradeUpdates(ctx)
	if err != nil {
		t.Fatalf("StreamTradeUpdates failed: %v", err)
	}

	b.SetPrice("AAPL", 100)
	order, err := b.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "AAPL",
		Side:   domain.SideBuy,
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	select {
	case up := <-updates:
		if up.OrderID != order.ID || up.Status != domain.OrderStatusFilled || up.FillPrice == nil {
			t.Fatalf("unexpected trade update: %#v", up)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for trade update")
	}
}
