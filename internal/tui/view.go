package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/domain"
)

func (m Model) View() string {
	header := titleStyle.Render("helix-tui | CLI + TUI trading cockpit")
	account := fmt.Sprintf(
		"Cash: $%.2f  BuyingPower: $%.2f  Equity: $%.2f",
		m.snapshot.Account.Cash,
		m.snapshot.Account.BuyingPower,
		m.snapshot.Account.Equity,
	)

	posRows := []string{"Positions"}
	if len(m.snapshot.Positions) == 0 {
		posRows = append(posRows, mutedStyle.Render("(none)"))
	} else {
		for _, p := range m.snapshot.Positions {
			posRows = append(posRows, fmt.Sprintf("%-6s qty=%8.2f avg=%8.2f last=%8.2f", p.Symbol, p.Qty, p.AvgCost, p.LastPrice))
		}
	}

	orderRows := []string{"Open Orders"}
	if len(m.snapshot.Orders) == 0 {
		orderRows = append(orderRows, mutedStyle.Render("(none)"))
	} else {
		for _, o := range m.snapshot.Orders {
			orderRows = append(orderRows, fmt.Sprintf("%-14s %-4s %-6s qty=%8.2f status=%s", o.ID, o.Side, o.Symbol, o.Qty, o.Status))
		}
	}

	watchRows := []string{"Watchlist"}
	if len(m.watchlist) == 0 {
		watchRows = append(watchRows, mutedStyle.Render("(none configured)"))
	} else {
		for _, symbol := range m.watchlist {
			if errMsg, ok := m.quoteErr[symbol]; ok {
				watchRows = append(watchRows, fmt.Sprintf("%-6s error=%s", symbol, errMsg))
				continue
			}
			q, ok := m.quotes[symbol]
			if !ok {
				watchRows = append(watchRows, fmt.Sprintf("%-6s pending...", symbol))
				continue
			}
			spread := q.Ask - q.Bid
			change := "n/a"
			if prev, ok := m.prevLast[symbol]; ok && prev > 0 {
				change = fmt.Sprintf("%+.2f%%", ((q.Last-prev)/prev)*100)
			}
			stale := ""
			if !q.Time.IsZero() && time.Since(q.Time) > 15*time.Second {
				stale = " stale"
			}
			watchRows = append(
				watchRows,
				fmt.Sprintf("%-6s last=%8.2f bid=%8.2f ask=%8.2f spr=%6.2f chg=%8s%s", symbol, q.Last, q.Bid, q.Ask, spread, change, stale),
			)
		}
	}

	pnlRows := []string{"Position P&L"}
	totalUPNL := 0.0
	if len(m.snapshot.Positions) == 0 {
		pnlRows = append(pnlRows, mutedStyle.Render("(none)"))
	} else {
		for _, p := range m.snapshot.Positions {
			mark := p.LastPrice
			if q, ok := m.quotes[p.Symbol]; ok && q.Last > 0 {
				mark = q.Last
			}
			if mark <= 0 {
				mark = p.AvgCost
			}
			u := (mark - p.AvgCost) * p.Qty
			totalUPNL += u
			pct := 0.0
			if p.AvgCost > 0 {
				pct = ((mark - p.AvgCost) / p.AvgCost) * 100
			}
			pnlRows = append(
				pnlRows,
				fmt.Sprintf("%-6s qty=%8.2f mark=%8.2f uPnL=%+9.2f (%+6.2f%%)", p.Symbol, p.Qty, mark, u, pct),
			)
		}
		pnlRows = append(pnlRows, fmt.Sprintf("Total uPnL=%+.2f", totalUPNL))
	}

	systemRows := []string{"System"}
	systemRows = append(systemRows, fmt.Sprintf("watchlist=%d events=%d", len(m.watchlist), len(m.snapshot.Events)))
	if e := latestEventByType(m.snapshot.Events, "sync"); e != nil {
		systemRows = append(systemRows, fmt.Sprintf("last_sync=%s", formatLocalClock(e.Time)))
	}
	if e := latestEventByType(m.snapshot.Events, "agent_mode"); e != nil {
		systemRows = append(systemRows, e.Details)
	}
	if e := latestEventByType(m.snapshot.Events, "agent_runner_error"); e != nil {
		systemRows = append(systemRows, fmt.Sprintf("runner_error=%s", e.Details))
	}
	systemRows = append(
		systemRows,
		fmt.Sprintf(
			"agent executed=%d rejected=%d dry_run=%d",
			countEventsByType(m.snapshot.Events, "agent_intent_executed"),
			countEventsByType(m.snapshot.Events, "agent_intent_rejected"),
			countEventsByType(m.snapshot.Events, "agent_intent_dry_run"),
		),
	)
	if e := latestEventByType(m.snapshot.Events, "agent_cycle_complete"); e != nil {
		systemRows = append(systemRows, fmt.Sprintf("last_cycle=%s %s", formatLocalClock(e.Time), e.Details))
	}

	eventRows := []string{"Recent Events"}
	if len(m.snapshot.Events) == 0 {
		eventRows = append(eventRows, mutedStyle.Render("(none)"))
	} else {
		start, end, total := m.eventWindow()
		for _, e := range m.snapshot.Events[start:end] {
			eventRows = append(eventRows, fmt.Sprintf("%s %-18s %s", formatLocalClock(e.Time), e.Type, e.Details))
		}
		eventRows = append(eventRows, mutedStyle.Render(fmt.Sprintf("showing %d-%d of %d (events up/down/top/tail, PgUp/PgDn)", start+1, end, total)))
	}

	left := panelStyle.Render(strings.Join(posRows, "\n"))
	mid := panelStyle.Render(strings.Join(orderRows, "\n"))
	right := panelStyle.Render(strings.Join(watchRows, "\n"))
	top := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	midRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		panelStyle.Render(strings.Join(pnlRows, "\n")),
		panelStyle.Render(strings.Join(systemRows, "\n")),
	)

	statusRenderer := okStyle
	if m.statusError {
		statusRenderer = errStyle
	}
	status := statusRenderer.Render(m.status)
	input := inputStyle.Render("> " + m.input)

	return strings.Join([]string{
		header,
		account,
		top,
		midRow,
		panelStyle.Render(strings.Join(eventRows, "\n")),
		status,
		input,
		mutedStyle.Render("Commands: buy/sell/cancel/flatten/sync/watch/events/help/q"),
	}, "\n")
}

func latestEventByType(events []domain.Event, eventType string) *domain.Event {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == eventType {
			e := events[i]
			return &e
		}
	}
	return nil
}

func countEventsByType(events []domain.Event, eventType string) int {
	count := 0
	for _, e := range events {
		if e.Type == eventType {
			count++
		}
	}
	return count
}

func formatLocalClock(t time.Time) string {
	if t.IsZero() {
		return "00:00:00"
	}
	return t.Local().Format("15:04:05")
}
