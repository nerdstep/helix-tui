package engine

import (
	"testing"

	"helix-tui/internal/domain"
)

func TestComplianceGateEvaluate_PDTAutoMarginBlocksBuyNearLimit(t *testing.T) {
	g := NewComplianceGate(CompliancePolicy{
		Enabled:         true,
		AccountType:     "margin",
		AvoidPDT:        true,
		MaxDayTrades5D:  3,
		MinEquityForPDT: 25000,
	})
	err := g.Evaluate(
		domain.OrderRequest{Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, Type: domain.OrderTypeMarket},
		domain.Quote{Last: 100},
		domain.Snapshot{
			Account: domain.Account{
				Equity:        10000,
				BuyingPower:   30000,
				Cash:          10000,
				DayTradeCount: 2,
			},
		},
	)
	if err == nil {
		t.Fatalf("expected pdt guard rejection")
	}
}

func TestComplianceGateEvaluate_DoesNotBlockSells(t *testing.T) {
	g := NewComplianceGate(CompliancePolicy{
		Enabled:         true,
		AccountType:     "margin",
		AvoidPDT:        true,
		MaxDayTrades5D:  3,
		MinEquityForPDT: 25000,
	})
	err := g.Evaluate(
		domain.OrderRequest{Symbol: "AAPL", Side: domain.SideSell, Qty: 1, Type: domain.OrderTypeMarket},
		domain.Quote{Last: 100},
		domain.Snapshot{
			Account: domain.Account{
				Equity:        10000,
				BuyingPower:   30000,
				Cash:          10000,
				DayTradeCount: 2,
			},
		},
	)
	if err != nil {
		t.Fatalf("expected sell to pass pdt guard, got %v", err)
	}
}

func TestComplianceGateEvaluate_CashAccountPassesPDTGuard(t *testing.T) {
	g := NewComplianceGate(CompliancePolicy{
		Enabled:         true,
		AccountType:     "cash",
		AvoidPDT:        true,
		MaxDayTrades5D:  3,
		MinEquityForPDT: 25000,
	})
	err := g.Evaluate(
		domain.OrderRequest{Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, Type: domain.OrderTypeMarket},
		domain.Quote{Last: 100},
		domain.Snapshot{
			Account: domain.Account{
				Equity:        10000,
				BuyingPower:   10000,
				Cash:          10000,
				DayTradeCount: 2,
				Multiplier:    1,
			},
		},
	)
	if err != nil {
		t.Fatalf("expected cash account to pass pdt guard, got %v", err)
	}
}

func TestComplianceGateEvaluate_PDTFlaggedBlocksBuyBelowMinEquity(t *testing.T) {
	g := NewComplianceGate(CompliancePolicy{
		Enabled:         true,
		AccountType:     "margin",
		AvoidPDT:        true,
		MaxDayTrades5D:  3,
		MinEquityForPDT: 25000,
	})
	err := g.Evaluate(
		domain.OrderRequest{Symbol: "AAPL", Side: domain.SideBuy, Qty: 1, Type: domain.OrderTypeMarket},
		domain.Quote{Last: 100},
		domain.Snapshot{
			Account: domain.Account{
				Equity:           12000,
				BuyingPower:      30000,
				Cash:             12000,
				PatternDayTrader: true,
			},
		},
	)
	if err == nil {
		t.Fatalf("expected flagged PDT account to be blocked below minimum equity")
	}
}
