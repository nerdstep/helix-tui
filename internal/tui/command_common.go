package tui

import "strings"

const helpCommandText = "buy/sell/cancel/flatten/sync/watch/events/tab (cancel <id|prefix|#row>, watch list|add|remove|sync, events up|down|top|tail, tab overview|logs|system)"

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
