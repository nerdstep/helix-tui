package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

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
	strategyOverview  []string
	strategyHealth    []string
	strategyPicks     []string
	strategyRecent    []string
	strategyChat      []string
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
		strategyOverview:  m.buildStrategyOverviewRows(),
		strategyHealth:    m.buildStrategyHealthRows(),
		strategyPicks:     m.buildStrategyRecommendationsRows(),
		strategyRecent:    m.buildStrategyRecentRows(),
		strategyChat:      m.buildStrategyChatRows(),
		events:            m.buildEventRows(),
		status:            m.status,
		statusError:       m.statusError,
		input:             m.input,
		footer:            m.buildFooter(),
	}
}

func (m Model) buildStrategyOverviewRows() []string {
	rows := []string{panelTitleStyle.Render("Strategy Plan")}
	if m.strategyLoadError != "" {
		rows = append(rows, errStyle.Render("load error: "+m.strategyLoadError))
	}
	active := m.strategy.Active
	if active == nil {
		rows = append(rows, mutedStyle.Render("(no active strategy plan)"))
		return rows
	}
	rows = append(rows, fmt.Sprintf("id=%d status=%s conf=%.2f", active.ID, active.Status, active.Confidence))
	rows = append(rows, fmt.Sprintf("generated=%s model=%s prompt=%s", active.GeneratedAt.Local().Format("2006-01-02 15:04:05"), active.AnalystModel, active.PromptVersion))
	if len(active.Watchlist) > 0 {
		rows = append(rows, "watchlist: "+strings.Join(active.Watchlist, ","))
	}
	if strings.TrimSpace(active.Summary) != "" {
		rows = append(rows, "summary: "+active.Summary)
	}
	rows = append(rows, "")
	rows = append(rows, headerLabelStyle.Render("Steering Context"))
	steering := m.strategy.Steering
	if steering == nil {
		rows = append(rows, mutedStyle.Render("(none)"))
		return rows
	}
	rows = append(rows, fmt.Sprintf("version=%d source=%s profile=%s", steering.Version, nonEmpty(steering.Source, "n/a"), nonEmpty(steering.RiskProfile, "n/a")))
	rows = append(rows, fmt.Sprintf("min_conf=%.2f max_pos_notional=%.2f horizon=%s", steering.MinConfidence, steering.MaxPositionNotional, nonEmpty(steering.Horizon, "n/a")))
	if len(steering.PreferredSymbols) > 0 {
		rows = append(rows, "preferred: "+strings.Join(steering.PreferredSymbols, ","))
	}
	if len(steering.ExcludedSymbols) > 0 {
		rows = append(rows, "excluded: "+strings.Join(steering.ExcludedSymbols, ","))
	}
	if strings.TrimSpace(steering.Objective) != "" {
		rows = append(rows, "objective: "+strings.TrimSpace(steering.Objective))
	}
	hash := strings.TrimSpace(steering.Hash)
	if hash == "" {
		hash = "n/a"
	}
	rows = append(rows, fmt.Sprintf("hash=%s updated=%s", hash, formatLocalClock(steering.UpdatedAt)))
	return rows
}

func (m Model) buildStrategyRecommendationsRows() []string {
	rows := []string{panelTitleStyle.Render("Recommendations")}
	view := strings.TrimRight(m.strategyViewport.View(), "\n")
	if view == "" {
		view = strings.Join(m.buildStrategyRecommendationBodyRows(), "\n")
	}
	if strings.TrimSpace(view) == "" {
		rows = append(rows, mutedStyle.Render("(none)"))
		return rows
	}
	rows = append(rows, strings.Split(view, "\n")...)
	start, end, total := m.strategyWindow()
	if total > maxInt(1, m.strategyViewport.VisibleLineCount()) {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("showing %d-%d of %d (Up/Down/PgUp/PgDn/Home/End)", start+1, end, total)))
	}
	return rows
}

