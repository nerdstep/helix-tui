package paper

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"helix-tui/internal/domain"
)

type Broker struct {
	mu        sync.RWMutex
	account   domain.Account
	positions map[string]domain.Position
	orders    map[string]domain.Order
	prices    map[string]float64
	updates   chan domain.TradeUpdate
	counter   uint64
}

func New(initialCash float64) *Broker {
	return &Broker{
		account: domain.Account{
			Cash:        initialCash,
			BuyingPower: initialCash,
			Equity:      initialCash,
			Multiplier:  1,
		},
		positions: map[string]domain.Position{},
		orders:    map[string]domain.Order{},
		prices: map[string]float64{
			"AAPL": 192.15,
			"MSFT": 416.75,
			"TSLA": 184.33,
			"NVDA": 731.20,
		},
		updates: make(chan domain.TradeUpdate, 256),
	}
}

func (b *Broker) SetPrice(symbol string, price float64) {
	if price <= 0 {
		return
	}
	b.mu.Lock()
	b.prices[strings.ToUpper(strings.TrimSpace(symbol))] = price
	b.mu.Unlock()
}

func (b *Broker) GetAccount(_ context.Context) (domain.Account, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	account := b.account
	equity := account.Cash
	for _, p := range b.positions {
		last := p.LastPrice
		if last <= 0 {
			last = p.AvgCost
		}
		equity += p.Qty * last
	}
	account.Equity = equity
	account.BuyingPower = account.Cash
	return account, nil
}

func (b *Broker) GetPositions(_ context.Context) ([]domain.Position, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]domain.Position, 0, len(b.positions))
	for _, p := range b.positions {
		out = append(out, p)
	}
	return out, nil
}

func (b *Broker) GetOpenOrders(_ context.Context) ([]domain.Order, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]domain.Order, 0, len(b.orders))
	for _, o := range b.orders {
		if o.Status == domain.OrderStatusNew || o.Status == domain.OrderStatusAccepted || o.Status == domain.OrderStatusPartially {
			out = append(out, o)
		}
	}
	return out, nil
}

func (b *Broker) GetQuote(_ context.Context, symbol string) (domain.Quote, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	b.mu.RLock()
	defer b.mu.RUnlock()
	last, ok := b.prices[symbol]
	if !ok {
		last = 100.0
	}
	return domain.Quote{
		Symbol: symbol,
		Bid:    last - 0.01,
		Ask:    last + 0.01,
		Last:   last,
		Time:   time.Now().UTC(),
	}, nil
}

func (b *Broker) PlaceOrder(ctx context.Context, req domain.OrderRequest) (domain.Order, error) {
	if req.Qty <= 0 {
		return domain.Order{}, fmt.Errorf("qty must be greater than 0")
	}
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		return domain.Order{}, fmt.Errorf("symbol is required")
	}

	quote, err := b.GetQuote(ctx, symbol)
	if err != nil {
		return domain.Order{}, err
	}
	fillPrice := quote.Last
	if req.Type == domain.OrderTypeLimit && req.LimitPrice != nil {
		fillPrice = *req.LimitPrice
	}
	notional := fillPrice * req.Qty

	b.mu.Lock()
	defer b.mu.Unlock()

	orderID := fmt.Sprintf("paper-%08d", atomic.AddUint64(&b.counter, 1))
	now := time.Now().UTC()
	order := domain.Order{
		ID:         orderID,
		Symbol:     symbol,
		Side:       req.Side,
		Qty:        req.Qty,
		FilledQty:  req.Qty,
		Type:       req.Type,
		LimitPrice: req.LimitPrice,
		Status:     domain.OrderStatusFilled,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	switch req.Side {
	case domain.SideBuy:
		if b.account.Cash < notional {
			return domain.Order{}, fmt.Errorf("insufficient cash for %.2f notional", notional)
		}
		b.account.Cash -= notional
		existing := b.positions[symbol]
		newQty := existing.Qty + req.Qty
		newAvg := fillPrice
		if newQty > 0 {
			newAvg = ((existing.AvgCost * existing.Qty) + notional) / newQty
		}
		b.positions[symbol] = domain.Position{
			Symbol:    symbol,
			Qty:       newQty,
			AvgCost:   newAvg,
			LastPrice: fillPrice,
		}
	case domain.SideSell:
		existing := b.positions[symbol]
		if existing.Qty < req.Qty {
			return domain.Order{}, fmt.Errorf("insufficient position in %s", symbol)
		}
		b.account.Cash += notional
		remaining := existing.Qty - req.Qty
		if remaining == 0 {
			delete(b.positions, symbol)
		} else {
			existing.Qty = remaining
			existing.LastPrice = fillPrice
			b.positions[symbol] = existing
		}
	default:
		return domain.Order{}, fmt.Errorf("unsupported side %q", req.Side)
	}

	b.orders[order.ID] = order
	update := domain.TradeUpdate{
		OrderID:   order.ID,
		Status:    order.Status,
		FillQty:   order.FilledQty,
		FillPrice: &fillPrice,
		Time:      now,
	}
	select {
	case b.updates <- update:
	default:
	}
	return order, nil
}

func (b *Broker) CancelOrder(_ context.Context, orderID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	ord, ok := b.orders[orderID]
	if !ok {
		return fmt.Errorf("order %s not found", orderID)
	}
	if ord.Status == domain.OrderStatusFilled {
		return fmt.Errorf("order %s already filled", orderID)
	}
	ord.Status = domain.OrderStatusCanceled
	ord.UpdatedAt = time.Now().UTC()
	b.orders[orderID] = ord
	select {
	case b.updates <- domain.TradeUpdate{
		OrderID: orderID,
		Status:  domain.OrderStatusCanceled,
		Time:    ord.UpdatedAt,
	}:
	default:
	}
	return nil
}

func (b *Broker) StreamTradeUpdates(ctx context.Context) (<-chan domain.TradeUpdate, error) {
	out := make(chan domain.TradeUpdate, 256)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case update := <-b.updates:
				select {
				case out <- update:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
