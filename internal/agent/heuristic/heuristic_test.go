package heuristic

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"helix-tui/internal/domain"
)

func TestNewDefaults(t *testing.T) {
	a := New(&quoteSeqBroker{}, 0, 0)
	if a.minMovePct != 0.01 {
		t.Fatalf("unexpected default minMovePct: %f", a.minMovePct)
	}
	if a.orderQty != 1 {
		t.Fatalf("unexpected default orderQty: %f", a.orderQty)
	}
}

func TestProposeTrades_GeneratesBuyAndSellSorted(t *testing.T) {
	b := &quoteSeqBroker{
		quotes: map[string][]float64{
			"AAPL": {100, 90},
			"TSLA": {100, 130},
		},
	}
	a := New(b, 0.05, 1)

	input := domain.AgentInput{
		Watchlist: []string{"AAPL", "TSLA"},
		Snapshot: domain.Snapshot{
			Positions: []domain.Position{{Symbol: "TSLA", Qty: 0.5}},
		},
	}

	first, err := a.ProposeTrades(context.Background(), input)
	if err != nil {
		t.Fatalf("first propose failed: %v", err)
	}
	if len(first) != 0 {
		t.Fatalf("first sample should not emit intents, got %d", len(first))
	}

	intents, err := a.ProposeTrades(context.Background(), input)
	if err != nil {
		t.Fatalf("second propose failed: %v", err)
	}
	if len(intents) != 2 {
		t.Fatalf("expected 2 intents, got %d", len(intents))
	}

	if intents[0].Symbol != "TSLA" || intents[0].Side != domain.SideSell {
		t.Fatalf("expected first intent to be TSLA sell, got %#v", intents[0])
	}
	if math.Abs(intents[0].Qty-0.5) > 1e-9 {
		t.Fatalf("expected TSLA qty to cap at position size, got %f", intents[0].Qty)
	}

	if intents[1].Symbol != "AAPL" || intents[1].Side != domain.SideBuy {
		t.Fatalf("expected second intent to be AAPL buy, got %#v", intents[1])
	}
}

func TestProposeTrades_SkipsOpenOrdersAndSmallMoves(t *testing.T) {
	b := &quoteSeqBroker{
		quotes: map[string][]float64{
			"MSFT": {100, 90},
			"NVDA": {100, 97},
		},
	}
	a := New(b, 0.05, 1)

	input := domain.AgentInput{
		Watchlist: []string{"AAPL", "MSFT", "MSFT", "NVDA"},
		Snapshot: domain.Snapshot{
			Orders: []domain.Order{
				{Symbol: "AAPL", Status: domain.OrderStatusNew},
			},
		},
	}

	if _, err := a.ProposeTrades(context.Background(), input); err != nil {
		t.Fatalf("first propose failed: %v", err)
	}

	intents, err := a.ProposeTrades(context.Background(), input)
	if err != nil {
		t.Fatalf("second propose failed: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}
	if intents[0].Symbol != "MSFT" || intents[0].Side != domain.SideBuy {
		t.Fatalf("unexpected intent: %#v", intents[0])
	}
}

func TestProposeTrades_QuoteErrorIgnored(t *testing.T) {
	b := &quoteSeqBroker{
		quoteErr: map[string]error{
			"AAPL": fmt.Errorf("downstream unavailable"),
		},
	}
	a := New(b, 0.01, 1)

	intents, err := a.ProposeTrades(context.Background(), domain.AgentInput{
		Watchlist: []string{"AAPL"},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(intents) != 0 {
		t.Fatalf("expected no intents, got %d", len(intents))
	}
}

func TestConfidenceBounds(t *testing.T) {
	if got := confidence(0.001, 0.01); got != 0.10 {
		t.Fatalf("expected low clamp 0.10, got %f", got)
	}
	if got := confidence(1.0, 0.01); got != 0.99 {
		t.Fatalf("expected high clamp 0.99, got %f", got)
	}
	if got := confidence(0.02, 0); got != 0.5 {
		t.Fatalf("expected default 0.5 when minMove<=0, got %f", got)
	}
}

type quoteSeqBroker struct {
	quotes   map[string][]float64
	quoteErr map[string]error
}

func (b *quoteSeqBroker) GetQuote(_ context.Context, symbol string) (domain.Quote, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if err := b.quoteErr[symbol]; err != nil {
		return domain.Quote{}, err
	}
	seq := b.quotes[symbol]
	if len(seq) == 0 {
		return domain.Quote{}, fmt.Errorf("no quote sequence for %s", symbol)
	}
	last := seq[0]
	b.quotes[symbol] = seq[1:]
	return domain.Quote{Symbol: symbol, Last: last}, nil
}

func (b *quoteSeqBroker) GetAccount(context.Context) (domain.Account, error) {
	return domain.Account{}, nil
}
func (b *quoteSeqBroker) GetPositions(context.Context) ([]domain.Position, error) {
	return nil, nil
}
func (b *quoteSeqBroker) GetOpenOrders(context.Context) ([]domain.Order, error) { return nil, nil }
func (b *quoteSeqBroker) PlaceOrder(context.Context, domain.OrderRequest) (domain.Order, error) {
	return domain.Order{}, nil
}
func (b *quoteSeqBroker) CancelOrder(context.Context, string) error { return nil }
func (b *quoteSeqBroker) StreamTradeUpdates(context.Context) (<-chan domain.TradeUpdate, error) {
	ch := make(chan domain.TradeUpdate)
	close(ch)
	return ch, nil
}