func (m Model) buildStrategyRecommendationBodyRows() []string {
	rows := make([]string, 0)
	active := m.strategy.Active
	if active == nil || len(active.Recommendations) == 0 {
		rows = append(rows, mutedStyle.Render("Recommendations: (none)"))
	} else {
		rows = append(rows, headerLabelStyle.Render("Recommendations"))
		for _, rec := range active.Recommendations {
			head := fmt.Sprintf("%d) %s %-4s conf=%.2f max_notional=%.2f", rec.Priority, rec.Symbol, strings.ToUpper(rec.Bias), rec.Confidence, rec.MaxNotional)
			rows = append(rows, head)
			if rec.EntryMin > 0 || rec.EntryMax > 0 {
				rows = append(rows, fmt.Sprintf("entry=%.2f-%.2f target=%.2f stop=%.2f", rec.EntryMin, rec.EntryMax, rec.TargetPrice, rec.StopPrice))
			}
			if strings.TrimSpace(rec.Thesis) != "" {
				rows = append(rows, "thesis: "+rec.Thesis)
			}
			if strings.TrimSpace(rec.Invalidation) != "" {
				rows = append(rows, "invalid: "+rec.Invalidation)
			}
		}
	}
	return rows
}

func (m Model) buildStrategyChatRows() []string {
	rows := []string{panelTitleStyle.Render("Copilot Chat")}
	view := strings.TrimRight(m.strategyChatViewport.View(), "\n")
	if view == "" {
		view = strings.Join(m.buildStrategyChatBodyRows(), "\n")
	}
	if strings.TrimSpace(view) == "" {
		rows = append(rows, mutedStyle.Render("(none)"))
		return rows
	}
	rows = append(rows, strings.Split(view, "\n")...)
	start, end, total := m.strategyChatWindow()
	if total > maxInt(1, m.strategyChatViewport.VisibleLineCount()) {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("showing %d-%d of %d (Up/Down/PgUp/PgDn/Home/End)", start+1, end, total)))
	}
	return rows
}

func (m Model) buildStrategyChatBodyRows() []string {
	rows := make([]string, 0)
	thread := m.currentStrategyChatThread()
	if thread == nil {
		rows = append(rows, mutedStyle.Render("(no thread selected)"))
		rows = append(rows, mutedStyle.Render("Use: strategy chat new <title>"))
		return rows
	}
	rows = append(rows, fmt.Sprintf("thread #%d: %s", thread.ID, thread.Title))
	rows = append(rows, mutedStyle.Render("cmd: strategy chat say <message>"))
	rows = append(rows, "")

	if len(m.strategy.Chat.Messages) == 0 {
		rows = append(rows, mutedStyle.Render("(no messages)"))
		return rows
	}

	messageCount := 0
	for _, msg := range m.strategy.Chat.Messages {
		if msg.ThreadID != thread.ID {
			continue
		}
		messageCount++
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		roleLabel := strings.ToUpper(role)
		roleStyle := mutedStyle
		switch role {
		case "user":
			roleStyle = headerValueStyle
		case "assistant":
			roleStyle = positiveStyle
		case "system":
			roleStyle = warnStyle
		}
		timeLabel := msg.CreatedAt.Local().Format("01-02 15:04")
		rows = append(rows, fmt.Sprintf("%s %s", mutedStyle.Render(timeLabel), roleStyle.Render(roleLabel)))
		for _, wrapped := range wrapPlainTextRows(strings.TrimSpace(msg.Content), maxInt(24, panelInnerWidth(m.strategyChatPanelWidth())-2)) {
			rows = append(rows, "  "+wrapped)
		}
	}
	if messageCount == 0 {
		rows = append(rows, mutedStyle.Render("(no messages in selected thread)"))
	}
	return rows
}

func (m Model) buildStrategyRecentRows() []string {
	rows := []string{panelTitleStyle.Render("Recent Plans")}
	if len(m.strategy.Recent) == 0 {
		rows = append(rows, mutedStyle.Render("(none)"))
		return rows
	}
	for _, plan := range m.strategy.Recent {
		rows = append(rows, fmt.Sprintf("#%d %s status=%s conf=%.2f model=%s", plan.ID, plan.GeneratedAt.Local().Format("01-02 15:04"), plan.Status, plan.Confidence, plan.AnalystModel))
	}
	return rows
}

