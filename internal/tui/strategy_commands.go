package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const strategyCommandUsage = "usage: strategy <run|status|approve <id>|reject <id>|archive <id>>"

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
	case strategyCommandApprove:
		return true, m.handleStrategyPlanStatus(cmd.PlanID, "approve", m.onStrategyApprove)
	case strategyCommandReject:
		return true, m.handleStrategyPlanStatus(cmd.PlanID, "reject", m.onStrategyReject)
	case strategyCommandArchive:
		return true, m.handleStrategyPlanStatus(cmd.PlanID, "archive", m.onStrategyArchive)
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

func (m *Model) handleStrategyPlanStatus(planID uint, verb string, fn func(uint) error) tea.Cmd {
	if fn == nil {
		m.setStatus("strategy plan controls are not configured", true)
		return nil
	}
	if err := fn(planID); err != nil {
		m.setStatus(fmt.Sprintf("strategy %s failed for #%d: %v", verb, planID, err), true)
		return nil
	}
	m.setStatus(fmt.Sprintf("strategy %s #%d", verb, planID), false)
	return m.refreshCmd()
}
