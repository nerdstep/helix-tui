package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/domain"
)

type viewModel struct {
	header            string
	account           string
	positions         []string
	orders            []string
	watchlist         []string
	pnl               []string
	momentum          []string
	systemRuntime     []string
	systemAgent       []string
	systemPersistence []string
	events            []string
	status            string
	statusError       bool
	input             string
	footer            string
}

func (m Model) buildViewModel() viewModel {
	return viewModel{
		header:            m.buildHeader(),
		account:           "",
		positions:         m.buildPositionRows(),
		orders:            m.buildOrderRows(),
		watchlist:         m.buildWatchRows(),
		pnl:               m.buildPnlRows(),
		momentum:          m.buildMomentumRows(),
		systemRuntime:     m.buildSystemRuntimeRows(),
		systemAgent:       m.buildSystemAgentRows(),
		systemPersistence: m.buildSystemPersistenceRows(),
		events:            m.buildEventRows(),
		status:            m.status,
		statusError:       m.statusError,
		input:             m.input,
		footer:            footerStyle.Render("Commands: buy/sell/cancel/flatten/sync/watch/events/tab/help/q | tabs: Tab key (overview/logs/system)"),
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
			upnl := (p.LastPrice - p.AvgCost) * p.Qty
			rows = append(rows, fmt.Sprintf("%-6s qty=%8.2f avg=%8.2f last=%8.2f uPnL=%s", p.Symbol, p.Qty, p.AvgCost, p.LastPrice, renderSignedCurrency(upnl)))
		}
		return rows
	}
	view = colorizeTableColumns(view, m.positionsTable.Columns(), map[int]func(string) string{
		4: colorizeSignedValueCell,
	})
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
	view = colorizeTableColumns(view, m.ordersTable.Columns(), map[int]func(string) string{
		2: colorizeOrderSideCell,
	})
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
	view = colorizeTableColumns(view, m.watchlistTable.Columns(), map[int]func(string) string{
		5: colorizeWatchChangeCell,
		6: colorizeWatchStateCell,
	})
	rows = append(rows, strings.Split(view, "\n")...)
	return rows
}

func colorizeOrderSideCell(cell string) string {
	switch strings.ToUpper(strings.TrimSpace(cell)) {
	case "BUY":
		return positiveStyle.Render(cell)
	case "SELL":
		return negativeStyle.Render(cell)
	default:
		return cell
	}
}

func colorizeWatchChangeCell(cell string) string {
	v := strings.TrimSpace(cell)
	switch {
	case strings.HasPrefix(v, "+") && v != "+0.00%":
		return positiveStyle.Render(cell)
	case strings.HasPrefix(v, "-"):
		return negativeStyle.Render(cell)
	case v == "n/a" || v == "+0.00%":
		return mutedStyle.Render(cell)
	default:
		return cell
	}
}

func colorizeWatchStateCell(cell string) string {
	v := strings.ToLower(strings.TrimSpace(cell))
	switch {
	case strings.HasPrefix(v, "ok"):
		return positiveStyle.Render(cell)
	case strings.HasPrefix(v, "stale"):
		return warnStyle.Render(cell)
	case strings.HasPrefix(v, "pending"):
		return mutedStyle.Render(cell)
	case strings.HasPrefix(v, "error"):
		return errStyle.Render(cell)
	default:
		return cell
	}
}

func colorizeSignedValueCell(cell string) string {
	v := strings.TrimSpace(cell)
	switch {
	case strings.HasPrefix(v, "+") && v != "+0.00":
		return positiveStyle.Render(cell)
	case strings.HasPrefix(v, "-"):
		return negativeStyle.Render(cell)
	case v == "+0.00" || v == "0.00":
		return mutedStyle.Render(cell)
	default:
		return cell
	}
}