func (m Model) buildStrategyHealthRows() []string {
	rows := []string{panelTitleStyle.Render("Health")}
	mode := latestEventByType(m.snapshot.Events, "strategy_mode")
	if mode == nil {
		rows = append(rows, mutedStyle.Render("status: strategy disabled"))
		return rows
	}
	fields := parseEventFields(mode.Details)
	interval := time.Duration(0)
	if raw := strings.TrimSpace(fields["interval"]); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			interval = parsed
		}
	}

	lastStart := latestEventByType(m.snapshot.Events, "strategy_cycle_start")
	lastCreated := latestEventByType(m.snapshot.Events, "strategy_plan_created")
	lastUnchanged := latestEventByType(m.snapshot.Events, "strategy_plan_unchanged")
	lastErr := latestEventByType(m.snapshot.Events, "strategy_cycle_error")
	lastRunnerErr := latestEventByType(m.snapshot.Events, "strategy_runner_error")
	lastSuccess := lastCreated
	if newerEvent(lastUnchanged, lastSuccess) {
		lastSuccess = lastUnchanged
	}

	status := "ok"
	statusStyle := positiveStyle
	if lastSuccess == nil {
		status = "waiting_for_first_plan"
		statusStyle = warnStyle
	}
	if newerEvent(lastErr, lastSuccess) || lastRunnerErr != nil {
		status = "error"
		statusStyle = errStyle
	}

	stale := false
	if interval > 0 {
		if lastSuccess == nil {
			if lastStart != nil && time.Since(lastStart.Time) > interval {
				stale = true
			}
		} else if time.Since(lastSuccess.Time) > interval*2 {
			stale = true
		}
	}
	rows = append(rows, "status: "+statusStyle.Render(status))
	rows = append(rows, fmt.Sprintf("interval: %s", intervalString(interval)))
	rows = append(rows, fmt.Sprintf("stale: %t", stale))
	rows = append(rows, "last cycle: "+eventClock(lastStart))
	rows = append(rows, "last success: "+eventClock(lastSuccess))
	if lastErr != nil {
		rows = append(rows, "last error: "+fmt.Sprintf("%s %s", formatLocalClock(lastErr.Time), strings.TrimSpace(lastErr.Details)))
	} else if lastRunnerErr != nil {
		rows = append(rows, "last error: "+fmt.Sprintf("%s %s", formatLocalClock(lastRunnerErr.Time), strings.TrimSpace(lastRunnerErr.Details)))
	} else {
		rows = append(rows, "last error: none")
	}
	return rows
}

func newerEvent(a *domain.Event, b *domain.Event) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.Time.After(b.Time)
}

func eventClock(e *domain.Event) string {
	if e == nil {
		return "n/a"
	}
	age := time.Since(e.Time)
	if age < 0 {
		age = 0
	}
	return fmt.Sprintf("%s (%s ago)", formatLocalClock(e.Time), age.Round(time.Second))
}

func intervalString(d time.Duration) string {
	if d <= 0 {
		return "n/a"
	}
	return d.String()
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
			limit := "market"
			if o.LimitPrice != nil && *o.LimitPrice > 0 {
				limit = fmt.Sprintf("%.2f", *o.LimitPrice)
			}
			rows = append(rows, fmt.Sprintf("%-14s %-4s %-6s qty=%8.2f limit=%7s status=%s", o.ID, o.Side, o.Symbol, o.Qty, limit, o.Status))
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
	rows := []string{panelTitleStyle.Render("Runtime")}
	view := strings.TrimRight(m.systemRuntimeTable.View(), "\n")
	if view == "" {
		return append(rows, mutedStyle.Render("(loading...)"))
	}
	view = colorizeTableColumns(view, m.systemRuntimeTable.Columns(), map[int]func(string) string{
		1: colorizeSystemValueCell,
	})
	rows = append(rows, strings.Split(view, "\n")...)
	return rows
}

func (m Model) buildSystemAgentRows() []string {
	rows := []string{panelTitleStyle.Render("Agent")}
	view := strings.TrimRight(m.systemAgentTable.View(), "\n")
	if view == "" {
		return append(rows, mutedStyle.Render("(loading...)"))
	}
	view = colorizeTableColumns(view, m.systemAgentTable.Columns(), map[int]func(string) string{
		1: colorizeSystemValueCell,
	})
	rows = append(rows, strings.Split(view, "\n")...)
	return rows
}

func (m Model) buildSystemPersistenceRows() []string {
	rows := []string{panelTitleStyle.Render("Persistence")}
	view := strings.TrimRight(m.systemPersistTable.View(), "\n")
	if view == "" {
		return append(rows, mutedStyle.Render("(loading...)"))
	}
	view = colorizeTableColumns(view, m.systemPersistTable.Columns(), map[int]func(string) string{
		1: colorizeSystemValueCell,
	})
	rows = append(rows, strings.Split(view, "\n")...)
	return rows
}

