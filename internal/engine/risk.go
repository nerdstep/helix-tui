package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"helix-tui/internal/domain"
	"helix-tui/internal/util"
)

type Policy struct {
	MaxNotionalPerTrade float64
	MaxNotionalPerDay   float64
	AllowMarketOrders   bool
	AllowSymbols        map[string]struct{}
	AllowlistRequired   bool
}

type RiskGate struct {
	policy        Policy
	mu            sync.Mutex
	dailyNotional float64
	lastResetDay  time.Time
	nowFn         func() time.Time
}

func NewRiskGate(policy Policy) *RiskGate {
	return &RiskGate{
		policy: policy,
		nowFn:  func() time.Time { return time.Now().UTC() },
	}
}

func (g *RiskGate) Evaluate(req domain.OrderRequest, quote domain.Quote) error {
	if req.Qty <= 0 {
		return fmt.Errorf("qty must be greater than 0")
	}

	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.rolloverDailyLocked(g.currentDay())

	if g.allowlistEnforcedLocked() {
		if _, ok := g.policy.AllowSymbols[symbol]; !ok {
			return fmt.Errorf("symbol %s is not allowlisted", symbol)
		}
	}

	if req.Type == domain.OrderTypeMarket && !g.policy.AllowMarketOrders {
		return fmt.Errorf("market orders are disabled by policy")
	}

	notional, err := g.orderNotional(req, quote)
	if err != nil {
		return err
	}
	if g.policy.MaxNotionalPerTrade > 0 && notional > g.policy.MaxNotionalPerTrade {
		return fmt.Errorf("trade notional %.2f exceeds max per trade %.2f", notional, g.policy.MaxNotionalPerTrade)
	}

	if g.policy.MaxNotionalPerDay > 0 && g.dailyNotional+notional > g.policy.MaxNotionalPerDay {
		return fmt.Errorf("daily notional %.2f exceeds max per day %.2f", g.dailyNotional+notional, g.policy.MaxNotionalPerDay)
	}
	g.dailyNotional += notional
	return nil
}

func (g *RiskGate) Rollback(req domain.OrderRequest, quote domain.Quote) {
	if g == nil {
		return
	}
	notional, err := g.orderNotional(req, quote)
	if err != nil || notional <= 0 {
		return
	}
	g.mu.Lock()
	g.rolloverDailyLocked(g.currentDay())
	g.dailyNotional -= notional
	if g.dailyNotional < 0 {
		g.dailyNotional = 0
	}
	g.mu.Unlock()
}

func (g *RiskGate) AllowSymbol(symbol string) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return
	}
	g.mu.Lock()
	if g.policy.AllowSymbols == nil {
		g.policy.AllowSymbols = map[string]struct{}{}
	}
	g.policy.AllowlistRequired = true
	g.policy.AllowSymbols[symbol] = struct{}{}
	g.mu.Unlock()
}

func (g *RiskGate) SetAllowSymbols(symbols []string) {
	allow := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		normalized := strings.ToUpper(strings.TrimSpace(symbol))
		if normalized == "" {
			continue
		}
		allow[normalized] = struct{}{}
	}
	g.SetAllowSymbolSet(allow)
}

func (g *RiskGate) SetAllowSymbolSet(allow map[string]struct{}) {
	g.mu.Lock()
	g.policy.AllowlistRequired = true
	g.policy.AllowSymbols = make(map[string]struct{}, len(allow))
	for symbol := range allow {
		normalized := strings.ToUpper(strings.TrimSpace(symbol))
		if normalized == "" {
			continue
		}
		g.policy.AllowSymbols[normalized] = struct{}{}
	}
	g.mu.Unlock()
}

func (g *RiskGate) ResetDaily() {
	g.mu.Lock()
	g.dailyNotional = 0
	g.lastResetDay = g.currentDay()
	g.mu.Unlock()
}

func (g *RiskGate) allowlistEnforcedLocked() bool {
	return g.policy.AllowlistRequired || len(g.policy.AllowSymbols) > 0
}

func (g *RiskGate) currentDay() time.Time {
	nowFn := g.nowFn
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return util.DateAtUTCMidnight(nowFn())
}

func (g *RiskGate) rolloverDailyLocked(day time.Time) {
	if day.IsZero() {
		return
	}
	if g.lastResetDay.IsZero() {
		g.lastResetDay = day
		return
	}
	if day.Equal(g.lastResetDay) {
		return
	}
	g.dailyNotional = 0
	g.lastResetDay = day
}

func (g *RiskGate) orderNotional(req domain.OrderRequest, quote domain.Quote) (float64, error) {
	if req.Qty <= 0 {
		return 0, fmt.Errorf("qty must be greater than 0")
	}
	price := quote.Last
	if req.Type == domain.OrderTypeLimit {
		if req.LimitPrice == nil || *req.LimitPrice <= 0 {
			return 0, fmt.Errorf("limit order requires positive limit price")
		}
		price = *req.LimitPrice
	}
	if price <= 0 {
		return 0, fmt.Errorf("missing reference price for risk evaluation")
	}
	return req.Qty * price, nil
}
