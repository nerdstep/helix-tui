package engine

import (
	"fmt"
	"strings"

	"helix-tui/internal/domain"
)

const (
	defaultPDTMinEquity   = 25000.0
	defaultPDTMaxTrades5D = 3
)

type CompliancePolicy struct {
	Enabled         bool
	AccountType     string
	AvoidPDT        bool
	MaxDayTrades5D  int
	MinEquityForPDT float64
	AvoidGoodFaith  bool
}

type ComplianceGate struct {
	policy CompliancePolicy
}

func NewComplianceGate(policy CompliancePolicy) *ComplianceGate {
	policy.AccountType = normalizeComplianceAccountType(policy.AccountType)
	if policy.MaxDayTrades5D <= 0 {
		policy.MaxDayTrades5D = defaultPDTMaxTrades5D
	}
	if policy.MinEquityForPDT <= 0 {
		policy.MinEquityForPDT = defaultPDTMinEquity
	}
	return &ComplianceGate{policy: policy}
}

func (g *ComplianceGate) Evaluate(req domain.OrderRequest, _ domain.Quote, snapshot domain.Snapshot) error {
	if g == nil || !g.policy.Enabled {
		return nil
	}
	if g.policy.AvoidPDT {
		if err := g.enforcePDT(req, snapshot.Account); err != nil {
			return err
		}
	}
	// Phase 2 (planned): enforce settled-cash checks to reduce GFV-like violations.
	return nil
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
