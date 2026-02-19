package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/symbols"
)

const watchCommandUsage = "usage: watch <list|add|remove|sync> [SYM]"

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
	next, err := m.onWatchlistSync(m.watchlist)
	if err != nil {
		m.setStatus(fmt.Sprintf("watchlist sync failed: %v", err), true)
		return nil
	}
	next = symbols.Normalize(next)
	m.watchlist = next
	for _, symbol := range next {
		m.engine.AllowSymbol(symbol)
	}
	if len(next) == 0 {
		m.setStatus("watchlist synced: (none)", false)
	} else {
		m.setStatus("watchlist synced: "+strings.Join(next, ","), false)
	}
	return m.refreshCmd()
}

func (m *Model) handleWatchAdd(symbol string) tea.Cmd {
	for _, s := range m.watchlist {
		if s == symbol {
			m.setStatus(fmt.Sprintf("%s already in watchlist", symbol), false)
			return nil
		}
	}

	next := append(append([]string{}, m.watchlist...), symbol)
	if m.onWatchlistChanged != nil {
		if err := m.onWatchlistChanged(next); err != nil {
			m.setStatus(fmt.Sprintf("watchlist sync failed: %v", err), true)
			return nil
		}
	}
	m.watchlist = next
	m.engine.AllowSymbol(symbol)
	m.setStatus(fmt.Sprintf("added %s to watchlist", symbol), false)
	return m.refreshCmd()
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
	if m.onWatchlistChanged != nil {
		if err := m.onWatchlistChanged(next); err != nil {
			m.setStatus(fmt.Sprintf("watchlist sync failed: %v", err), true)
			return nil
		}
	}
	m.watchlist = next
	delete(m.quotes, symbol)
	delete(m.prevLast, symbol)
	delete(m.quoteErr, symbol)
	m.setStatus(fmt.Sprintf("removed %s from watchlist", symbol), false)
	return nil
}
