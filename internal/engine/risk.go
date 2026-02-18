package engine

import (
	"fmt"
	"strings"
	"sync"

	"helix-tui/internal/domain"
)

type Policy struct {
	MaxNotionalPerTrade float64
	MaxNotionalPerDay   float64
	AllowMarketOrders   bool
	AllowSymbols        map[string]struct{}
}

type RiskGate struct {
	policy        Policy
	mu            sync.Mutex
	dailyNotional float64
}

func NewRiskGate(policy Policy) *RiskGate {
	return &RiskGate{policy: policy}
}

func (g *RiskGate) Evaluate(req domain.OrderRequest, quote domain.Quote) error {
	if req.Qty <= 0 {
		return fmt.Errorf("qty must be greater than 0")
	}

	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}

	if len(g.policy.AllowSymbols) > 0 {
		if _, ok := g.policy.AllowSymbols[symbol]; !ok {
			return fmt.Errorf("symbol %s is not allowlisted", symbol)
		}
	}

	if req.Type == domain.OrderTypeMarket && !g.policy.AllowMarketOrders {
		return fmt.Errorf("market orders are disabled by policy")
	}

	price := quote.Last
	if req.Type == domain.OrderTypeLimit {
		if req.LimitPrice == nil || *req.LimitPrice <= 0 {
			return fmt.Errorf("limit order requires positive limit price")
		}
		price = *req.LimitPrice
	}
	if price <= 0 {
		return fmt.Errorf("missing reference price for risk evaluation")
	}

	notional := req.Qty * price
	if g.policy.MaxNotionalPerTrade > 0 && notional > g.policy.MaxNotionalPerTrade {
		return fmt.Errorf("trade notional %.2f exceeds max per trade %.2f", notional, g.policy.MaxNotionalPerTrade)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.policy.MaxNotionalPerDay > 0 && g.dailyNotional+notional > g.policy.MaxNotionalPerDay {
		return fmt.Errorf("daily notional %.2f exceeds max per day %.2f", g.dailyNotional+notional, g.policy.MaxNotionalPerDay)
	}
	g.dailyNotional += notional
	return nil
}

func (g *RiskGate) ResetDaily() {
	g.mu.Lock()
	g.dailyNotional = 0
	g.mu.Unlock()
}
