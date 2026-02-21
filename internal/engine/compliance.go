package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"helix-tui/internal/domain"
)

const (
	defaultPDTMinEquity   = 25000.0
	defaultPDTMaxTrades5D = 3
	defaultSettlementDays = 1
)

type CompliancePolicy struct {
	Enabled         bool
	AccountType     string
	AvoidPDT        bool
	MaxDayTrades5D  int
	MinEquityForPDT float64
	AvoidGoodFaith  bool
	SettlementDays  int
}

type ComplianceGate struct {
	policy CompliancePolicy

	mu             sync.Mutex
	unsettledSells []UnsettledSellProceeds
	store          ComplianceSettlementStore
	calendar       ComplianceSettlementCalendar
	now            func() time.Time
}

type UnsettledSellProceeds struct {
	Amount    float64
	SettlesAt time.Time
}

type ComplianceSettlementStore interface {
	LoadUnsettledSellProceeds(asOf time.Time) ([]UnsettledSellProceeds, error)
	AppendUnsettledSellProceeds(lot UnsettledSellProceeds, createdAt time.Time) error
	PruneSettledSellProceeds(asOf time.Time) error
}

type ComplianceSettlementCalendar interface {
	SettlementDate(fillTime time.Time, settlementDays int) (time.Time, error)
}

