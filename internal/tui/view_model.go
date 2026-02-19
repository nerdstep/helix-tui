package tui

import (
	"fmt"
	"time"

	"helix-tui/internal/domain"
)

type viewModel struct {
	header      string
	account     string
	positions   []string
	orders      []string
	watchlist   []string
	pnl         []string
	system      []string
	events      []string
	status      string
	statusError bool
	input       string
	footer      string
}

func (m Model) buildViewModel() viewModel {
	return viewModel{
		header:      titleStyle.Render("helix-tui | CLI + TUI trading cockpit"),
		account:     fmt.Sprintf("Cash: $%.2f  BuyingPower: $%.2f  Equity: $%.2f", m.snapshot.Account.Cash, m.snapshot.Account.BuyingPower, m.snapshot.Account.Equity),
		positions:   m.buildPositionRows(),
		orders:      m.buildOrderRows(),
		watchlist:   m.buildWatchRows(),
		pnl:         m.buildPnlRows(),
		system:      m.buildSystemRows(),
		events:      m.buildEventRows(),
		status:      m.status,
		statusError: m.statusError,
		input:       m.input,
		footer:      mutedStyle.Render("Commands: buy/sell/cancel/flatten/sync/watch/events/help/q"),
	}
}

func (m Model) buildPositionRows() []string {
	rows := []string{"Positions"}
	if len(m.snapshot.Positions) == 0 {
		return append(rows, mutedStyle.Render("(none)"))
	}
	for _, p := range m.snapshot.Positions {
		rows = append(rows, fmt.Sprintf("%-6s qty=%8.2f avg=%8.2f last=%8.2f", p.Symbol, p.Qty, p.AvgCost, p.LastPrice))
	}
	return rows
}

func (m Model) buildOrderRows() []string {
	rows := []string{"Open Orders"}
	if len(m.snapshot.Orders) == 0 {
		return append(rows, mutedStyle.Render("(none)"))
	}
	for _, o := range m.snapshot.Orders {
		rows = append(rows, fmt.Sprintf("%-14s %-4s %-6s qty=%8.2f status=%s", o.ID, o.Side, o.Symbol, o.Qty, o.Status))
	}
	return rows
}

func (m Model) buildWatchRows() []string {
	rows := []string{"Watchlist"}
	if len(m.watchlist) == 0 {
		return append(rows, mutedStyle.Render("(none configured)"))
	}
	for _, symbol := range m.watchlist {
		if errMsg, ok := m.quoteErr[symbol]; ok {
			rows = append(rows, fmt.Sprintf("%-6s error=%s", symbol, errMsg))
			continue
		}
		q, ok := m.quotes[symbol]
		if !ok {
			rows = append(rows, fmt.Sprintf("%-6s pending...", symbol))
			continue
		}
		spread := q.Ask - q.Bid
		change := "n/a"
		if prev, ok := m.prevLast[symbol]; ok && prev > 0 {
			change = fmt.Sprintf("%+.2f%%", ((q.Last-prev)/prev)*100)
		}
		stale := ""
		if !q.Time.IsZero() && time.Since(q.Time) > 15*time.Second {
			stale = " stale"
		}
		rows = append(rows, fmt.Sprintf("%-6s last=%8.2f bid=%8.2f ask=%8.2f spr=%6.2f chg=%8s%s", symbol, q.Last, q.Bid, q.Ask, spread, change, stale))
	}
	return rows
}

func (m Model) buildPnlRows() []string {
	rows := []string{"Position P&L"}
	rows = append(rows, m.buildEquityChartRows()...)
	if len(m.snapshot.Positions) == 0 {
		return append(rows, mutedStyle.Render("(no open positions)"))
	}

	totalUPNL := 0.0
	for _, p := range m.snapshot.Positions {
		mark := p.LastPrice
		if q, ok := m.quotes[p.Symbol]; ok && q.Last > 0 {
			mark = q.Last
		}
		if mark <= 0 {
			mark = p.AvgCost
		}
		u := (mark - p.AvgCost) * p.Qty
		totalUPNL += u
		pct := 0.0
		if p.AvgCost > 0 {
			pct = ((mark - p.AvgCost) / p.AvgCost) * 100
		}
		rows = append(rows, fmt.Sprintf("%-6s qty=%8.2f mark=%8.2f uPnL=%+9.2f (%+6.2f%%)", p.Symbol, p.Qty, mark, u, pct))
	}
	rows = append(rows, fmt.Sprintf("Total uPnL=%+.2f", totalUPNL))
	return rows
}

