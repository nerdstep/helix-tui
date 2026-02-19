package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	inputStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("228"))
)

type tickMsg time.Time

type refreshMsg struct {
	snapshot domain.Snapshot
	quotes   map[string]domain.Quote
	quoteErr map[string]string
	err      error
}

type quitMsg struct{}

type Model struct {
	engine             *engine.Engine
	snapshot           domain.Snapshot
	watchlist          []string
	onWatchlistChanged func([]string) error
	onWatchlistSync    func([]string) ([]string, error)
	eventScroll        int
	quotes             map[string]domain.Quote
	prevLast           map[string]float64
	quoteErr           map[string]string
	input              string
	status             string
	statusError        bool
	width              int
	height             int
}

func New(engine *engine.Engine, watchlist ...string) Model {
	return Model{
		engine:    engine,
		watchlist: normalizeSymbols(watchlist),
		quotes:    map[string]domain.Quote{},
		prevLast:  map[string]float64{},
		quoteErr:  map[string]string{},
		status:    "Type 'help' for commands.",
	}
}

func (m Model) WithWatchlistChangeHandler(fn func([]string) error) Model {
	m.onWatchlistChanged = fn
	return m
}

func (m Model) WithWatchlistSyncHandler(fn func([]string) ([]string, error)) Model {
	m.onWatchlistSync = fn
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.refreshCmd(), tickCmd())
	case refreshMsg:
		if msg.err != nil {
			m.statusError = true
			m.status = msg.err.Error()
			return m, nil
		}
		m.snapshot = msg.snapshot
		m.clampEventScroll()
		for symbol, q := range msg.quotes {
			if prev, ok := m.quotes[symbol]; ok {
				m.prevLast[symbol] = prev.Last
			}
			m.quotes[symbol] = q
			delete(m.quoteErr, symbol)
		}
		for symbol, errMsg := range msg.quoteErr {
			if errMsg == "" {
				continue
			}
			m.quoteErr[symbol] = errMsg
		}
		return m, nil
	case statusOnlyMsg:
		m.status = msg.status
		m.statusError = msg.isErr
		if msg.refresh {
			return m, m.refreshCmd()
		}
		return m, nil
	case quitMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyRunes:
			m.input += msg.String()
			return m, nil
		case tea.KeyBackspace, tea.KeyCtrlH:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil
		case tea.KeyEsc:
			m.input = ""
			return m, nil
		case tea.KeyPgUp:
			m.scrollEvents(m.eventPageSize())
			return m, nil
		case tea.KeyPgDown:
			m.scrollEvents(-m.eventPageSize())
			return m, nil
		case tea.KeyHome:
			m.setEventScroll(m.maxEventScroll())
			m.status = m.eventScrollStatus()
			m.statusError = false
			return m, nil
		case tea.KeyEnd:
			m.setEventScroll(0)
			m.status = m.eventScrollStatus()
			m.statusError = false
			return m, nil
		case tea.KeyEnter:
			input := strings.TrimSpace(m.input)
			m.input = ""
			if input == "" {
				return m, nil
			}
			if handled, cmd := m.handleWatchCommand(input); handled {
				return m, cmd
			}
			if handled, cmd := m.handleEventsCommand(input); handled {
				return m, cmd
			}
			return m, m.runCommand(input)
		}
	}
	return m, nil
}