func NewComplianceGate(policy CompliancePolicy) *ComplianceGate {
	policy.AccountType = normalizeComplianceAccountType(policy.AccountType)
	if policy.MaxDayTrades5D <= 0 {
		policy.MaxDayTrades5D = defaultPDTMaxTrades5D
	}
	if policy.MinEquityForPDT <= 0 {
		policy.MinEquityForPDT = defaultPDTMinEquity
	}
	if policy.SettlementDays <= 0 {
		policy.SettlementDays = defaultSettlementDays
	}
	return &ComplianceGate{
		policy: policy,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (g *ComplianceGate) Evaluate(req domain.OrderRequest, quote domain.Quote, snapshot domain.Snapshot) error {
	if g == nil || !g.policy.Enabled {
		return nil
	}
	if g.policy.AvoidPDT {
		if err := g.enforcePDT(req, snapshot.Account); err != nil {
			return err
		}
	}
	if g.policy.AvoidGoodFaith {
		if err := g.enforceSettledCashForBuys(req, quote, snapshot.Account); err != nil {
			return err
		}
	}
	return nil
}

func (g *ComplianceGate) SetSettlementStore(store ComplianceSettlementStore) error {
	if g == nil {
		return nil
	}
	now := g.now()
	if store == nil {
		g.mu.Lock()
		g.store = nil
		g.mu.Unlock()
		return nil
	}
	if err := store.PruneSettledSellProceeds(now); err != nil {
		return err
	}
	lots, err := store.LoadUnsettledSellProceeds(now)
	if err != nil {
		return err
	}

	g.mu.Lock()
	g.store = store
	g.unsettledSells = make([]UnsettledSellProceeds, len(lots))
	copy(g.unsettledSells, lots)
	g.mu.Unlock()
	return nil
}

func (g *ComplianceGate) SetSettlementCalendar(calendar ComplianceSettlementCalendar) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.calendar = calendar
	g.mu.Unlock()
}

func (g *ComplianceGate) RecordFill(side domain.Side, qty, fillPrice float64, fillTime time.Time) error {
	if g == nil || !g.policy.Enabled || !g.policy.AvoidGoodFaith {
		return nil
	}
	if side != domain.SideSell || qty <= 0 || fillPrice <= 0 {
		return nil
	}
	settlesAt, err := g.resolveSettlementTime(nonZeroTime(fillTime, g.now()))
	if err != nil {
		return err
	}
	proceeds := qty * fillPrice
	if proceeds <= 0 {
		return nil
	}

	g.mu.Lock()
	g.pruneSettledLocked(g.now())
	lot := UnsettledSellProceeds{
		Amount:    proceeds,
		SettlesAt: settlesAt,
	}
	g.unsettledSells = append(g.unsettledSells, lot)
	store := g.store
	g.mu.Unlock()
	if store == nil {
		return nil
	}
	if err := store.AppendUnsettledSellProceeds(lot, nonZeroTime(fillTime, g.now())); err != nil {
		return err
	}
	return nil
}

func (g *ComplianceGate) resolveSettlementTime(fillTime time.Time) (time.Time, error) {
	g.mu.Lock()
	calendar := g.calendar
	g.mu.Unlock()
	if calendar == nil {
		return settlementTime(fillTime, g.policy.SettlementDays), nil
	}
	settlesAt, err := calendar.SettlementDate(fillTime, g.policy.SettlementDays)
	if err != nil {
		return time.Time{}, fmt.Errorf("resolve settlement date from calendar: %w", err)
	}
	if settlesAt.IsZero() {
		return time.Time{}, fmt.Errorf("resolve settlement date from calendar: zero settlement date")
	}
	return settlesAt.UTC(), nil
}

func (g *ComplianceGate) enforcePDT(req domain.OrderRequest, account domain.Account) error {
	if req.Side != domain.SideBuy {
		return nil
	}
	if !g.isMarginAccount(account) {
		return nil
	}
	maxTrades := g.policy.MaxDayTrades5D
	minEquity := g.policy.MinEquityForPDT
	if account.Equity >= minEquity {
		return nil
	}

	if account.PatternDayTrader {
		return fmt.Errorf(
			"pdt guard: account flagged as pattern day trader with equity %.2f below %.2f; buy orders blocked",
			account.Equity,
			minEquity,
		)
	}

	threshold := maxTrades - 1
	if threshold < 0 {
		threshold = 0
	}
	if account.DayTradeCount >= threshold {
		return fmt.Errorf(
			"pdt guard: day_trade_count=%d/%d and equity %.2f below %.2f; buy orders blocked",
			account.DayTradeCount,
			maxTrades,
			account.Equity,
			minEquity,
		)
	}
	return nil
}

func (g *ComplianceGate) enforceSettledCashForBuys(req domain.OrderRequest, quote domain.Quote, account domain.Account) error {
	if req.Side != domain.SideBuy {
		return nil
	}
	if g.isMarginAccount(account) {
		return nil
	}
	if req.Qty <= 0 {
		return fmt.Errorf("gfv guard: qty must be greater than 0")
	}
	price := g.referencePrice(req, quote)
	if price <= 0 {
		return fmt.Errorf("gfv guard: missing reference price")
	}

	unsettled := g.unsettledSellProceedsTotal()
	settledCash := account.Cash - unsettled
	if account.BuyingPower > 0 && account.BuyingPower < settledCash {
		settledCash = account.BuyingPower
	}
	if settledCash < 0 {
		settledCash = 0
	}

	notional := req.Qty * price
	if notional > settledCash+0.01 {
		return fmt.Errorf(
			"gfv guard: buy notional %.2f exceeds estimated settled cash %.2f (cash %.2f unsettled %.2f)",
			notional,
			settledCash,
			account.Cash,
			unsettled,
		)
	}
	return nil
}

func (g *ComplianceGate) referencePrice(req domain.OrderRequest, quote domain.Quote) float64 {
	if req.Type == domain.OrderTypeLimit && req.LimitPrice != nil && *req.LimitPrice > 0 {
		return *req.LimitPrice
	}
	if quote.Ask > 0 {
		return quote.Ask
	}
	if quote.Last > 0 {
		return quote.Last
	}
	if quote.Bid > 0 {
		return quote.Bid
	}
	return 0
}

func (g *ComplianceGate) unsettledSellProceedsTotal() float64 {
	if g == nil {
		return 0
	}
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pruneSettledLocked(now)
	total := 0.0
	for _, lot := range g.unsettledSells {
		total += lot.Amount
	}
	return total
}

func (g *ComplianceGate) pruneSettledLocked(now time.Time) {
	if len(g.unsettledSells) == 0 {
		return
	}
	kept := g.unsettledSells[:0]
	for _, lot := range g.unsettledSells {
		if lot.SettlesAt.After(now) {
			kept = append(kept, lot)
		}
	}
	g.unsettledSells = kept
}

func (g *ComplianceGate) isMarginAccount(account domain.Account) bool {
	switch normalizeComplianceAccountType(g.policy.AccountType) {
	case "margin":
		return true
	case "cash":
		return false
	default:
		if account.Multiplier > 1.0 {
			return true
		}
		return account.BuyingPower > account.Cash+0.01
	}
}

func settlementTime(fillTime time.Time, settlementDays int) time.Time {
	if settlementDays <= 0 {
		return fillTime
	}
	cursor := time.Date(fillTime.Year(), fillTime.Month(), fillTime.Day(), 0, 0, 0, 0, fillTime.Location())
	added := 0
	for added < settlementDays {
		cursor = cursor.AddDate(0, 0, 1)
		switch cursor.Weekday() {
		case time.Saturday, time.Sunday:
			continue
		default:
			added++
		}
	}
	return cursor
}

func nonZeroTime(t, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t
}

func normalizeComplianceAccountType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "margin":
		return "margin"
	case "cash":
		return "cash"
	default:
		return "auto"
	}
}
