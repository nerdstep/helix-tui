package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

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

func TestComplianceGateEvaluate_GFVCashBlocksBuyUsingUnsettledProceeds(t *testing.T) {
	g := NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	})
	fillTime := time.Date(2026, time.February, 20, 10, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return fillTime }
	if err := g.RecordFill(domain.SideSell, 100, 10, fillTime); err != nil { // unsettled proceeds: 1000
		t.Fatalf("record fill failed: %v", err)
	}

	err := g.Evaluate(
		domain.OrderRequest{Symbol: "AAPL", Side: domain.SideBuy, Qty: 120, Type: domain.OrderTypeMarket},
		domain.Quote{Symbol: "AAPL", Ask: 10, Last: 10},
		domain.Snapshot{
			Account: domain.Account{
				Cash:        1500,
				BuyingPower: 1500,
				Multiplier:  1,
			},
		},
	)
	if err == nil {
		t.Fatalf("expected gfv guard rejection")
	}
	if !strings.Contains(err.Error(), "gfv guard") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplianceGateEvaluate_GFVCashAllowsAfterSettlement(t *testing.T) {
	g := NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	})
	fillTime := time.Date(2026, time.February, 20, 10, 0, 0, 0, time.UTC) // Friday
	g.now = func() time.Time { return fillTime }
	if err := g.RecordFill(domain.SideSell, 100, 10, fillTime); err != nil {
		t.Fatalf("record fill failed: %v", err)
	}

	// T+1 business-day settlement from Friday resolves on Monday.
	g.now = func() time.Time { return time.Date(2026, time.February, 23, 10, 0, 0, 0, time.UTC) }
	err := g.Evaluate(
		domain.OrderRequest{Symbol: "AAPL", Side: domain.SideBuy, Qty: 120, Type: domain.OrderTypeMarket},
		domain.Quote{Symbol: "AAPL", Ask: 10, Last: 10},
		domain.Snapshot{
			Account: domain.Account{
				Cash:        1500,
				BuyingPower: 1500,
				Multiplier:  1,
			},
		},
	)
	if err != nil {
		t.Fatalf("expected buy to pass after settlement, got %v", err)
	}
}

func TestComplianceGateEvaluate_GFVSkippedForMarginAccounts(t *testing.T) {
	g := NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "margin",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	})
	fillTime := time.Date(2026, time.February, 20, 10, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return fillTime }
	if err := g.RecordFill(domain.SideSell, 100, 10, fillTime); err != nil {
		t.Fatalf("record fill failed: %v", err)
	}

	err := g.Evaluate(
		domain.OrderRequest{Symbol: "AAPL", Side: domain.SideBuy, Qty: 500, Type: domain.OrderTypeMarket},
		domain.Quote{Symbol: "AAPL", Ask: 10, Last: 10},
		domain.Snapshot{
			Account: domain.Account{
				Cash:        500,
				BuyingPower: 10000,
				Multiplier:  2,
			},
		},
	)
	if err != nil {
		t.Fatalf("expected margin account to skip gfv guard, got %v", err)
	}
}

func TestSettlementTime_TPlusOneSkipsWeekend(t *testing.T) {
	friday := time.Date(2026, time.February, 20, 15, 30, 0, 0, time.UTC)
	settles := settlementTime(friday, 1)
	if settles.Weekday() != time.Monday {
		t.Fatalf("expected Monday settlement, got %s", settles.Weekday())
	}
}

func TestComplianceGate_SetSettlementStoreLoadsUnsettled(t *testing.T) {
	now := time.Date(2026, time.February, 21, 10, 0, 0, 0, time.UTC)
	store := &stubSettlementStore{
		lots: []UnsettledSellProceeds{
			{Amount: 900, SettlesAt: now.Add(24 * time.Hour)},
		},
	}
	g := NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	})
	g.now = func() time.Time { return now }
	if err := g.SetSettlementStore(store); err != nil {
		t.Fatalf("set settlement store failed: %v", err)
	}

	err := g.Evaluate(
		domain.OrderRequest{Symbol: "AAPL", Side: domain.SideBuy, Qty: 95, Type: domain.OrderTypeMarket},
		domain.Quote{Symbol: "AAPL", Ask: 10},
		domain.Snapshot{Account: domain.Account{Cash: 1000, BuyingPower: 1000, Multiplier: 1}},
	)
	if err == nil {
		t.Fatalf("expected gfv guard rejection from loaded unsettled state")
	}
}

func TestComplianceGate_RecordFillPersistsViaSettlementStore(t *testing.T) {
	now := time.Date(2026, time.February, 21, 10, 0, 0, 0, time.UTC)
	store := &stubSettlementStore{}
	g := NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	})
	g.now = func() time.Time { return now }
	if err := g.SetSettlementStore(store); err != nil {
		t.Fatalf("set settlement store failed: %v", err)
	}
	if err := g.RecordFill(domain.SideSell, 10, 15, now); err != nil {
		t.Fatalf("record fill failed: %v", err)
	}
	if len(store.appended) != 1 {
		t.Fatalf("expected 1 appended lot, got %d", len(store.appended))
	}
	if store.appended[0].Amount != 150 {
		t.Fatalf("unexpected appended amount: %#v", store.appended[0])
	}
}

func TestComplianceGate_RecordFillStoreErrorReturned(t *testing.T) {
	now := time.Date(2026, time.February, 21, 10, 0, 0, 0, time.UTC)
	store := &stubSettlementStore{appendErr: fmt.Errorf("boom")}
	g := NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	})
	g.now = func() time.Time { return now }
	if err := g.SetSettlementStore(store); err != nil {
		t.Fatalf("set settlement store failed: %v", err)
	}
	if err := g.RecordFill(domain.SideSell, 10, 15, now); err == nil {
		t.Fatalf("expected record fill error")
	}
}

