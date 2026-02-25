package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"helix-tui/internal/domain"
	"helix-tui/internal/eventmeta"
)

const maxSnapshotEvents = 500
const cachedQuoteFreshFor = 20 * time.Second

type Engine struct {
	broker     domain.Broker
	gate       *RiskGate
	compliance *ComplianceGate

	mu         sync.RWMutex
	account    domain.Account
	positions  map[string]domain.Position
	orders     map[string]domain.Order
	quotes     map[string]domain.Quote
	quoteSeen  map[string]time.Time
	events     []domain.Event
	eventStart int
	eventCount int
	eventSinks []func(domain.Event)
}

func New(broker domain.Broker, gate *RiskGate) *Engine {
	return &Engine{
		broker:    broker,
		gate:      gate,
		positions: map[string]domain.Position{},
		orders:    map[string]domain.Order{},
		quotes:    map[string]domain.Quote{},
		quoteSeen: map[string]time.Time{},
		events:    make([]domain.Event, maxSnapshotEvents),
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

	var complianceResult ComplianceReconcileResult
	if e.compliance != nil {
		complianceResult = e.compliance.ReconcileBrokerAccount(account)
	}

	var emitted domain.Event
	var complianceEvents []domain.Event
	e.mu.Lock()
	e.account = account
	e.positions = posMap
	e.orders = orderMap
	if complianceResult.Status.Enabled {
		if complianceResult.PostureChanged {
			complianceEvents = append(complianceEvents, domain.Event{
				Type:    "compliance_posture",
				Details: formatCompliancePostureDetails(complianceResult.Status),
			})
		}
		if complianceResult.DriftChanged {
			eventType := "compliance_drift_cleared"
			if complianceResult.Status.UnsettledDriftDetected {
				eventType = "compliance_drift_detected"
			}
			complianceEvents = append(complianceEvents, domain.Event{
				Type:    eventType,
				Details: formatComplianceDriftDetails(complianceResult.Status),
			})
		}
	}
	if emitEvent {
		emitted = e.addEventLocked(domain.Event{
			Type:    "sync",
			Details: "reconciled account, positions, and orders",
		})
	}
	dispatchedComplianceEvents := make([]domain.Event, 0, len(complianceEvents))
	for _, evt := range complianceEvents {
		dispatchedComplianceEvents = append(dispatchedComplianceEvents, e.addEventLocked(evt))
	}
	e.mu.Unlock()
	if emitEvent {
		e.dispatchEventSinks(emitted)
	}
	for _, evt := range dispatchedComplianceEvents {
		e.dispatchEventSinks(evt)
	}
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
	rollbackRisk := true
	defer func() {
		if rollbackRisk {
			e.gate.Rollback(req, quote)
		}
	}()
	if e.compliance != nil {
		snapshot := e.Snapshot()
		if account, accountErr := e.broker.GetAccount(ctx); accountErr == nil {
			snapshot.Account = account
			e.mu.Lock()
			e.account = account
			e.mu.Unlock()
		}
		if err := e.compliance.Evaluate(req, quote, snapshot); err != nil {
			e.AddEvent("compliance_rejected", fmt.Sprintf("%s %s %.2f: %v", req.Side, req.Symbol, req.Qty, err))
			return domain.Order{}, err
		}
	}
	order, err := e.broker.PlaceOrder(ctx, req)
	if err != nil {
		return domain.Order{}, err
	}
	rollbackRisk = false

	recordFill := false
	fillSide := order.Side
	fillQty := order.FilledQty
	fillPrice := estimateOrderFillPrice(order, quote)
	fillTime := order.UpdatedAt
	if fillTime.IsZero() {
		fillTime = order.CreatedAt
	}
	if fillQty > 0 && fillPrice > 0 {
		recordFill = true
	}

	e.mu.Lock()
	e.orders[order.ID] = order
	emitted := e.addEventLocked(domain.Event{
		Type:    "order_placed",
		Details: fmt.Sprintf("%s %s %.2f (%s)", order.Side, order.Symbol, order.Qty, order.ID),
	})
	e.mu.Unlock()

	if recordFill && e.compliance != nil {
		if recordErr := e.compliance.RecordFill(fillSide, fillQty, fillPrice, fillTime); recordErr != nil {
			e.AddEvent("database_error", fmt.Sprintf("persist compliance settlement state: %v", recordErr))
		}
	}
	e.dispatchEventSinks(emitted)
	return order, nil
}

func (e *Engine) GetQuote(ctx context.Context, symbol string) (domain.Quote, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return domain.Quote{}, fmt.Errorf("symbol is required")
	}
	e.mu.RLock()
	if q, ok := e.quotes[symbol]; ok && quoteIsFresh(q, e.quoteSeen[symbol]) {
		e.mu.RUnlock()
		return q, nil
	}
	e.mu.RUnlock()

	q, err := e.broker.GetQuote(ctx, symbol)
	if err != nil {
		return domain.Quote{}, err
	}
	e.UpsertQuote(q)
	return q, nil
}