func (m Model) View() string {
	header := titleStyle.Render("helix-tui | CLI + TUI trading cockpit")
	account := fmt.Sprintf(
		"Cash: $%.2f  BuyingPower: $%.2f  Equity: $%.2f",
		m.snapshot.Account.Cash,
		m.snapshot.Account.BuyingPower,
		m.snapshot.Account.Equity,
	)

	posRows := []string{"Positions"}
	if len(m.snapshot.Positions) == 0 {
		posRows = append(posRows, mutedStyle.Render("(none)"))
	} else {
		for _, p := range m.snapshot.Positions {
			posRows = append(posRows, fmt.Sprintf("%-6s qty=%8.2f avg=%8.2f last=%8.2f", p.Symbol, p.Qty, p.AvgCost, p.LastPrice))
		}
	}

	orderRows := []string{"Open Orders"}
	if len(m.snapshot.Orders) == 0 {
		orderRows = append(orderRows, mutedStyle.Render("(none)"))
	} else {
		for _, o := range m.snapshot.Orders {
			orderRows = append(orderRows, fmt.Sprintf("%-14s %-4s %-6s qty=%8.2f status=%s", o.ID, o.Side, o.Symbol, o.Qty, o.Status))
		}
	}

	watchRows := []string{"Watchlist"}
	if len(m.watchlist) == 0 {
		watchRows = append(watchRows, mutedStyle.Render("(none configured)"))
	} else {
		for _, symbol := range m.watchlist {
			if errMsg, ok := m.quoteErr[symbol]; ok {
				watchRows = append(watchRows, fmt.Sprintf("%-6s error=%s", symbol, errMsg))
				continue
			}
			q, ok := m.quotes[symbol]
			if !ok {
				watchRows = append(watchRows, fmt.Sprintf("%-6s pending...", symbol))
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
			watchRows = append(
				watchRows,
				fmt.Sprintf("%-6s last=%8.2f bid=%8.2f ask=%8.2f spr=%6.2f chg=%8s%s", symbol, q.Last, q.Bid, q.Ask, spread, change, stale),
			)
		}
	}

	pnlRows := []string{"Position P&L"}
	totalUPNL := 0.0
	if len(m.snapshot.Positions) == 0 {
		pnlRows = append(pnlRows, mutedStyle.Render("(none)"))
	} else {
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
			pnlRows = append(
				pnlRows,
				fmt.Sprintf("%-6s qty=%8.2f mark=%8.2f uPnL=%+9.2f (%+6.2f%%)", p.Symbol, p.Qty, mark, u, pct),
			)
		}
		pnlRows = append(pnlRows, fmt.Sprintf("Total uPnL=%+.2f", totalUPNL))
	}

	systemRows := []string{"System"}
	systemRows = append(systemRows, fmt.Sprintf("watchlist=%d events=%d", len(m.watchlist), len(m.snapshot.Events)))
	if e := latestEventByType(m.snapshot.Events, "sync"); e != nil {
		systemRows = append(systemRows, fmt.Sprintf("last_sync=%s", formatLocalClock(e.Time)))
	}
	if e := latestEventByType(m.snapshot.Events, "agent_mode"); e != nil {
		systemRows = append(systemRows, e.Details)
	}
	if e := latestEventByType(m.snapshot.Events, "agent_runner_error"); e != nil {
		systemRows = append(systemRows, fmt.Sprintf("runner_error=%s", e.Details))
	}
	systemRows = append(
		systemRows,
		fmt.Sprintf(
			"agent executed=%d rejected=%d dry_run=%d",
			countEventsByType(m.snapshot.Events, "agent_intent_executed"),
			countEventsByType(m.snapshot.Events, "agent_intent_rejected"),
			countEventsByType(m.snapshot.Events, "agent_intent_dry_run"),
		),
	)
	if e := latestEventByType(m.snapshot.Events, "agent_cycle_complete"); e != nil {
		systemRows = append(systemRows, fmt.Sprintf("last_cycle=%s %s", formatLocalClock(e.Time), e.Details))
	}

	eventRows := []string{"Recent Events"}
	if len(m.snapshot.Events) == 0 {
		eventRows = append(eventRows, mutedStyle.Render("(none)"))
	} else {
		start, end, total := m.eventWindow()
		for _, e := range m.snapshot.Events[start:end] {
			eventRows = append(eventRows, fmt.Sprintf("%s %-18s %s", formatLocalClock(e.Time), e.Type, e.Details))
		}
		eventRows = append(eventRows, mutedStyle.Render(fmt.Sprintf("showing %d-%d of %d (events up/down/top/tail, PgUp/PgDn)", start+1, end, total)))
	}

	left := panelStyle.Render(strings.Join(posRows, "\n"))
	mid := panelStyle.Render(strings.Join(orderRows, "\n"))
	right := panelStyle.Render(strings.Join(watchRows, "\n"))
	top := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	midRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		panelStyle.Render(strings.Join(pnlRows, "\n")),
		panelStyle.Render(strings.Join(systemRows, "\n")),
	)

	statusRenderer := okStyle
	if m.statusError {
		statusRenderer = errStyle
	}
	status := statusRenderer.Render(m.status)
	input := inputStyle.Render("> " + m.input)

	return strings.Join([]string{
		header,
		account,
		top,
		midRow,
		panelStyle.Render(strings.Join(eventRows, "\n")),
		status,
		input,
		mutedStyle.Render("Commands: buy/sell/cancel/flatten/sync/watch/events/help/q"),
	}, "\n")
}

func tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := m.engine.SyncQuiet(ctx)
		if err != nil {
			return refreshMsg{err: err}
		}
		quotes := make(map[string]domain.Quote, len(m.watchlist))
		quoteErr := make(map[string]string, len(m.watchlist))
		for _, symbol := range m.watchlist {
			q, err := m.engine.GetQuote(ctx, symbol)
			if err != nil {
				quoteErr[symbol] = err.Error()
				continue
			}
			quotes[symbol] = q
		}
		return refreshMsg{
			snapshot: m.engine.Snapshot(),
			quotes:   quotes,
			quoteErr: quoteErr,
		}
	}
}

func (m Model) runCommand(raw string) tea.Cmd {
	return func() tea.Msg {
		args := strings.Fields(raw)
		if len(args) == 0 {
			return refreshMsg{snapshot: m.engine.Snapshot()}
		}
		cmd := strings.ToLower(args[0])

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		switch cmd {
		case "q", "quit", "exit":
			return quitMsg{}
		case "help":
			return statusOnlyMsg{status: "buy/sell/cancel/flatten/sync/watch/events (watch list|add|remove|sync, events up|down|top|tail)"}
		case "sync":
			if err := m.engine.Sync(ctx); err != nil {
				return statusOnlyMsg{status: err.Error(), isErr: true}
			}
			return statusOnlyMsg{status: "sync complete", refresh: true}
		case "flatten":
			if err := m.engine.Flatten(ctx); err != nil {
				return statusOnlyMsg{status: err.Error(), isErr: true}
			}
			return statusOnlyMsg{status: "flatten orders submitted", refresh: true}
		case "cancel":
			if len(args) != 2 {
				return statusOnlyMsg{status: "usage: cancel <ORDER_ID>", isErr: true}
			}
			if err := m.engine.CancelOrder(ctx, args[1]); err != nil {
				return statusOnlyMsg{status: err.Error(), isErr: true}
			}
			return statusOnlyMsg{status: "cancel requested", refresh: true}
		case "buy", "sell":
			if len(args) != 3 {
				return statusOnlyMsg{status: fmt.Sprintf("usage: %s <SYM> <QTY>", cmd), isErr: true}
			}
			qty, err := strconv.ParseFloat(args[2], 64)
			if err != nil || qty <= 0 {
				return statusOnlyMsg{status: "qty must be a positive number", isErr: true}
			}
			side := domain.SideBuy
			if cmd == "sell" {
				side = domain.SideSell
			}
			_, err = m.engine.PlaceOrder(ctx, domain.OrderRequest{
				Symbol: strings.ToUpper(args[1]),
				Side:   side,
				Qty:    qty,
				Type:   domain.OrderTypeMarket,
			})
			if err != nil {
				return statusOnlyMsg{status: err.Error(), isErr: true}
			}
			return statusOnlyMsg{status: "order submitted", refresh: true}
		default:
			return statusOnlyMsg{status: "unknown command; type help", isErr: true}
		}
	}
}

func (m *Model) handleWatchCommand(raw string) (bool, tea.Cmd) {
	args := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	if len(args) == 0 || args[0] != "watch" {
		return false, nil
	}
	if len(args) == 1 || args[1] == "list" {
		if len(m.watchlist) == 0 {
			m.status = "watchlist: (none)"
			m.statusError = false
			return true, nil
		}
		m.status = "watchlist: " + strings.Join(m.watchlist, ",")
		m.statusError = false
		return true, nil
	}
	if len(args) == 2 && (args[1] == "sync" || args[1] == "pull") {
		if m.onWatchlistSync == nil {
			m.status = "watchlist sync is not configured"
			m.statusError = true
			return true, nil
		}
		next, err := m.onWatchlistSync(m.watchlist)
		if err != nil {
			m.status = fmt.Sprintf("watchlist sync failed: %v", err)
			m.statusError = true
			return true, nil
		}
		next = normalizeSymbols(next)
		m.watchlist = next
		for _, symbol := range next {
			m.engine.AllowSymbol(symbol)
		}
		if len(next) == 0 {
			m.status = "watchlist synced: (none)"
		} else {
			m.status = "watchlist synced: " + strings.Join(next, ",")
		}
		m.statusError = false
		return true, m.refreshCmd()
	}
	if len(args) != 3 {
		m.status = "usage: watch <list|add|remove|sync> [SYM]"
		m.statusError = true
		return true, nil
	}
	symbols := normalizeSymbols([]string{args[2]})
	if len(symbols) == 0 {
		m.status = "symbol is required"
		m.statusError = true
		return true, nil
	}
	symbol := symbols[0]

	switch args[1] {
	case "add":
		for _, s := range m.watchlist {
			if s == symbol {
				m.status = fmt.Sprintf("%s already in watchlist", symbol)
				m.statusError = false
				return true, nil
			}
		}
		next := append(append([]string{}, m.watchlist...), symbol)
		if m.onWatchlistChanged != nil {
			if err := m.onWatchlistChanged(next); err != nil {
				m.status = fmt.Sprintf("watchlist sync failed: %v", err)
				m.statusError = true
				return true, nil
			}
		}
		m.watchlist = next
		m.engine.AllowSymbol(symbol)
		m.status = fmt.Sprintf("added %s to watchlist", symbol)
		m.statusError = false
		return true, m.refreshCmd()
	case "remove":
		if len(m.watchlist) == 0 {
			m.status = fmt.Sprintf("%s not in watchlist", symbol)
			m.statusError = true
			return true, nil
		}
		next := make([]string, 0, len(m.watchlist))
		removed := false
		for _, s := range m.watchlist {
			if s == symbol {
				removed = true
				continue
			}
			next = append(next, s)
		}
		if !removed {
			m.status = fmt.Sprintf("%s not in watchlist", symbol)
			m.statusError = true
			return true, nil
		}
		if m.onWatchlistChanged != nil {
			if err := m.onWatchlistChanged(next); err != nil {
				m.status = fmt.Sprintf("watchlist sync failed: %v", err)
				m.statusError = true
				return true, nil
			}
		}
		m.watchlist = next
		delete(m.quotes, symbol)
		delete(m.prevLast, symbol)
		delete(m.quoteErr, symbol)
		m.status = fmt.Sprintf("removed %s from watchlist", symbol)
		m.statusError = false
		return true, nil
	default:
		m.status = "usage: watch <list|add|remove|sync> [SYM]"
		m.statusError = true
		return true, nil
	}
}

