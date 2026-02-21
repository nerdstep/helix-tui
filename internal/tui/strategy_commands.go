package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const strategyCommandUsage = "usage: strategy <run|status>"

func (m *Model) handleStrategyCommand(raw string) (bool, tea.Cmd) {
	cmd, handled, parseErr := parseStrategyCommand(raw)
	if !handled {
		return false, nil
	}
	if parseErr != nil {
		m.setStatus(parseErr.status, parseErr.isErr)
		return true, nil
	}
	switch cmd.Type {
	case strategyCommandRun:
		if m.onStrategyRun == nil {
			m.setStatus("strategy runner is not configured", true)
			return true, nil
		}
		if err := m.onStrategyRun(); err != nil {
			m.setStatus(fmt.Sprintf("strategy run request failed: %v", err), true)
			return true, nil
		}
		m.startStrategyLoading()
		return true, tea.Batch(m.refreshCmd(), m.spinner.Tick)
	case strategyCommandStatus:
		return true, m.handleStrategyStatus()
	default:
		m.setStatus(strategyCommandUsage, true)
		return true, nil
	}
}

func (m *Model) handleStrategyStatus() tea.Cmd {
	if m.strategy.Active == nil {
		if m.strategyLoadError != "" {
			m.setStatus("strategy status error: "+m.strategyLoadError, true)
			return nil
		}
		m.setStatus("strategy status: no active plan", false)
		return nil
	}
	active := m.strategy.Active
	msg := fmt.Sprintf("strategy status: active plan #%d (%s) conf=%.2f model=%s", active.ID, strings.ToLower(strings.TrimSpace(active.Status)), active.Confidence, active.AnalystModel)
	m.setStatus(msg, false)
	return nil
}