func (m Model) buildEquityChartRows() []string {
	if len(m.equityHistory) < 2 {
		return []string{mutedStyle.Render("Equity trend: collecting data...")}
	}

	chartWidth := 56
	if m.width > 0 {
		chartWidth = minInt(maxInt(24, m.width/3-6), 72)
	}
	chart := buildEquitySparkline(m.equityHistory, chartWidth)
	first := m.equityHistory[0]
	last := m.equityHistory[len(m.equityHistory)-1]
	delta := last.Equity - first.Equity
	pct := 0.0
	if first.Equity != 0 {
		pct = (delta / first.Equity) * 100
	}

	return []string{
		fmt.Sprintf("Equity trend (%d pts): %s", len(m.equityHistory), chart),
		fmt.Sprintf("Start=%.2f Last=%.2f Delta=%+.2f (%+.2f%%)", first.Equity, last.Equity, delta, pct),
	}
}

func (m Model) buildSystemRows() []string {
	rows := []string{"System"}
	rows = append(rows, fmt.Sprintf("watchlist=%d events=%d", len(m.watchlist), len(m.snapshot.Events)))
	if e := latestEventByType(m.snapshot.Events, "sync"); e != nil {
		rows = append(rows, fmt.Sprintf("last_sync=%s", formatLocalClock(e.Time)))
	}
	if e := latestEventByType(m.snapshot.Events, "agent_mode"); e != nil {
		rows = append(rows, e.Details)
	}
	cycles := countEventsByType(m.snapshot.Events, "agent_cycle_complete")
	rows = append(rows, fmt.Sprintf("agent cycles=%d", cycles))
	if e := latestEventByType(m.snapshot.Events, "agent_cycle_start"); e != nil {
		rows = append(rows, fmt.Sprintf("cycle_start=%s", formatLocalClock(e.Time)))
	}
	if e := latestEventByType(m.snapshot.Events, "agent_proposal"); e != nil {
		rows = append(rows, "last_proposal="+e.Details)
	}
	if e := latestEventByType(m.snapshot.Events, "agent_heartbeat"); e != nil {
		rows = append(rows, "heartbeat="+e.Details)
	}
	if e := latestEventByType(m.snapshot.Events, "agent_runner_error"); e != nil {
		rows = append(rows, fmt.Sprintf("runner_error=%s", e.Details))
	}
	if e := latestEventByType(m.snapshot.Events, "agent_cycle_error"); e != nil {
		rows = append(rows, fmt.Sprintf("last_error=%s %s", formatLocalClock(e.Time), e.Details))
	}
	rows = append(rows, fmt.Sprintf("agent executed=%d rejected=%d dry_run=%d", countEventsByType(m.snapshot.Events, "agent_intent_executed"), countEventsByType(m.snapshot.Events, "agent_intent_rejected"), countEventsByType(m.snapshot.Events, "agent_intent_dry_run")))
	if e := latestEventByType(m.snapshot.Events, "agent_cycle_complete"); e != nil {
		rows = append(rows, fmt.Sprintf("last_cycle=%s %s", formatLocalClock(e.Time), e.Details))
		rows = append(rows, fmt.Sprintf("last_cycle_age=%s", time.Since(e.Time).Round(time.Second)))
	}
	return rows
}

func (m Model) buildEventRows() []string {
	rows := []string{"Recent Events"}
	if len(m.snapshot.Events) == 0 {
		return append(rows, mutedStyle.Render("(none)"))
	}
	start, end, total := m.eventWindow()
	for _, e := range m.snapshot.Events[start:end] {
		rows = append(rows, fmt.Sprintf("%s %-18s %s", formatLocalClock(e.Time), e.Type, e.Details))
	}
	rows = append(rows, mutedStyle.Render(fmt.Sprintf("showing %d-%d of %d (events up/down/top/tail, PgUp/PgDn)", start+1, end, total)))
	return rows
}

func latestEventByType(events []domain.Event, eventType string) *domain.Event {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eventType {
			e := events[i]
			return &e
		}
	}
	return nil
}

func countEventsByType(events []domain.Event, eventType string) int {
	count := 0
	for _, e := range events {
		if e.Type == eventType {
			count++
		}
	}
	return count
}

func formatLocalClock(t time.Time) string {
	if t.IsZero() {
		return "00:00:00"
	}
	return t.Local().Format("15:04:05")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