func (m *Model) handleEventsCommand(raw string) (bool, tea.Cmd) {
	args := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	if len(args) == 0 || args[0] != "events" {
		return false, nil
	}
	if len(args) == 1 || args[1] == "tail" || args[1] == "latest" || args[1] == "end" {
		m.setEventScroll(0)
		m.status = m.eventScrollStatus()
		m.statusError = false
		return true, nil
	}
	if args[1] == "top" || args[1] == "oldest" {
		m.setEventScroll(m.maxEventScroll())
		m.status = m.eventScrollStatus()
		m.statusError = false
		return true, nil
	}
	step := m.eventPageSize()
	if len(args) == 3 {
		n, err := strconv.Atoi(args[2])
		if err != nil || n <= 0 {
			m.status = "usage: events <up|down|top|tail> [N]"
			m.statusError = true
			return true, nil
		}
		step = n
	}
	switch args[1] {
	case "up", "older", "back":
		m.scrollEvents(step)
		return true, nil
	case "down", "newer", "forward":
		m.scrollEvents(-step)
		return true, nil
	default:
		m.status = "usage: events <up|down|top|tail> [N]"
		m.statusError = true
		return true, nil
	}
}

type statusOnlyMsg struct {
	status  string
	isErr   bool
	refresh bool
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

func normalizeSymbols(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, symbol := range raw {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" {
			continue
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		out = append(out, symbol)
	}
	return out
}

func (m Model) eventPageSize() int {
	return 8
}

func (m Model) maxEventScroll() int {
	total := len(m.snapshot.Events)
	visible := m.eventPageSize()
	if total <= visible {
		return 0
	}
	return total - visible
}

func (m *Model) clampEventScroll() {
	if m.eventScroll < 0 {
		m.eventScroll = 0
		return
	}
	max := m.maxEventScroll()
	if m.eventScroll > max {
		m.eventScroll = max
	}
}

func (m *Model) setEventScroll(next int) {
	m.eventScroll = next
	m.clampEventScroll()
}

func (m *Model) scrollEvents(delta int) {
	m.setEventScroll(m.eventScroll + delta)
	m.status = m.eventScrollStatus()
	m.statusError = false
}

func (m Model) eventWindow() (start, end, total int) {
	total = len(m.snapshot.Events)
	if total == 0 {
		return 0, 0, 0
	}
	visible := m.eventPageSize()
	end = total - m.eventScroll
	if end < 0 {
		end = 0
	}
	start = end - visible
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	return start, end, total
}

func (m Model) eventScrollStatus() string {
	start, end, total := m.eventWindow()
	if total == 0 {
		return "events: (none)"
	}
	return fmt.Sprintf("events: showing %d-%d of %d", start+1, end, total)
}

func formatLocalClock(t time.Time) string {
	if t.IsZero() {
		return "00:00:00"
	}
	return t.Local().Format("15:04:05")
}
