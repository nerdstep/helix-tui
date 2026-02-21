package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	return m, tea.Batch(m.refreshCmd(), tickCmd())
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
	m.syncWidgets()
	return m, nil
}

func (m Model) updateStatus(msg statusOnlyMsg) (tea.Model, tea.Cmd) {
	m.status = msg.status
	m.statusError = msg.isErr
	if msg.refresh {
		return m, m.refreshCmd()
	}
	return m, nil
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
	return m, m.runCommand(input)
}
