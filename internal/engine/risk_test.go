package engine

import (
	"strings"
	"testing"
	"time"

	"helix-tui/internal/domain"
)

func TestRiskGateEvaluate_AllowsValidTradeAndEnforcesDailyLimit(t *testing.T) {
	gate := NewRiskGate(Policy{
		MaxNotionalPerTrade: 1000,
		MaxNotionalPerDay:   1500,
		AllowMarketOrders:   true,
		AllowSymbols: map[string]struct{}{
			"AAPL": {},
		},
	})
	quote := domain.Quote{Last: 100}

	if err := gate.Evaluate(domain.OrderRequest{
		Symbol: "aapl",
		Qty:    2,
		Type:   domain.OrderTypeMarket,
	}, quote); err != nil {
		t.Fatalf("first evaluate failed: %v", err)
	}
	if err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    10,
		Type:   domain.OrderTypeMarket,
	}, quote); err != nil {
		t.Fatalf("second evaluate failed: %v", err)
	}

	err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    4,
		Type:   domain.OrderTypeMarket,
	}, quote)
	if err == nil {
		t.Fatalf("expected daily limit error")
	}
	if !strings.Contains(err.Error(), "max per day") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRiskGateEvaluate_RejectsDisallowedSymbol(t *testing.T) {
	gate := NewRiskGate(Policy{
		AllowMarketOrders: true,
		AllowSymbols: map[string]struct{}{
			"AAPL": {},
		},
	})
	err := gate.Evaluate(domain.OrderRequest{
		Symbol: "MSFT",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 10})
	if err == nil {
		t.Fatalf("expected allowlist error")
	}
}

func TestRiskGateEvaluate_RejectsMarketWhenDisabled(t *testing.T) {
	gate := NewRiskGate(Policy{
		AllowMarketOrders: false,
	})
	err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 10})
	if err == nil {
		t.Fatalf("expected market order policy error")
	}
}

func TestRiskGateEvaluate_RejectsInvalidLimitPrice(t *testing.T) {
	gate := NewRiskGate(Policy{
		AllowMarketOrders: true,
	})
	err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeLimit,
	}, domain.Quote{Last: 10})
	if err == nil {
		t.Fatalf("expected limit price error")
	}
}

func TestRiskGateResetDaily(t *testing.T) {
	gate := NewRiskGate(Policy{
		MaxNotionalPerDay: 100,
		AllowMarketOrders: true,
	})

	if err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 100}); err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}
	err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 1})
	if err == nil {
		t.Fatalf("expected daily notional error before reset")
	}

	gate.ResetDaily()
	if err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 1}); err != nil {
		t.Fatalf("evaluate failed after reset: %v", err)
	}
}

func TestRiskGateAllowSymbol(t *testing.T) {
	gate := NewRiskGate(Policy{
		AllowMarketOrders: true,
		AllowSymbols: map[string]struct{}{
			"AAPL": {},
		},
	})
	err := gate.Evaluate(domain.OrderRequest{
		Symbol: "MSFT",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 10})
	if err == nil {
		t.Fatalf("expected allowlist error before AllowSymbol")
	}

	gate.AllowSymbol(" msft ")
	if err := gate.Evaluate(domain.OrderRequest{
		Symbol: "MSFT",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 10}); err != nil {
		t.Fatalf("expected symbol allowed after AllowSymbol, got %v", err)
	}
}

func TestRiskGateEvaluate_RejectsWhenAllowlistRequiredButEmpty(t *testing.T) {
	gate := NewRiskGate(Policy{
		AllowMarketOrders: true,
		AllowlistRequired: true,
	})
	err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 10})
	if err == nil || !strings.Contains(err.Error(), "allowlisted") {
		t.Fatalf("expected allowlist enforcement error, got %v", err)
	}
}

func TestRiskGateSetAllowSymbols_ReplacesAllowlist(t *testing.T) {
	gate := NewRiskGate(Policy{
		AllowMarketOrders: true,
		AllowlistRequired: true,
		AllowSymbols: map[string]struct{}{
			"AAPL": {},
		},
	})
	if err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 10}); err != nil {
		t.Fatalf("expected AAPL initially allowlisted, got %v", err)
	}

	gate.SetAllowSymbols([]string{"msft"})
	if err := gate.Evaluate(domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 10}); err == nil || !strings.Contains(err.Error(), "allowlisted") {
		t.Fatalf("expected AAPL rejected after allowlist replacement, got %v", err)
	}
	if err := gate.Evaluate(domain.OrderRequest{
		Symbol: "MSFT",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}, domain.Quote{Last: 10}); err != nil {
		t.Fatalf("expected MSFT allowlisted after replacement, got %v", err)
	}
}

func TestRiskGateRollback_ReclaimsDailyNotional(t *testing.T) {
	gate := NewRiskGate(Policy{
		MaxNotionalPerDay: 100,
		AllowMarketOrders: true,
	})
	req := domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}
	quote := domain.Quote{Last: 100}
	if err := gate.Evaluate(req, quote); err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}
	gate.Rollback(req, quote)
	if err := gate.Evaluate(req, quote); err != nil {
		t.Fatalf("expected evaluate to pass after rollback, got %v", err)
	}
}

func TestRiskGateEvaluate_AutoResetsDailyAcrossUTCDay(t *testing.T) {
	gate := NewRiskGate(Policy{
		MaxNotionalPerDay: 100,
		AllowMarketOrders: true,
	})
	now := time.Date(2026, 2, 24, 23, 0, 0, 0, time.UTC)
	gate.nowFn = func() time.Time { return now }
	req := domain.OrderRequest{
		Symbol: "AAPL",
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	}
	quote := domain.Quote{Last: 100}
	if err := gate.Evaluate(req, quote); err != nil {
		t.Fatalf("evaluate failed: %v", err)
	}

	now = now.Add(2 * time.Hour) // Next UTC day.
	if err := gate.Evaluate(req, quote); err != nil {
		t.Fatalf("expected daily notional reset on new UTC day, got %v", err)
	}
}