func (m Model) systemRuntimeData() []systemKV {
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
	power := "active"
	if e := latestEventByType(m.snapshot.Events, "agent_power_state"); e != nil {
		fields := parseEventFields(e.Details)
		state := strings.TrimSpace(fields["state"])
		reason := strings.TrimSpace(fields["reason"])
		if state != "" {
			power = state
		}
		if reason != "" && reason != "market_open" {
			power = fmt.Sprintf("%s (%s)", power, reason)
		}
	}
	return []systemKV{
		{key: "watchlist", value: fmt.Sprintf("%d symbols", len(m.watchlist))},
		{key: "events", value: fmt.Sprintf("%d in-memory", len(m.snapshot.Events))},
		{key: "mode", value: mode},
		{key: "power", value: power},
		{key: "last sync", value: lastSync},
		{key: "cycle start", value: cycleStart},
		{key: "cycle age", value: lastCycleAge},
	}
}

func (m Model) systemAgentData() []systemKV {
	identity := "n/a"
	if e := latestEventByType(m.snapshot.Events, "identity_config"); e != nil {
		fields := parseEventFields(e.Details)
		agent := restoreEventField(fields["agent"])
		human := restoreEventField(fields["human"])
		alias := restoreEventField(fields["alias"])
		if agent != "" || human != "" || alias != "" {
			identity = fmt.Sprintf("agent=%s human=%s alias=%s", nonEmpty(identityValue(agent), "n/a"), nonEmpty(identityValue(human), "n/a"), nonEmpty(identityValue(alias), "n/a"))
		}
	}
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
	compliancePosture := m.compliancePostureSummary()
	complianceDrift := m.complianceDriftSummary()
	strategySummary := "none"
	if m.strategy.Active != nil {
		active := m.strategy.Active
		strategySummary = fmt.Sprintf(
			"active #%d status=%s recs=%d conf=%.2f",
			active.ID,
			strings.ToLower(strings.TrimSpace(active.Status)),
			len(active.Recommendations),
			active.Confidence,
		)
	}
	return []systemKV{
		{key: "identity", value: identity},
		{key: "cycles", value: fmt.Sprintf("%d", countEventsByType(m.snapshot.Events, "agent_cycle_complete"))},
		{key: "requests", value: fmt.Sprintf("ok=%d failed=%d", countEventsByType(m.snapshot.Events, "agent_proposal"), countEventsByType(m.snapshot.Events, "agent_cycle_error"))},
		{key: "intents", value: fmt.Sprintf("executed=%d rejected=%d dry_run=%d", countEventsByType(m.snapshot.Events, "agent_intent_executed"), countEventsByType(m.snapshot.Events, "agent_intent_rejected"), countEventsByType(m.snapshot.Events, "agent_intent_dry_run"))},
		{key: "compliance", value: fmt.Sprintf("rejected=%d", countEventsByType(m.snapshot.Events, "compliance_rejected"))},
		{key: "posture", value: compliancePosture},
		{key: "drift", value: complianceDrift},
		{key: "strategy", value: strategySummary},
		{key: "last proposal", value: lastProposal},
		{key: "heartbeat", value: heartbeat},
		{key: "last error", value: lastError},
	}
}

func (m Model) compliancePostureSummary() string {
	e := latestEventByType(m.snapshot.Events, "compliance_posture")
	if e == nil {
		return fmt.Sprintf(
			"acct=n/a pdt=%t day_trades=%d",
			m.snapshot.Account.PatternDayTrader,
			m.snapshot.Account.DayTradeCount,
		)
	}
	fields := parseEventFields(e.Details)
	accountType := strings.TrimSpace(fields["account_type"])
	if accountType == "" {
		accountType = "n/a"
	}
	pdt := strings.TrimSpace(fields["pdt"])
	if pdt == "" {
		pdt = "n/a"
	}
	dayTrades := strings.TrimSpace(fields["day_trades"])
	if dayTrades == "" {
		dayTrades = "n/a"
	}
	return fmt.Sprintf("acct=%s pdt=%s day_trades=%s", accountType, pdt, dayTrades)
}

func (m Model) complianceDriftSummary() string {
	detected := latestEventByType(m.snapshot.Events, "compliance_drift_detected")
	cleared := latestEventByType(m.snapshot.Events, "compliance_drift_cleared")
	active := detected
	state := "clear"
	if newerEvent(cleared, detected) {
		active = cleared
		state = "clear"
	} else if detected != nil {
		state = "detected"
	}
	if active == nil {
		return "clear"
	}
	fields := parseEventFields(active.Details)
	local := strings.TrimSpace(fields["local_unsettled"])
	broker := strings.TrimSpace(fields["broker_unsettled"])
	drift := strings.TrimSpace(fields["drift"])
	if local == "" && broker == "" && drift == "" {
		return state
	}
	return fmt.Sprintf("%s local=%s broker=%s delta=%s", state, nonEmpty(local, "n/a"), nonEmpty(broker, "n/a"), nonEmpty(drift, "n/a"))
}