func TestComplianceGate_RecordFillUsesSettlementCalendar(t *testing.T) {
	now := time.Date(2026, time.February, 21, 10, 0, 0, 0, time.UTC)
	calendar := &stubSettlementCalendar{
		settlesAt: time.Date(2026, time.February, 24, 0, 0, 0, 0, time.UTC),
	}
	g := NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 2,
	})
	g.SetSettlementCalendar(calendar)

	if err := g.RecordFill(domain.SideSell, 10, 15, now); err != nil {
		t.Fatalf("record fill failed: %v", err)
	}
	if len(g.unsettledSells) != 1 {
		t.Fatalf("expected 1 unsettled lot, got %d", len(g.unsettledSells))
	}
	if got := g.unsettledSells[0].SettlesAt; !got.Equal(calendar.settlesAt) {
		t.Fatalf("expected calendar settlement date %s, got %s", calendar.settlesAt, got)
	}
}

func TestComplianceGate_RecordFillCalendarErrorReturned(t *testing.T) {
	now := time.Date(2026, time.February, 21, 10, 0, 0, 0, time.UTC)
	g := NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	})
	g.SetSettlementCalendar(&stubSettlementCalendar{err: fmt.Errorf("calendar unavailable")})
	if err := g.RecordFill(domain.SideSell, 10, 15, now); err == nil {
		t.Fatalf("expected calendar error")
	}
}

func TestComplianceGate_ReconcileBrokerAccountBuildsStatus(t *testing.T) {
	now := time.Date(2026, time.February, 21, 10, 0, 0, 0, time.UTC)
	g := NewComplianceGate(CompliancePolicy{
		Enabled:         true,
		AccountType:     "cash",
		AvoidPDT:        true,
		MaxDayTrades5D:  3,
		MinEquityForPDT: 25000,
		AvoidGoodFaith:  true,
		SettlementDays:  1,
	})
	g.now = func() time.Time { return now }
	g.unsettledSells = []UnsettledSellProceeds{
		{Amount: 900, SettlesAt: now.Add(24 * time.Hour)},
	}
	account := domain.Account{
		Cash:             2000,
		BuyingPower:      1500, // implied unsettled 500
		Equity:           2000,
		PatternDayTrader: true,
		DayTradeCount:    2,
	}

	result := g.ReconcileBrokerAccount(account)
	if !result.PostureChanged {
		t.Fatalf("expected initial posture change")
	}
	if !result.DriftChanged {
		t.Fatalf("expected initial drift state change")
	}
	if !result.Status.UnsettledDriftDetected {
		t.Fatalf("expected drift detection from local/broker mismatch")
	}
	if result.Status.AccountType != "cash" {
		t.Fatalf("expected cash account type, got %q", result.Status.AccountType)
	}
	if result.Status.LocalUnsettledProceeds != 900 {
		t.Fatalf("unexpected local unsettled: %#v", result.Status)
	}
	if result.Status.BrokerUnsettledProceeds != 500 {
		t.Fatalf("unexpected broker unsettled: %#v", result.Status)
	}

	got, ok := g.Status()
	if !ok {
		t.Fatalf("expected compliance status snapshot")
	}
	if got.LastReconciledAt.IsZero() {
		t.Fatalf("expected reconciled timestamp")
	}
}

func TestComplianceGate_ReconcileBrokerAccountDriftClears(t *testing.T) {
	now := time.Date(2026, time.February, 21, 10, 0, 0, 0, time.UTC)
	g := NewComplianceGate(CompliancePolicy{
		Enabled:        true,
		AccountType:    "cash",
		AvoidGoodFaith: true,
		SettlementDays: 1,
	})
	g.now = func() time.Time { return now }
	g.unsettledSells = []UnsettledSellProceeds{
		{Amount: 900, SettlesAt: now.Add(24 * time.Hour)},
	}
	initial := g.ReconcileBrokerAccount(domain.Account{
		Cash:        2000,
		BuyingPower: 1500,
	})
	if !initial.Status.UnsettledDriftDetected {
		t.Fatalf("expected initial drift")
	}

	g.unsettledSells = []UnsettledSellProceeds{
		{Amount: 500, SettlesAt: now.Add(24 * time.Hour)},
	}
	next := g.ReconcileBrokerAccount(domain.Account{
		Cash:        2000,
		BuyingPower: 1500,
	})
	if next.Status.UnsettledDriftDetected {
		t.Fatalf("expected drift to clear")
	}
	if !next.DriftChanged {
		t.Fatalf("expected drift state change")
	}
}

type stubSettlementStore struct {
	lots      []UnsettledSellProceeds
	appended  []UnsettledSellProceeds
	loadErr   error
	appendErr error
	pruneErr  error
}

type stubSettlementCalendar struct {
	settlesAt time.Time
	err       error
}

func (s *stubSettlementCalendar) SettlementDate(_ time.Time, _ int) (time.Time, error) {
	if s.err != nil {
		return time.Time{}, s.err
	}
	return s.settlesAt, nil
}

func (s *stubSettlementStore) LoadUnsettledSellProceeds(_ time.Time) ([]UnsettledSellProceeds, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	out := make([]UnsettledSellProceeds, len(s.lots))
	copy(out, s.lots)
	return out, nil
}

func (s *stubSettlementStore) AppendUnsettledSellProceeds(lot UnsettledSellProceeds, _ time.Time) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	s.appended = append(s.appended, lot)
	return nil
}

func (s *stubSettlementStore) PruneSettledSellProceeds(_ time.Time) error {
	return s.pruneErr
}
