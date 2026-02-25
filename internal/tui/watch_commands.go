package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/symbols"
)

const watchCommandUsage = "usage: watch <list|add|remove|sync> [SYM]"

type watchCommandResultMsg struct {
	watchlist []string
	apply     bool
	status    string
	isErr     bool
	refresh   bool
}

func (m *Model) handleWatchCommand(raw string) (bool, tea.Cmd) {
	cmd, handled, parseErr := parseWatchCommand(raw)
	if !handled {
		return false, nil
	}
	if parseErr != nil {
		m.setStatus(parseErr.status, parseErr.isErr)
		return true, nil
	}
	switch cmd.Type {
	case watchCommandList:
		return true, m.handleWatchList()
	case watchCommandSync:
		return true, m.handleWatchSync()
	case watchCommandAdd:
		return true, m.handleWatchAdd(cmd.Symbol)
	case watchCommandRemove:
		return true, m.handleWatchRemove(cmd.Symbol)
	default:
		m.setStatus(watchCommandUsage, true)
		return true, nil
	}
}

func (m *Model) handleWatchList() tea.Cmd {
	if len(m.watchlist) == 0 {
		m.setStatus("watchlist: (none)", false)
		return nil
	}
	m.setStatus("watchlist: "+strings.Join(m.watchlist, ","), false)
	return nil
}

func (m *Model) handleWatchSync() tea.Cmd {
	if m.onWatchlistSync == nil {
		m.setStatus("watchlist sync is not configured", true)
		return nil
	}
	current := append([]string{}, m.watchlist...)
	syncFn := m.onWatchlistSync
	return m.startWatchAsync("running watch sync", func() watchCommandResultMsg {
		next, err := syncFn(current)
		if err != nil {
			return watchCommandResultMsg{
				status: fmt.Sprintf("watchlist sync failed: %v", err),
				isErr:  true,
			}
		}
		next = symbols.Normalize(next)
		status := "watchlist synced: (none)"
		if len(next) > 0 {
			status = "watchlist synced: " + strings.Join(next, ",")
		}
		return watchCommandResultMsg{
			watchlist: next,
			apply:     true,
			status:    status,
			refresh:   true,
		}
	})
}

func (m *Model) handleWatchAdd(symbol string) tea.Cmd {
	for _, s := range m.watchlist {
		if s == symbol {
			m.setStatus(fmt.Sprintf("%s already in watchlist", symbol), false)
			return nil
		}
	}

	next := append(append([]string{}, m.watchlist...), symbol)
	if m.onWatchlistChanged == nil {
		m.watchlist = next
		m.pruneWatchlistQuoteState()
		m.engine.SetAllowSymbols(next)
		m.syncWidgets()
		m.setStatus(fmt.Sprintf("added %s to watchlist", symbol), false)
		return m.refreshCmd()
	}
	changeFn := m.onWatchlistChanged
	return m.startWatchAsync("running watch add", func() watchCommandResultMsg {
		if err := changeFn(next); err != nil {
			return watchCommandResultMsg{
				status: fmt.Sprintf("watchlist sync failed: %v", err),
				isErr:  true,
			}
		}
		return watchCommandResultMsg{
			watchlist: next,
			apply:     true,
			status:    fmt.Sprintf("added %s to watchlist", symbol),
			refresh:   true,
		}
	})
}

func (m *Model) handleWatchRemove(symbol string) tea.Cmd {
	if len(m.watchlist) == 0 {
		m.setStatus(fmt.Sprintf("%s not in watchlist", symbol), true)
		return nil
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
		m.setStatus(fmt.Sprintf("%s not in watchlist", symbol), true)
		return nil
	}
	if m.onWatchlistChanged == nil {
		m.watchlist = next
		delete(m.quotes, symbol)
		delete(m.quoteSeenAt, symbol)
		delete(m.prevLast, symbol)
		delete(m.quoteErr, symbol)
		m.engine.SetAllowSymbols(next)
		m.syncWidgets()
		m.setStatus(fmt.Sprintf("removed %s from watchlist", symbol), false)
		return nil
	}
	changeFn := m.onWatchlistChanged
	return m.startWatchAsync("running watch remove", func() watchCommandResultMsg {
		if err := changeFn(next); err != nil {
			return watchCommandResultMsg{
				status: fmt.Sprintf("watchlist sync failed: %v", err),
				isErr:  true,
			}
		}
		return watchCommandResultMsg{
			watchlist: next,
			apply:     true,
			status:    fmt.Sprintf("removed %s from watchlist", symbol),
		}
	})
}

func (m *Model) startWatchAsync(label string, fn func() watchCommandResultMsg) tea.Cmd {
	m.startCommandLoading(label)
	return func() tea.Msg {
		return fn()
	}
}

func (m *Model) pruneWatchlistQuoteState() {
	allow := make(map[string]struct{}, len(m.watchlist))
	for _, symbol := range m.watchlist {
		allow[symbol] = struct{}{}
	}
	for symbol := range m.quotes {
		if _, ok := allow[symbol]; !ok {
			delete(m.quotes, symbol)
		}
	}
	for symbol := range m.quoteSeenAt {
		if _, ok := allow[symbol]; !ok {
			delete(m.quoteSeenAt, symbol)
		}
	}
	for symbol := range m.prevLast {
		if _, ok := allow[symbol]; !ok {
			delete(m.prevLast, symbol)
		}
	}
	for symbol := range m.quoteErr {
		if _, ok := allow[symbol]; !ok {
			delete(m.quoteErr, symbol)
		}
	}
}