func (e *Engine) UpsertQuote(quote domain.Quote) {
	symbol := strings.ToUpper(strings.TrimSpace(quote.Symbol))
	if symbol == "" {
		return
	}
	quote.Symbol = symbol
	e.mu.Lock()
	e.quotes[symbol] = quote
	e.quoteSeen[symbol] = time.Now().UTC()
	e.mu.Unlock()
}

func quoteIsFresh(q domain.Quote, seenAt time.Time) bool {
	if q.Last <= 0 && q.Bid <= 0 && q.Ask <= 0 {
		return false
	}
	if seenAt.IsZero() {
		return false
	}
	return time.Since(seenAt) <= cachedQuoteFreshFor
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
	emitted := e.addEventLocked(domain.Event{
		Type:    "order_canceled",
		Details: orderID,
	})
	e.mu.Unlock()
	e.dispatchEventSinks(emitted)
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

	events := make([]domain.Event, e.eventCount)
	for i := 0; i < e.eventCount; i++ {
		idx := (e.eventStart + i) % len(e.events)
		events[i] = e.events[idx]
	}

	return domain.Snapshot{
		Account:   e.account,
		Positions: positions,
		Orders:    orders,
		Events:    events,
	}
}

func (e *Engine) AddEvent(eventType, details string) {
	e.AddStructuredEvent(domain.Event{
		Type:    eventType,
		Details: details,
	})
}

func (e *Engine) AddStructuredEvent(event domain.Event) {
	e.mu.Lock()
	emitted := e.addEventLocked(event)
	e.mu.Unlock()
	e.dispatchEventSinks(emitted)
}

func (e *Engine) AddEventSink(sink func(domain.Event)) {
	if sink == nil {
		return
	}
	e.mu.Lock()
	e.eventSinks = append(e.eventSinks, sink)
	e.mu.Unlock()
}

func (e *Engine) AllowSymbol(symbol string) {
	e.gate.AllowSymbol(symbol)
}

func (e *Engine) SetAllowSymbols(symbols []string) {
	e.gate.SetAllowSymbols(symbols)
}

func (e *Engine) SetComplianceGate(gate *ComplianceGate) {
	e.mu.Lock()
	e.compliance = gate
	e.mu.Unlock()
}

func (e *Engine) SetComplianceSettlementStore(store ComplianceSettlementStore) error {
	e.mu.RLock()
	gate := e.compliance
	e.mu.RUnlock()
	if gate == nil {
		return nil
	}
	return gate.SetSettlementStore(store)
}

func (e *Engine) SetComplianceSettlementCalendar(calendar ComplianceSettlementCalendar) {
	e.mu.RLock()
	gate := e.compliance
	e.mu.RUnlock()
	if gate == nil {
		return
	}
	gate.SetSettlementCalendar(calendar)
}

func (e *Engine) ComplianceStatus() (*domain.ComplianceStatus, bool) {
	e.mu.RLock()
	gate := e.compliance
	e.mu.RUnlock()
	if gate == nil {
		return nil, false
	}
	status, ok := gate.Status()
	if !ok {
		return nil, false
	}
	out := status
	return &out, true
}

