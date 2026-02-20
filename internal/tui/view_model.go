package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/domain"
)

type viewModel struct {
	header      string
	account     string
	positions   []string
	orders      []string
	watchlist   []string
	pnl         []string
	momentum    []string
	system      []string
	events      []string
	status      string
	statusError bool
	input       string
	footer      string
}

func (m Model) buildViewModel() viewModel {
	return viewModel{
		header:      m.buildHeader(),
		account:     "",
		positions:   m.buildPositionRows(),
		orders:      m.buildOrderRows(),
		watchlist:   m.buildWatchRows(),
		pnl:         m.buildPnlRows(),
		momentum:    m.buildMomentumRows(),
		system:      m.buildSystemRows(),
		events:      m.buildEventRows(),
		status:      m.status,
		statusError: m.statusError,
		input:       m.input,
		footer:      footerStyle.Render("Commands: buy/sell/cancel/flatten/sync/watch/events/tab/help/q | tabs: Tab key (overview/logs/system) | log scroll: Up/Down/PgUp/PgDn/Home/End"),
	}
}

func (m Model) buildPositionRows() []string {
	rows := []string{panelTitleStyle.Render("Positions")}
	if len(m.snapshot.Positions) == 0 {
		return append(rows, mutedStyle.Render("(none)"))
	}
	view := strings.TrimRight(m.positionsTable.View(), "\n")
	if view == "" {
		for _, p := range m.snapshot.Positions {
			rows = append(rows, fmt.Sprintf("%-6s qty=%8.2f avg=%8.2f last=%8.2f", p.Symbol, p.Qty, p.AvgCost, p.LastPrice))
		}
		return rows
	}
	rows = append(rows, strings.Split(view, "\n")...)
	return rows
}

func (m Model) buildOrderRows() []string {
	rows := []string{panelTitleStyle.Render("Open Orders")}
	if len(m.snapshot.Orders) == 0 {
		return append(rows, mutedStyle.Render("(none)"))
	}
	view := strings.TrimRight(m.ordersTable.View(), "\n")
	if view == "" {
		for _, o := range m.snapshot.Orders {
			rows = append(rows, fmt.Sprintf("%-14s %-4s %-6s qty=%8.2f status=%s", o.ID, o.Side, o.Symbol, o.Qty, o.Status))
		}
		return rows
	}
	rows = append(rows, strings.Split(view, "\n")...)
	rows = append(rows, mutedStyle.Render("cancel: cancel #<row> or cancel <id-prefix>"))
	return rows
}

func (m Model) buildWatchRows() []string {
	rows := []string{panelTitleStyle.Render("Watchlist")}
	if len(m.watchlist) == 0 {
		return append(rows, mutedStyle.Render("(none configured)"))
	}
	view := strings.TrimRight(m.watchlistTable.View(), "\n")
	if view == "" {
		return append(rows, mutedStyle.Render("(loading...)"))
	}
	rows = append(rows, strings.Split(view, "\n")...)
	return rows
}

func (m Model) buildPnlRows() []string {
	rows := []string{panelTitleStyle.Render("Position P&L")}
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
		rows = append(rows, fmt.Sprintf("%-6s qty=%8.2f mark=%8.2f uPnL=%9s (%8s)", p.Symbol, p.Qty, mark, renderSignedCurrency(u), renderSignedPct(pct)))
	}
	rows = append(rows, fmt.Sprintf("Total uPnL=%s", renderSignedCurrency(totalUPNL)))
	return rows
}

func (m Model) buildMomentumRows() []string {
	rows := []string{panelTitleStyle.Render("Equity Momentum")}
	if len(m.equityHistory) < 3 {
		return append(rows, mutedStyle.Render("Momentum trend: collecting data..."))
	}

	chartWidth := 56
	if m.width > 0 {
		chartWidth = minInt(maxInt(28, m.width/2-12), 96)
	}
	chartHeight := 6

	momentum := make([]EquityPoint, 0, len(m.equityHistory)-1)
	for i := 1; i < len(m.equityHistory); i++ {
		prev := m.equityHistory[i-1]
		curr := m.equityHistory[i]
		momentum = append(momentum, EquityPoint{
			Time:   curr.Time,
			Equity: curr.Equity - prev.Equity,
		})
	}
	last := momentum[len(momentum)-1].Equity
	avg := 0.0
	for _, p := range momentum {
		avg += p.Equity
	}
	avg /= float64(len(momentum))

	chart := buildEquitySparkline(momentum, chartWidth, chartHeight, styleForSigned(last))
	rows = append(rows, fmt.Sprintf("Momentum trend (%d pts):", len(momentum)))
	rows = append(rows, strings.Split(chart, "\n")...)
	rows = append(rows, fmt.Sprintf("Last step=%s  Avg step=%s", renderSignedCurrency(last), renderSignedCurrency(avg)))
	return rows
}

func (m Model) buildEquityChartRows() []string {
	if len(m.equityHistory) < 2 {
		return []string{mutedStyle.Render("Equity trend: collecting data...")}
	}

	chartWidth := 56
	if m.width > 0 {
		chartWidth = minInt(maxInt(28, m.width/2-12), 96)
	}
	chartHeight := 6
	first := m.equityHistory[0]
	last := m.equityHistory[len(m.equityHistory)-1]
	delta := last.Equity - first.Equity
	pct := 0.0
	if first.Equity != 0 {
		pct = (delta / first.Equity) * 100
	}
	chart := buildEquitySparkline(m.equityHistory, chartWidth, chartHeight, styleForSigned(delta))

	rows := []string{fmt.Sprintf("Equity trend (%d pts):", len(m.equityHistory))}
	rows = append(rows, strings.Split(chart, "\n")...)
	rows = append(rows, fmt.Sprintf("Start=%.2f Last=%.2f Delta=%s (%s)", first.Equity, last.Equity, renderSignedCurrency(delta), renderSignedPct(pct)))
	return rows
}

func (m Model) buildSystemRows() []string {
	rows := []string{panelTitleStyle.Render("System")}
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
	rows := []string{panelTitleStyle.Render("Recent Events")}
	if len(m.snapshot.Events) == 0 {
		return append(rows, mutedStyle.Render("(none)"))
	}
	view := strings.TrimRight(m.eventsViewport.View(), "\n")
	if view != "" {
		rows = append(rows, strings.Split(view, "\n")...)
	}
	start, end, total := m.eventWindow()
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

func styleForSigned(v float64) lipgloss.Style {
	if v < 0 {
		return negativeStyle
	}
	return positiveStyle
}

func renderSignedCurrency(v float64) string {
	if v < 0 {
		return negativeStyle.Render(fmt.Sprintf("%+.2f", v))
	}
	return positiveStyle.Render(fmt.Sprintf("%+.2f", v))
}

func renderSignedPct(v float64) string {
	if v < 0 {
		return negativeStyle.Render(fmt.Sprintf("%+.2f%%", v))
	}
	return positiveStyle.Render(fmt.Sprintf("%+.2f%%", v))
}
