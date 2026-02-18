package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	inputStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("228"))
)

type tickMsg time.Time

type refreshMsg struct {
	snapshot domain.Snapshot
	err      error
}

type quitMsg struct{}

type Model struct {
	engine      *engine.Engine
	snapshot    domain.Snapshot
	input       string
	status      string
	statusError bool
	width       int
	height      int
}

func New(engine *engine.Engine) Model {
	return Model{
		engine: engine,
		status: "Type 'help' for commands.",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.refreshCmd(), tickCmd())
	case refreshMsg:
		if msg.err != nil {
			m.statusError = true
			m.status = msg.err.Error()
			return m, nil
		}
		m.snapshot = msg.snapshot
		return m, nil
	case statusOnlyMsg:
		m.status = msg.status
		m.statusError = msg.isErr
		if msg.refresh {
			return m, m.refreshCmd()
		}
		return m, nil
	case quitMsg:
		return m, tea.Quit
	case tea.KeyMsg:
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
		case tea.KeyEnter:
			input := strings.TrimSpace(m.input)
			m.input = ""
			if input == "" {
				return m, nil
			}
			return m, m.runCommand(input)
		}
	}
	return m, nil
}

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

	eventRows := []string{"Recent Events"}
	if len(m.snapshot.Events) == 0 {
		eventRows = append(eventRows, mutedStyle.Render("(none)"))
	} else {
		start := 0
		if len(m.snapshot.Events) > 8 {
			start = len(m.snapshot.Events) - 8
		}
		for _, e := range m.snapshot.Events[start:] {
			eventRows = append(eventRows, fmt.Sprintf("%s %-18s %s", e.Time.Format("15:04:05"), e.Type, e.Details))
		}
	}

	left := panelStyle.Render(strings.Join(posRows, "\n"))
	right := panelStyle.Render(strings.Join(orderRows, "\n"))
	top := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

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
		panelStyle.Render(strings.Join(eventRows, "\n")),
		status,
		input,
		mutedStyle.Render("Commands: buy <SYM> <QTY> | sell <SYM> <QTY> | cancel <ORDER_ID> | flatten | sync | help | q"),
	}, "\n")
}

func tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := m.engine.Sync(ctx)
		if err != nil {
			return refreshMsg{err: err}
		}
		return refreshMsg{snapshot: m.engine.Snapshot()}
	}
}

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
			return statusOnlyMsg{status: "buy/sell/cancel/flatten/sync"}
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

type statusOnlyMsg struct {
	status  string
	isErr   bool
	refresh bool
}
