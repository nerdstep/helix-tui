package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/symbols"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.updateWindowSize(msg)
	case tickMsg:
		return m.updateTick()
	case refreshMsg:
		return m.updateRefresh(msg)
	case statusOnlyMsg:
		return m.updateStatus(msg)
	case watchCommandResultMsg:
		return m.updateWatchCommandResult(msg)
	case spinner.TickMsg:
		return m.updateSpinner(msg)
	case quitMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m Model) updateWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.syncWidgets()
	return m, nil
}

func (m Model) updateTick() (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{m.refreshCmd(), tickCmd()}
	if m.isLoading() {
		cmds = append(cmds, m.spinner.Tick)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) updateRefresh(msg refreshMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.statusError = true
		m.status = msg.err.Error()
		return m, nil
	}

	m.snapshot = msg.snapshot
	m.recordEquityPoint(msg.snapshot.Account.Equity, time.Now().UTC())
	m.clampEventScroll()
	now := time.Now().UTC()
	for symbol, q := range msg.quotes {
		if prev, ok := m.quotes[symbol]; ok {
			m.prevLast[symbol] = prev.Last
		}
		m.quotes[symbol] = q
		m.quoteSeenAt[symbol] = now
		delete(m.quoteErr, symbol)
	}
	for symbol, errMsg := range msg.quoteErr {
		if errMsg == "" {
			continue
		}
		m.quoteErr[symbol] = errMsg
	}
	m.strategy = msg.strategy
	if msg.strategyErr != nil {
		m.strategyLoadError = msg.strategyErr.Error()
	} else {
		m.strategyLoadError = ""
	}
	m.reconcileStrategyLoading(time.Now().UTC())
	m.syncWidgets()
	if m.isLoading() {
		return m, m.spinner.Tick
	}
	return m, nil
}

func (m Model) updateStatus(msg statusOnlyMsg) (tea.Model, tea.Cmd) {
	m.clearCommandLoading()
	m.status = msg.status
	m.statusError = msg.isErr
	if msg.refresh {
		cmds := []tea.Cmd{m.refreshCmd()}
		if m.isLoading() {
			cmds = append(cmds, m.spinner.Tick)
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) updateSpinner(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if !m.isLoading() {
		return m, nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) updateWatchCommandResult(msg watchCommandResultMsg) (tea.Model, tea.Cmd) {
	m.clearCommandLoading()
	if msg.apply {
		next := symbols.Normalize(msg.watchlist)
		m.watchlist = next
		m.pruneWatchlistQuoteState()
		for _, symbol := range next {
			m.engine.AllowSymbol(symbol)
		}
		m.syncWidgets()
	}
	m.status = msg.status
	m.statusError = msg.isErr
	cmds := []tea.Cmd{}
	if msg.refresh {
		cmds = append(cmds, m.refreshCmd())
	}
	if m.isLoading() {
		cmds = append(cmds, m.spinner.Tick)
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	case tea.KeyTab:
		m.toggleTab()
		return m, nil
	case tea.KeyPgUp:
		m.scrollEvents(m.eventPageSize())
		return m, nil
	case tea.KeyPgDown:
		m.scrollEvents(-m.eventPageSize())
		return m, nil
	case tea.KeyUp:
		m.scrollEvents(1)
		return m, nil
	case tea.KeyDown:
		m.scrollEvents(-1)
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
		return m.submitInput()
	default:
		return m, nil
	}
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input)
	m.input = ""
	if input == "" {
		return m, nil
	}
	if handled, cmd := m.handleWatchCommand(input); handled {
		return m, cmd
	}
	if handled, cmd := m.handleTabCommand(input); handled {
		return m, cmd
	}
	if handled, cmd := m.handleEventsCommand(input); handled {
		return m, cmd
	}
	if handled, cmd := m.handleStrategyCommand(input); handled {
		return m, cmd
	}
	if shouldTrackCoreCommandLoading(input) {
		m.startCommandLoading("running " + coreCommandLabel(input))
		return m, m.runCommand(input)
	}
	return m, m.runCommand(input)
}
