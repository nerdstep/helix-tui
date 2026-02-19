package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"helix-tui/internal/domain"
)

const maxSnapshotEvents = 500

type Engine struct {
	broker domain.Broker
	gate   *RiskGate

	mu        sync.RWMutex
	account   domain.Account
	positions map[string]domain.Position
	orders    map[string]domain.Order
	events    []domain.Event
}

func New(broker domain.Broker, gate *RiskGate) *Engine {
	return &Engine{
		broker:    broker,
		gate:      gate,
		positions: map[string]domain.Position{},
		orders:    map[string]domain.Order{},
		events:    make([]domain.Event, 0, 256),
	}
}

func (e *Engine) Sync(ctx context.Context) error {
	return e.sync(ctx, true)
}

func (e *Engine) SyncQuiet(ctx context.Context) error {
	return e.sync(ctx, false)
}

func (e *Engine) sync(ctx context.Context, emitEvent bool) error {
	account, err := e.broker.GetAccount(ctx)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}
	positions, err := e.broker.GetPositions(ctx)
	if err != nil {
		return fmt.Errorf("get positions: %w", err)
	}
	orders, err := e.broker.GetOpenOrders(ctx)
	if err != nil {
		return fmt.Errorf("get open orders: %w", err)
	}

	posMap := make(map[string]domain.Position, len(positions))
	for _, p := range positions {
		posMap[strings.ToUpper(p.Symbol)] = p
	}

	orderMap := make(map[string]domain.Order, len(orders))
	for _, o := range orders {
		orderMap[o.ID] = o
	}

	e.mu.Lock()
	e.account = account
	e.positions = posMap
	e.orders = orderMap
	if emitEvent {
		e.addEventLocked("sync", "reconciled account, positions, and orders")
	}
	e.mu.Unlock()
	return nil
}

func (e *Engine) PlaceOrder(ctx context.Context, req domain.OrderRequest) (domain.Order, error) {
	req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	quote, err := e.broker.GetQuote(ctx, req.Symbol)
	if err != nil {
		return domain.Order{}, fmt.Errorf("get quote for %s: %w", req.Symbol, err)
	}
	if err := e.gate.Evaluate(req, quote); err != nil {
		return domain.Order{}, err
	}
	order, err := e.broker.PlaceOrder(ctx, req)
	if err != nil {
		return domain.Order{}, err
	}

	e.mu.Lock()
	e.orders[order.ID] = order
	e.addEventLocked(
		"order_placed",
		fmt.Sprintf("%s %s %.2f (%s)", order.Side, order.Symbol, order.Qty, order.ID),
	)
	e.mu.Unlock()
	return order, nil
}

func (e *Engine) GetQuote(ctx context.Context, symbol string) (domain.Quote, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return domain.Quote{}, fmt.Errorf("symbol is required")
	}
	return e.broker.GetQuote(ctx, symbol)
}

func (e *Engine) CancelOrder(ctx context.Context, orderID string) error {
	if strings.TrimSpace(orderID) == "" {
		return fmt.Errorf("order id is required")
	}
	if err := e.broker.CancelOrder(ctx, orderID); err != nil {
		return err
	}
	e.mu.Lock()
	if ord, ok := e.orders[orderID]; ok {
		ord.Status = domain.OrderStatusCanceled
		ord.UpdatedAt = time.Now().UTC()
		e.orders[orderID] = ord
	}
	e.addEventLocked("order_canceled", orderID)
	e.mu.Unlock()
	return nil
}

func (e *Engine) Flatten(ctx context.Context) error {
	e.mu.RLock()
	positions := make([]domain.Position, 0, len(e.positions))
	for _, p := range e.positions {
		if p.Qty > 0 {
			positions = append(positions, p)
		}
	}
	e.mu.RUnlock()

	for _, p := range positions {
		_, err := e.PlaceOrder(ctx, domain.OrderRequest{
			Symbol: p.Symbol,
			Side:   domain.SideSell,
			Qty:    p.Qty,
			Type:   domain.OrderTypeMarket,
		})
		if err != nil {
			return fmt.Errorf("flatten %s: %w", p.Symbol, err)
		}
	}
	return nil
}

func (e *Engine) StartTradeUpdateLoop(ctx context.Context) error {
	updates, err := e.broker.StreamTradeUpdates(ctx)
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case update, ok := <-updates:
				if !ok {
					return
				}
				e.applyTradeUpdate(update)
			}
		}
	}()
	return nil
}

func (e *Engine) Snapshot() domain.Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	positions := make([]domain.Position, 0, len(e.positions))
	for _, p := range e.positions {
		positions = append(positions, p)
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].Symbol < positions[j].Symbol })

	orders := make([]domain.Order, 0, len(e.orders))
	for _, o := range e.orders {
		orders = append(orders, o)
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i].UpdatedAt.After(orders[j].UpdatedAt) })

	events := make([]domain.Event, len(e.events))
	copy(events, e.events)
	if len(events) > maxSnapshotEvents {
		events = events[len(events)-maxSnapshotEvents:]
	}

	return domain.Snapshot{
		Account:   e.account,
		Positions: positions,
		Orders:    orders,
		Events:    events,
	}
}

func (e *Engine) AddEvent(eventType, details string) {
	e.mu.Lock()
	e.addEventLocked(eventType, details)
	e.mu.Unlock()
}

func (e *Engine) AllowSymbol(symbol string) {
	e.gate.AllowSymbol(symbol)
}

func (e *Engine) applyTradeUpdate(update domain.TradeUpdate) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ord, ok := e.orders[update.OrderID]
	if !ok {
		e.addEventLocked("trade_update_unknown_order", update.OrderID)
		return
	}

	ord.Status = update.Status
	ord.FilledQty = update.FillQty
	ord.UpdatedAt = update.Time
	e.orders[ord.ID] = ord
	e.addEventLocked(
		"trade_update",
		fmt.Sprintf("%s status=%s filled=%.2f", ord.ID, ord.Status, ord.FilledQty),
	)
}

func (e *Engine) addEventLocked(eventType, details string) {
	e.events = append(e.events, domain.Event{
		Time:    time.Now().UTC(),
		Type:    eventType,
		Details: details,
	})
}
