package tui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

const eventsCommandUsage = "usage: events <up|down|top|tail> [N]"

func (m *Model) handleEventsCommand(raw string) (bool, tea.Cmd) {
	cmd, handled, parseErr := parseEventsCommand(raw, m.eventPageSize())
	if !handled {
		return false, nil
	}
	if parseErr != nil {
		m.setStatus(parseErr.status, parseErr.isErr)
		return true, nil
	}
	switch cmd.Type {
	case eventsCommandTail:
		m.setEventScroll(0)
		m.setStatus(m.eventScrollStatus(), false)
		return true, nil
	case eventsCommandTop:
		m.setEventScroll(m.maxEventScroll())
		m.setStatus(m.eventScrollStatus(), false)
		return true, nil
	case eventsCommandUp:
		m.scrollEvents(cmd.Step)
		return true, nil
	case eventsCommandDown:
		m.scrollEvents(-cmd.Step)
		return true, nil
	default:
		m.setStatus(eventsCommandUsage, true)
		return true, nil
	}
}

func parseEventStep(args []string, defaultStep int) (int, bool) {
	step := defaultStep
	if len(args) == 2 {
		return step, true
	}
	if len(args) != 3 {
		return 0, false
	}
	n, err := strconv.Atoi(args[2])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
