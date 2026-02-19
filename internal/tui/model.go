package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
	"helix-tui/internal/symbols"
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
	quotes   map[string]domain.Quote
	quoteErr map[string]string
	err      error
}

type quitMsg struct{}

type Model struct {
	engine             *engine.Engine
	snapshot           domain.Snapshot
	watchlist          []string
	onWatchlistChanged func([]string) error
	onWatchlistSync    func([]string) ([]string, error)
	eventScroll        int
	quotes             map[string]domain.Quote
	prevLast           map[string]float64
	quoteErr           map[string]string
	input              string
	status             string
	statusError        bool
	width              int
	height             int
}

func New(engine *engine.Engine, watchlist ...string) Model {
	return Model{
		engine:    engine,
		watchlist: symbols.Normalize(watchlist),
		quotes:    map[string]domain.Quote{},
		prevLast:  map[string]float64{},
		quoteErr:  map[string]string{},
		status:    "Type 'help' for commands.",
	}
}

func (m Model) WithWatchlistChangeHandler(fn func([]string) error) Model {
	m.onWatchlistChanged = fn
	return m
}

func (m Model) WithWatchlistSyncHandler(fn func([]string) ([]string, error)) Model {
	m.onWatchlistSync = fn
	return m
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
		m.clampEventScroll()
		for symbol, q := range msg.quotes {
			if prev, ok := m.quotes[symbol]; ok {
				m.prevLast[symbol] = prev.Last
			}
			m.quotes[symbol] = q
			delete(m.quoteErr, symbol)
		}
		for symbol, errMsg := range msg.quoteErr {
			if errMsg == "" {
				continue
			}
			m.quoteErr[symbol] = errMsg
		}
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
		case tea.KeyPgUp:
			m.scrollEvents(m.eventPageSize())
			return m, nil
		case tea.KeyPgDown:
			m.scrollEvents(-m.eventPageSize())
			return m, nil
		case tea.KeyHome:
			m.setEventScroll(m.maxEventScroll())
			m.status = m.eventScrollStatus()
			m.statusError = false
			return m, nil
		case tea.KeyEnd:
			m.setEventScroll(0)
			m.status = m.eventScrollStatus()
			m.statusError = false
			return m, nil
		case tea.KeyEnter:
			input := strings.TrimSpace(m.input)
			m.input = ""
			if input == "" {
				return m, nil
			}
			if handled, cmd := m.handleWatchCommand(input); handled {
				return m, cmd
			}
			if handled, cmd := m.handleEventsCommand(input); handled {
				return m, cmd
			}
			return m, m.runCommand(input)
		}
	}
	return m, nil
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
		err := m.engine.SyncQuiet(ctx)
		if err != nil {
			return refreshMsg{err: err}
		}
		quotes := make(map[string]domain.Quote, len(m.watchlist))
		quoteErr := make(map[string]string, len(m.watchlist))
		for _, symbol := range m.watchlist {
			q, err := m.engine.GetQuote(ctx, symbol)
			if err != nil {
				quoteErr[symbol] = err.Error()
				continue
			}
			quotes[symbol] = q
		}
		return refreshMsg{
			snapshot: m.engine.Snapshot(),
			quotes:   quotes,
			quoteErr: quoteErr,
		}
	}
}