func (m Model) buildPnlRows() []string {
	rows := []string{panelTitleStyle.Render("Position P&L")}
	rows = append(rows, m.buildEquityChartRows()...)
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

type systemKV struct {
	key   string
	value string
}

func (m Model) buildSystemRuntimeRows() []string {
	mode := "unknown"
	if e := latestEventByType(m.snapshot.Events, "agent_mode"); e != nil && strings.TrimSpace(e.Details) != "" {
		mode = e.Details
	}
	lastSync := "n/a"
	if e := latestEventByType(m.snapshot.Events, "sync"); e != nil {
		lastSync = formatLocalClock(e.Time)
	}
	cycleStart := "n/a"
	if e := latestEventByType(m.snapshot.Events, "agent_cycle_start"); e != nil {
		cycleStart = formatLocalClock(e.Time)
	}
	lastCycleAge := "n/a"
	if e := latestEventByType(m.snapshot.Events, "agent_cycle_complete"); e != nil {
		lastCycleAge = time.Since(e.Time).Round(time.Second).String()
	}
	return buildSystemKVPairs(
		"Runtime",
		[]systemKV{
			{key: "watchlist", value: fmt.Sprintf("%d symbols", len(m.watchlist))},
			{key: "events", value: fmt.Sprintf("%d in-memory", len(m.snapshot.Events))},
			{key: "mode", value: mode},
			{key: "last sync", value: lastSync},
			{key: "cycle start", value: cycleStart},
			{key: "cycle age", value: lastCycleAge},
		},
	)
}

func (m Model) buildSystemAgentRows() []string {
	lastProposal := "n/a"
	if e := latestEventByType(m.snapshot.Events, "agent_proposal"); e != nil && strings.TrimSpace(e.Details) != "" {
		lastProposal = e.Details
	}
	heartbeat := "n/a"
	if e := latestEventByType(m.snapshot.Events, "agent_heartbeat"); e != nil && strings.TrimSpace(e.Details) != "" {
		heartbeat = e.Details
	}
	lastError := "none"
	if e := latestEventByType(m.snapshot.Events, "agent_cycle_error"); e != nil {
		lastError = fmt.Sprintf("%s %s", formatLocalClock(e.Time), e.Details)
	}
	return buildSystemKVPairs(
		"Agent",
		[]systemKV{
			{key: "cycles", value: fmt.Sprintf("%d", countEventsByType(m.snapshot.Events, "agent_cycle_complete"))},
			{key: "requests", value: fmt.Sprintf("ok=%d failed=%d", countEventsByType(m.snapshot.Events, "agent_proposal"), countEventsByType(m.snapshot.Events, "agent_cycle_error"))},
			{key: "intents", value: fmt.Sprintf("executed=%d rejected=%d dry_run=%d", countEventsByType(m.snapshot.Events, "agent_intent_executed"), countEventsByType(m.snapshot.Events, "agent_intent_rejected"), countEventsByType(m.snapshot.Events, "agent_intent_dry_run"))},
			{key: "last proposal", value: lastProposal},
			{key: "heartbeat", value: heartbeat},
			{key: "last error", value: lastError},
		},
	)
}

func (m Model) buildSystemPersistenceRows() []string {
	persistStats := "n/a"
	if e := latestEventByType(m.snapshot.Events, "event_persist_stats"); e != nil && strings.TrimSpace(e.Details) != "" {
		persistStats = e.Details
	}
	persistError := "none"
	if e := latestEventByType(m.snapshot.Events, "event_persist_error"); e != nil {
		persistError = fmt.Sprintf("%s %s", formatLocalClock(e.Time), e.Details)
	}
	lastCycle := "n/a"
	if e := latestEventByType(m.snapshot.Events, "agent_cycle_complete"); e != nil {
		lastCycle = fmt.Sprintf("%s %s", formatLocalClock(e.Time), e.Details)
	}
	runnerError := "none"
	if e := latestEventByType(m.snapshot.Events, "agent_runner_error"); e != nil && strings.TrimSpace(e.Details) != "" {
		runnerError = e.Details
	}
	return buildSystemKVPairs(
		"Persistence",
		[]systemKV{
			{key: "persist stats", value: persistStats},
			{key: "persist error", value: persistError},
			{key: "runner error", value: runnerError},
			{key: "last cycle", value: lastCycle},
		},
	)
}

func buildSystemKVPairs(title string, pairs []systemKV) []string {
	rows := []string{panelTitleStyle.Render(title)}
	if len(pairs) == 0 {
		return append(rows, mutedStyle.Render("(none)"))
	}
	keyWidth := 0
	for _, pair := range pairs {
		if len(pair.key) > keyWidth {
			keyWidth = len(pair.key)
		}
	}
	keyWidth = maxInt(keyWidth, 10)
	for _, pair := range pairs {
		key := mutedStyle.Render(fmt.Sprintf("%-*s", keyWidth, pair.key+":"))
		rows = append(rows, fmt.Sprintf("%s %s", key, pair.value))
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
	rows = append(rows, mutedStyle.Render(fmt.Sprintf("showing %d-%d of %d (events up/down/top/tail [N], Up/Down/PgUp/PgDn/Home/End)", start+1, end, total)))
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
