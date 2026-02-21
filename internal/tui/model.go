package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
	"helix-tui/internal/symbols"
)

var (
	headerLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("117")).
				Bold(true)
	headerValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230"))
	okStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("119"))
	errStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("203"))
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("33")).
			Padding(0, 1)
	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("159"))
	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229"))
	positiveStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	negativeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	footerStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("24")).
			Padding(0, 1)
	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 1)
)

type uiTab string

const (
	tabOverview uiTab = "overview"
	tabLogs     uiTab = "logs"
	tabSystem   uiTab = "system"
	tabStrategy uiTab = "strategy"
	minUIWidth        = 60
	minUIHeight       = 37
)

type tickMsg time.Time

type refreshMsg struct {
	snapshot    domain.Snapshot
	quotes      map[string]domain.Quote
	quoteErr    map[string]string
	strategy    StrategySnapshot
	strategyErr error
	err         error
}

type quitMsg struct{}

type Model struct {
	engine             *engine.Engine
	snapshot           domain.Snapshot
	watchlist          []string
	onWatchlistChanged func([]string) error
	onWatchlistSync    func([]string) ([]string, error)
	onStrategyRun      func() error
	onStrategyApprove  func(uint) error
	onStrategyReject   func(uint) error
	onStrategyArchive  func(uint) error
	onEquityPoint      func(EquityPoint) error
	onStrategyLoad     func() (StrategySnapshot, error)
	eventScroll        int
	eventsViewport     viewport.Model
	strategyViewport   viewport.Model
	positionsTable     table.Model
	ordersTable        table.Model
	watchlistTable     table.Model
	systemRuntimeTable table.Model
	systemAgentTable   table.Model
	systemPersistTable table.Model
	helpModel          help.Model
	helpKeys           dashboardKeyMap
	showFullHelp       bool
	quotes             map[string]domain.Quote
	quoteSeenAt        map[string]time.Time
	prevLast           map[string]float64
	quoteErr           map[string]string
	equityHistory      []EquityPoint
	equityMaxPoints    int
	strategy           StrategySnapshot
	strategyLoadError  string
	commandBusy        bool
	commandBusyLabel   string
	strategyBusy       bool
	strategyBusySince  time.Time
	spinner            spinner.Model
	input              string
	status             string
	statusError        bool
	activeTab          uiTab
	width              int
	height             int
}

func New(engine *engine.Engine, watchlist ...string) Model {
	spin := spinner.New()
	spin.Spinner = spinner.MiniDot
	spin.Style = warnStyle

	m := Model{
		engine:          engine,
		watchlist:       symbols.Normalize(watchlist),
		quotes:          map[string]domain.Quote{},
		quoteSeenAt:     map[string]time.Time{},
		prevLast:        map[string]float64{},
		quoteErr:        map[string]string{},
		equityMaxPoints: 1000,
		status:          "Type ? for commands.",
		activeTab:       tabOverview,
		spinner:         spin,
	}
	m.eventsViewport = viewport.New(1, m.eventPageSize())
	m.strategyViewport = viewport.New(1, m.strategyPageSize())
	m.positionsTable = newPositionsTable()
	m.ordersTable = newOrdersTable()
	m.watchlistTable = newWatchlistTable()
	m.systemRuntimeTable = newSystemTable()
	m.systemAgentTable = newSystemTable()
	m.systemPersistTable = newSystemTable()
	m.helpModel = help.New()
	m.helpKeys = newDashboardKeyMap()
	m.syncWidgets()
	return m
}

func (m Model) WithWatchlistChangeHandler(fn func([]string) error) Model {
	m.onWatchlistChanged = fn
	return m
}

func (m Model) WithWatchlistSyncHandler(fn func([]string) ([]string, error)) Model {
	m.onWatchlistSync = fn
	return m
}

func (m Model) WithStrategyRunHandler(fn func() error) Model {
	m.onStrategyRun = fn
	return m
}

func (m Model) WithStrategyApproveHandler(fn func(uint) error) Model {
	m.onStrategyApprove = fn
	return m
}

func (m Model) WithStrategyRejectHandler(fn func(uint) error) Model {
	m.onStrategyReject = fn
	return m
}

func (m Model) WithStrategyArchiveHandler(fn func(uint) error) Model {
	m.onStrategyArchive = fn
	return m
}

func (m Model) WithEquityHistory(points []EquityPoint, appendFn func(EquityPoint) error) Model {
	if points != nil {
		m.equityHistory = append([]EquityPoint{}, points...)
		if len(m.equityHistory) > m.equityMaxPoints {
			m.equityHistory = m.equityHistory[len(m.equityHistory)-m.equityMaxPoints:]
		}
	}
	m.onEquityPoint = appendFn
	return m
}

func (m Model) WithStrategyLoader(fn func() (StrategySnapshot, error)) Model {
	m.onStrategyLoad = fn
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd(), m.spinner.Tick)
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
		var strategySnapshot StrategySnapshot
		var strategyErr error
		if m.onStrategyLoad != nil {
			strategySnapshot, strategyErr = m.onStrategyLoad()
		}
		return refreshMsg{
			snapshot:    m.engine.Snapshot(),
			quotes:      quotes,
			quoteErr:    quoteErr,
			strategy:    strategySnapshot,
			strategyErr: strategyErr,
		}
	}
}