func (e *Engine) applyTradeUpdate(update domain.TradeUpdate) {
	var recordFill bool
	var fillSide domain.Side
	var fillQty float64
	var fillPrice float64
	var fillTime time.Time

	e.mu.Lock()
	ord, ok := e.orders[update.OrderID]
	if !ok {
		emitted := e.addEventLocked(domain.Event{
			Type:    "trade_update_unknown_order",
			Details: update.OrderID,
		})
		e.mu.Unlock()
		e.dispatchEventSinks(emitted)
		return
	}

	prevFilledQty := ord.FilledQty
	ord.Status = update.Status
	ord.FilledQty = update.FillQty
	ord.UpdatedAt = update.Time
	e.orders[ord.ID] = ord

	filledDelta := update.FillQty - prevFilledQty
	if filledDelta > 0 {
		recordFill = true
		fillSide = ord.Side
		fillQty = filledDelta
		fillPrice = estimateTradeUpdateFillPrice(ord, update, e.quotes[strings.ToUpper(strings.TrimSpace(ord.Symbol))])
		fillTime = update.Time
	}

	emitted := e.addEventLocked(domain.Event{
		Type:    "trade_update",
		Details: fmt.Sprintf("%s status=%s filled=%.2f", ord.ID, ord.Status, ord.FilledQty),
	})
	e.mu.Unlock()

	if recordFill && fillPrice > 0 && e.compliance != nil {
		if recordErr := e.compliance.RecordFill(fillSide, fillQty, fillPrice, fillTime); recordErr != nil {
			e.AddEvent("database_error", fmt.Sprintf("persist compliance settlement state: %v", recordErr))
		}
	}
	e.dispatchEventSinks(emitted)
}

func estimateOrderFillPrice(order domain.Order, quote domain.Quote) float64 {
	if order.LimitPrice != nil && *order.LimitPrice > 0 {
		return *order.LimitPrice
	}
	return quoteReferencePrice(order.Side, quote)
}

func estimateTradeUpdateFillPrice(order domain.Order, update domain.TradeUpdate, cachedQuote domain.Quote) float64 {
	if update.FillPrice != nil && *update.FillPrice > 0 {
		return *update.FillPrice
	}
	if order.LimitPrice != nil && *order.LimitPrice > 0 {
		return *order.LimitPrice
	}
	return quoteReferencePrice(order.Side, cachedQuote)
}

func quoteReferencePrice(side domain.Side, quote domain.Quote) float64 {
	if side == domain.SideBuy {
		if quote.Ask > 0 {
			return quote.Ask
		}
		if quote.Last > 0 {
			return quote.Last
		}
		return quote.Bid
	}
	if quote.Bid > 0 {
		return quote.Bid
	}
	if quote.Last > 0 {
		return quote.Last
	}
	return quote.Ask
}

func formatCompliancePostureDetails(status domain.ComplianceStatus) string {
	return eventmeta.EncodeCompliancePosture(status)
}

func formatComplianceDriftDetails(status domain.ComplianceStatus) string {
	return eventmeta.EncodeComplianceDrift(status)
}

func (e *Engine) addEventLocked(event domain.Event) domain.Event {
	event.Type = strings.TrimSpace(event.Type)
	event.Details = strings.TrimSpace(event.Details)
	event.RejectionReason = strings.TrimSpace(event.RejectionReason)
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}

	if len(e.events) == 0 || event.Type == "" {
		return event
	}
	idx := (e.eventStart + e.eventCount) % len(e.events)
	if e.eventCount == len(e.events) {
		idx = e.eventStart
		e.eventStart = (e.eventStart + 1) % len(e.events)
	} else {
		e.eventCount++
	}
	e.events[idx] = event
	return event
}

func (e *Engine) dispatchEventSinks(event domain.Event) {
	if event.Type == "" {
		return
	}
	e.mu.RLock()
	sinks := make([]func(domain.Event), len(e.eventSinks))
	copy(sinks, e.eventSinks)
	e.mu.RUnlock()
	for _, sink := range sinks {
		if sink == nil {
			continue
		}
		sink(event)
	}
}
