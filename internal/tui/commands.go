package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/domain"
	"helix-tui/internal/symbols"
)

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
		next = symbols.Normalize(next)
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
	normalized := symbols.Normalize([]string{args[2]})
	if len(normalized) == 0 {
		m.status = "symbol is required"
		m.statusError = true
		return true, nil
	}
	symbol := normalized[0]

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
