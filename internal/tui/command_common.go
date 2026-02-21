package tui

import "strings"

type statusOnlyMsg struct {
	status  string
	isErr   bool
	refresh bool
}

func statusError(status string) *statusOnlyMsg {
	return &statusOnlyMsg{status: status, isErr: true}
}

func (m *Model) setStatus(status string, isErr bool) {
	m.status = status
	m.statusError = isErr
}

func lowerCommandArgs(raw string) []string {
	return strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
}