func restoreEventField(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "_", " ")
}

func identityValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "n/a"
	}
	return value
}

func nonEmpty(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func (m Model) systemPersistenceData() []systemKV {
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
	return []systemKV{
		{key: "persist stats", value: persistStats},
		{key: "persist error", value: persistError},
		{key: "runner error", value: runnerError},
		{key: "last cycle", value: lastCycle},
	}
}

func (m Model) buildEventRows() []string {
	rows := []string{panelTitleStyle.Render("Recent Events")}
	if m.eventLineCount() == 0 {
		return append(rows, mutedStyle.Render("(none)"))
	}
	view := strings.TrimRight(m.eventsViewport.View(), "\n")
	if view != "" {
		rows = append(rows, strings.Split(view, "\n")...)
	}
	start, end, total := m.eventWindow()
	if total == 0 {
		return append(rows, mutedStyle.Render("(none)"))
	}
	rows = append(rows, m.singleLineMutedEventHint(start, end, total))
	return rows
}

func (m Model) singleLineMutedEventHint(start, end, total int) string {
	verbose := fmt.Sprintf("showing %d-%d of %d (events up/down/top/tail [N], Up/Down/PgUp/PgDn/Home/End)", start+1, end, total)
	medium := fmt.Sprintf("showing %d-%d of %d (events up/down/top/tail)", start+1, end, total)
	compact := fmt.Sprintf("%d-%d/%d", start+1, end, total)
	maxWidth := panelInnerWidth(m.eventsPanelWidth())
	if maxWidth <= 0 {
		return mutedStyle.Render(compact)
	}
	text := verbose
	if runewidth.StringWidth(text) > maxWidth {
		text = medium
	}
	if runewidth.StringWidth(text) > maxWidth {
		text = compact
	}
	if runewidth.StringWidth(text) > maxWidth {
		text = runewidth.Truncate(text, maxWidth, "…")
	}
	return mutedStyle.Render(text)
}

func colorizeSystemValueCell(cell string) string {
	return replaceNonSpaceTokens(cell, func(tok string) string {
		if strings.Contains(tok, "=") {
			return styleDetailToken(tok)
		}
		return styleDetailValue("", tok)
	})
}

func replaceNonSpaceTokens(s string, f func(string) string) string {
	if s == "" {
		return s
	}
	var out strings.Builder
	out.Grow(len(s) + 16)
	i := 0
	for i < len(s) {
		if s[i] == ' ' || s[i] == '\t' {
			out.WriteByte(s[i])
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '\t' {
			j++
		}
		out.WriteString(f(s[i:j]))
		i = j
	}
	return out.String()
}

func wrapPlainTextRows(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{""}
	}
	if width <= 8 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := make([]string, 0, 2)
	current := make([]string, 0, len(words))
	currentWidth := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		lines = append(lines, strings.Join(current, " "))
		current = current[:0]
		currentWidth = 0
	}
	for _, word := range words {
		wordWidth := runewidth.StringWidth(word)
		if wordWidth <= 0 {
			wordWidth = len(word)
		}
		sep := 0
		if len(current) > 0 {
			sep = 1
		}
		if len(current) > 0 && currentWidth+sep+wordWidth > width {
			flush()
		}
		if len(current) == 0 && wordWidth > width {
			remaining := word
			for runewidth.StringWidth(remaining) > width {
				part := runewidth.Truncate(remaining, width, "")
				if strings.TrimSpace(part) == "" {
					break
				}
				lines = append(lines, part)
				remaining = strings.TrimPrefix(remaining, part)
			}
			if strings.TrimSpace(remaining) != "" {
				current = append(current, remaining)
				currentWidth = runewidth.StringWidth(remaining)
			}
			continue
		}
		current = append(current, word)
		currentWidth += sep + wordWidth
	}
	flush()
	if len(lines) == 0 {
		return []string{text}
	}
	return lines
}

func (m Model) buildFooter() string {
	helper := m.helpModel
	helper.ShowAll = m.showFullHelp
	helper.Width = maxInt(1, m.computeLayoutSpec().usableWidth)
	helpText := strings.TrimSpace(helper.View(m.helpKeys))
	if helpText == "" {
		helpText = "? toggle help"
	}
	return footerStyle.Render(helpText)
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
