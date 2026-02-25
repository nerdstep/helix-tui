package tui

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/broker/paper"
	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
	"helix-tui/internal/symbols"
)

func TestNewAndInit(t *testing.T) {
	m := New(newTestEngine(), "aapl", "AAPL", " msft ")
	if m.status == "" {
		t.Fatalf("expected initial status")
	}
	if len(m.watchlist) != 2 || m.watchlist[0] != "AAPL" || m.watchlist[1] != "MSFT" {
		t.Fatalf("unexpected watchlist normalization: %#v", m.watchlist)
	}
	if cmd := m.Init(); cmd == nil {
		t.Fatalf("expected init command")
	}
}

func TestUpdate_KeyInputFlow(t *testing.T) {
	m := New(newTestEngine())

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m2 := model.(Model)
	if cmd != nil {
		t.Fatalf("expected nil cmd for rune input")
	}
	if m2.input != "b" {
		t.Fatalf("unexpected input: %q", m2.input)
	}

	model, _ = m2.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m3 := model.(Model)
	if m3.input != "" {
		t.Fatalf("expected input cleared by backspace")
	}

	m3.input = "buy AAPL 1"
	model, cmd = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4 := model.(Model)
	if m4.input != "" {
		t.Fatalf("expected input cleared on enter")
	}
	if cmd == nil {
		t.Fatalf("expected command cmd")
	}
	msg := cmd()
	status, ok := msg.(statusOnlyMsg)
	if !ok || status.isErr {
		t.Fatalf("expected success status message, got %#v", msg)
	}
}

func TestUpdate_Messages(t *testing.T) {
	m := New(newTestEngine())

	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m2 := model.(Model)
	if m2.width != 120 || m2.height != 30 {
		t.Fatalf("unexpected size: %dx%d", m2.width, m2.height)
	}

	model, cmd := m2.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatalf("expected cmd on tick")
	}
	m3 := model.(Model)

	model, _ = m3.Update(refreshMsg{err: context.DeadlineExceeded})
	m4 := model.(Model)
	if !m4.statusError || !strings.Contains(m4.status, "deadline") {
		t.Fatalf("expected error status, got %q", m4.status)
	}

	snap := domain.Snapshot{Account: domain.Account{Cash: 1}}
	model, _ = m4.Update(refreshMsg{snapshot: snap})
	m5 := model.(Model)
	if m5.snapshot.Account.Cash != 1 {
		t.Fatalf("expected snapshot update")
	}

	model, cmd = m5.Update(statusOnlyMsg{status: "ok", refresh: true})
	if cmd == nil {
		t.Fatalf("expected refresh cmd")
	}
	m6 := model.(Model)
	if m6.status != "ok" {
		t.Fatalf("expected status update")
	}

	_, cmd = m6.Update(quitMsg{})
	if cmd == nil {
		t.Fatalf("expected quit cmd")
	}
}

func TestRunCommandCoverage(t *testing.T) {
	m := New(newTestEngine())

	tests := []struct {
		name    string
		raw     string
		wantErr bool
		wantSub string
	}{
		{name: "help removed", raw: "help", wantErr: true, wantSub: "use ? for help"},
		{name: "unknown", raw: "xyz", wantErr: true, wantSub: "unknown command"},
		{name: "cancel usage", raw: "cancel", wantErr: true, wantSub: "usage: cancel"},
		{name: "buy usage", raw: "buy AAPL", wantErr: true, wantSub: "usage: buy"},
		{name: "buy qty invalid", raw: "buy AAPL nope", wantErr: true, wantSub: "qty must"},
		{name: "sync", raw: "sync", wantSub: "sync complete"},
		{name: "flatten", raw: "flatten", wantSub: "flatten orders submitted"},
		{name: "buy", raw: "buy AAPL 1", wantSub: "order submitted"},
		{name: "sell", raw: "sell AAPL 1", wantSub: "order submitted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := m.runCommand(tt.raw)
			if cmd == nil {
				t.Fatalf("expected cmd")
			}
			msg := cmd()
			status, ok := msg.(statusOnlyMsg)
			if !ok {
				t.Fatalf("expected statusOnlyMsg, got %#v", msg)
			}
			if tt.wantErr != status.isErr {
				t.Fatalf("unexpected isErr: got %v want %v", status.isErr, tt.wantErr)
			}
			if !strings.Contains(status.status, tt.wantSub) {
				t.Fatalf("expected %q to contain %q", status.status, tt.wantSub)
			}
		})
	}

	quitMsgVal := m.runCommand("quit")()
	if _, ok := quitMsgVal.(quitMsg); !ok {
		t.Fatalf("expected quitMsg, got %#v", quitMsgVal)
	}
}

func TestRefreshCmdAndView(t *testing.T) {
	e := newTestEngine()
	m := New(e, "AAPL", "MSFT")
	msg := m.refreshCmd()()
	r, ok := msg.(refreshMsg)
	if !ok {
		t.Fatalf("expected refreshMsg, got %#v", msg)
	}
	if r.err != nil {
		t.Fatalf("unexpected refresh error: %v", r.err)
	}

	m.snapshot = r.snapshot
	m.quotes = r.quotes
	m.quoteErr = r.quoteErr
	m.snapshot.Positions = []domain.Position{
		{Symbol: "AAPL", Qty: 1, AvgCost: 90, LastPrice: 100},
	}
	m.snapshot.Events = append(m.snapshot.Events, domain.Event{
		Time:    time.Now().UTC(),
		Type:    "agent_cycle_complete",
		Details: "generated=1 attempted=1 executed=1 rejected=0 approvals=0 dry_run=0 skipped=0",
	})
	view := m.View()
	if !strings.Contains(view, "Cash:") || !strings.Contains(view, "? toggle help") {
		t.Fatalf("unexpected view output: %q", view)
	}
	if !strings.Contains(view, "Overview") || !strings.Contains(view, "Logs") || !strings.Contains(view, "Strategy") || !strings.Contains(view, "Chat") || !strings.Contains(view, "System") {
		t.Fatalf("expected tabs in view output: %q", view)
	}
	if !strings.Contains(view, "Watchlist") || !strings.Contains(view, "AAPL") {
		t.Fatalf("expected watchlist panel in view output: %q", view)
	}
	if !strings.Contains(view, "Position P&L") || !strings.Contains(view, "uPnL") {
		t.Fatalf("expected pnl panel in view output: %q", view)
	}
	if !strings.Contains(view, "Equity Momentum") {
		t.Fatalf("expected momentum panel in view output: %q", view)
	}
	if !strings.Contains(view, "Equity trend") {
		t.Fatalf("expected equity trend output: %q", view)
	}
	if strings.Contains(view, "Recent Events") {
		t.Fatalf("events should render on logs tab only: %q", view)
	}
}

func TestWithEquityHistoryAndChart(t *testing.T) {
	m := New(newTestEngine(), "AAPL").WithEquityHistory([]EquityPoint{
		{Time: time.Now().UTC().Add(-2 * time.Minute), Equity: 100000},
		{Time: time.Now().UTC().Add(-time.Minute), Equity: 100050},
		{Time: time.Now().UTC(), Equity: 100020},
	}, nil)
	rows := m.buildEquityChartRows()
	if len(rows) < 2 {
		t.Fatalf("expected chart rows, got %#v", rows)
	}
	if !strings.Contains(rows[0], "Equity trend") {
		t.Fatalf("expected chart header row, got %q", rows[0])
	}
}

func TestNormalizeSymbols(t *testing.T) {
	got := symbols.Normalize([]string{"aapl", " AAPL ", "msft", ""})
	if len(got) != 2 || got[0] != "AAPL" || got[1] != "MSFT" {
		t.Fatalf("unexpected normalize result: %#v", got)
	}
}

func TestWatchCommands(t *testing.T) {
	m := New(newTestEngine(), "AAPL")

	m.input = "watch list"
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if cmd != nil {
		t.Fatalf("watch list should not schedule refresh")
	}
	if !strings.Contains(m1.status, "watchlist: AAPL") {
		t.Fatalf("unexpected status: %q", m1.status)
	}

	m1.input = "watch add msft"
	model, cmd = m1.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := model.(Model)
	if cmd == nil {
		t.Fatalf("watch add should trigger refresh")
	}
	if len(m2.watchlist) != 2 || m2.watchlist[1] != "MSFT" {
		t.Fatalf("unexpected watchlist after add: %#v", m2.watchlist)
	}

	m2.input = "watch remove aapl"
	model, cmd = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := model.(Model)
	if cmd != nil {
		t.Fatalf("watch remove should not require refresh")
	}
	if len(m3.watchlist) != 1 || m3.watchlist[0] != "MSFT" {
		t.Fatalf("unexpected watchlist after remove: %#v", m3.watchlist)
	}

	m3.input = "buy AAPL 1"
	model, cmd = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("buy command should schedule execution")
	}
	msg := cmd()
	status, ok := msg.(statusOnlyMsg)
	if !ok || !status.isErr || !strings.Contains(status.status, "allowlisted") {
		t.Fatalf("expected allowlist rejection after remove, got %#v", msg)
	}
	m3 = model.(Model)

	m3.input = "watch remove aapl"
	model, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4 := model.(Model)
	if !m4.statusError {
		t.Fatalf("expected error when removing unknown symbol")
	}
}

func TestWatchCommandCallbackError(t *testing.T) {
	m := New(newTestEngine(), "AAPL").WithWatchlistChangeHandler(func([]string) error {
		return context.DeadlineExceeded
	})
	m.input = "watch add MSFT"
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if cmd == nil {
		t.Fatalf("expected async command for watch add with callback")
	}
	model, _ = m1.Update(cmd())
	m1 = model.(Model)
	if !m1.statusError || !strings.Contains(strings.ToLower(m1.status), "sync failed") {
		t.Fatalf("expected sync failure status, got %q", m1.status)
	}
}

func TestWatchSyncCommand(t *testing.T) {
	m := New(newTestEngine(), "AAPL").WithWatchlistSyncHandler(func(current []string) ([]string, error) {
		if len(current) != 1 || current[0] != "AAPL" {
			t.Fatalf("unexpected current watchlist: %#v", current)
		}
		return []string{"AAPL", "BYND"}, nil
	})

	m.input = "watch sync"
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if cmd == nil {
		t.Fatalf("watch sync should schedule async work")
	}
	model, follow := m1.Update(cmd())
	m2 := model.(Model)
	if follow == nil {
		t.Fatalf("watch sync completion should trigger refresh")
	}
	if len(m2.watchlist) != 2 || m2.watchlist[1] != "BYND" {
		t.Fatalf("unexpected watchlist after sync: %#v", m2.watchlist)
	}
	if m2.statusError || !strings.Contains(m2.status, "watchlist synced") {
		t.Fatalf("unexpected sync status: %q", m2.status)
	}
}

func TestWatchSyncCommandError(t *testing.T) {
	m := New(newTestEngine(), "AAPL").WithWatchlistSyncHandler(func([]string) ([]string, error) {
		return nil, context.DeadlineExceeded
	})

	m.input = "watch sync"
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if cmd == nil {
		t.Fatalf("watch sync should schedule async work")
	}
	model, follow := m1.Update(cmd())
	m2 := model.(Model)
	if follow != nil {
		t.Fatalf("watch sync should not refresh on error")
	}
	if !m2.statusError || !strings.Contains(strings.ToLower(m2.status), "sync failed") {
		t.Fatalf("unexpected sync error status: %q", m2.status)
	}
}

func TestEventsCommandScroll(t *testing.T) {
	m := New(newTestEngine())
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		m.snapshot.Events = append(m.snapshot.Events, domain.Event{
			Time:    now.Add(time.Duration(i) * time.Second),
			Type:    "evt",
			Details: strconv.Itoa(i),
		})
	}

	m.input = "events top"
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if m1.eventScroll == 0 {
		t.Fatalf("expected non-zero scroll at top")
	}
	if !strings.Contains(m1.status, "showing") {
		t.Fatalf("unexpected events status: %q", m1.status)
	}

	m1.input = "events down 3"
	model, _ = m1.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := model.(Model)
	if m2.eventScroll != m1.eventScroll-3 {
		t.Fatalf("expected scroll to move down by 3; got %d want %d", m2.eventScroll, m1.eventScroll-3)
	}

	m2.input = "events tail"
	model, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := model.(Model)
	if m3.eventScroll != 0 {
		t.Fatalf("expected tail to reset scroll, got %d", m3.eventScroll)
	}
}

func TestEventsKeyScroll(t *testing.T) {
	m := New(newTestEngine())
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		m.snapshot.Events = append(m.snapshot.Events, domain.Event{
			Time:    now.Add(time.Duration(i) * time.Second),
			Type:    "evt",
			Details: strconv.Itoa(i),
		})
	}

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m1 := model.(Model)
	if m1.eventScroll == 0 {
		t.Fatalf("expected page up to increase event scroll")
	}

	model, _ = m1.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m2 := model.(Model)
	if m2.eventScroll != 0 {
		t.Fatalf("expected end to return to latest events")
	}
}

func TestStrategyKeyScroll(t *testing.T) {
	m := New(newTestEngine())
	m.width = 120
	m.height = 22
	m.activeTab = tabStrategy
	recs := make([]StrategyRecommendationView, 0, 24)
	for i := 0; i < 24; i++ {
		recs = append(recs, StrategyRecommendationView{
			Priority:     i + 1,
			Symbol:       "SYM" + strconv.Itoa(i),
			Bias:         "buy",
			Confidence:   0.61,
			MaxNotional:  1000,
			Thesis:       "thesis line",
			Invalidation: "invalid line",
		})
	}
	m.strategy = StrategySnapshot{
		Active: &StrategyPlanView{
			ID:              1,
			GeneratedAt:     time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
			Status:          "active",
			AnalystModel:    "gpt-5",
			PromptVersion:   "v1",
			Confidence:      0.7,
			Recommendations: recs,
		},
	}
	m.syncWidgets()
	if m.strategyViewport.TotalLineCount() <= m.strategyViewport.VisibleLineCount() {
		t.Fatalf("expected overflow for strategy recommendations viewport")
	}

	before := m.strategyViewport.YOffset
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m1 := model.(Model)
	if m1.strategyViewport.YOffset <= before {
		t.Fatalf("expected strategy down key to scroll viewport")
	}
	if !strings.Contains(m1.status, "strategy: showing") {
		t.Fatalf("expected strategy status update, got %q", m1.status)
	}

	model, _ = m1.Update(tea.KeyMsg{Type: tea.KeyHome})
	m2 := model.(Model)
	if m2.strategyViewport.YOffset != 0 {
		t.Fatalf("expected strategy home key to jump to top")
	}

	model, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m3 := model.(Model)
	if m3.strategyViewport.YOffset <= 0 {
		t.Fatalf("expected strategy end key to jump to bottom")
	}
}

func TestStrategyChatKeyScroll(t *testing.T) {
	m := New(newTestEngine())
	m.width = 120
	m.height = 22
	m.activeTab = tabChat
	msgs := make([]StrategyChatMessageView, 0, 24)
	for i := 0; i < 24; i++ {
		msgs = append(msgs, StrategyChatMessageView{
			ThreadID:  1,
			Role:      "assistant",
			Content:   "message line " + strconv.Itoa(i) + " with extra content to wrap",
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Minute),
		})
	}
	m.strategy = StrategySnapshot{
		Chat: StrategyChatView{
			ActiveThreadID: 1,
			Threads: []StrategyChatThreadView{
				{ID: 1, Title: "Main"},
			},
			Messages: msgs,
		},
	}
	m.strategyThreadID = 1
	m.syncWidgets()
	if m.strategyChatViewport.TotalLineCount() <= m.strategyChatViewport.VisibleLineCount() {
		t.Fatalf("expected overflow for strategy chat viewport")
	}

	before := m.strategyChatViewport.YOffset
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m1 := model.(Model)
	if m1.strategyChatViewport.YOffset <= before {
		t.Fatalf("expected chat down key to scroll viewport")
	}
	if !strings.Contains(m1.status, "chat: showing") {
		t.Fatalf("expected chat status update, got %q", m1.status)
	}

	model, _ = m1.Update(tea.KeyMsg{Type: tea.KeyHome})
	m2 := model.(Model)
	if m2.strategyChatViewport.YOffset != 0 {
		t.Fatalf("expected chat home key to jump to top")
	}

	model, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m3 := model.(Model)
	if m3.strategyChatViewport.YOffset <= 0 {
		t.Fatalf("expected chat end key to jump to bottom")
	}
}

func TestHelpToggleKey(t *testing.T) {
	m := New(newTestEngine())
	m.width = 140
	m.height = 40
	m.syncWidgets()

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m1 := model.(Model)
	if !m1.showFullHelp {
		t.Fatalf("expected full help to be enabled")
	}
	view := m1.View()
	if !strings.Contains(view, "toggle help") {
		t.Fatalf("expected expanded help content in footer, got %q", view)
	}

	model, _ = m1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m2 := model.(Model)
	if m2.showFullHelp {
		t.Fatalf("expected full help to be disabled")
	}
}

func TestTabSwitching(t *testing.T) {
	m := New(newTestEngine())
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		m.snapshot.Events = append(m.snapshot.Events, domain.Event{
			Time:    now.Add(time.Duration(i) * time.Second),
			Type:    "evt",
			Details: strconv.Itoa(i),
		})
	}
	m.syncWidgets()

	overviewView := m.View()
	if strings.Contains(overviewView, "Recent Events") {
		t.Fatalf("events should not show on overview tab: %q", overviewView)
	}

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m1 := model.(Model)
	if m1.activeTab != tabStrategy {
		t.Fatalf("expected active tab strategy, got %s", m1.activeTab)
	}

	model, _ = m1.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := model.(Model)
	if m2.activeTab != tabChat {
		t.Fatalf("expected active tab chat, got %s", m2.activeTab)
	}

	model, _ = m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := model.(Model)
	if m3.activeTab != tabSystem {
		t.Fatalf("expected active tab system, got %s", m3.activeTab)
	}

	model, _ = m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m4 := model.(Model)
	if m4.activeTab != tabLogs {
		t.Fatalf("expected active tab logs, got %s", m4.activeTab)
	}
	logsView := m4.View()
	if !strings.Contains(logsView, "Recent Events") {
		t.Fatalf("expected events on logs tab: %q", logsView)
	}

	model, _ = m4.Update(tea.KeyMsg{Type: tea.KeyTab})
	m5 := model.(Model)
	if m5.activeTab != tabOverview {
		t.Fatalf("expected active tab overview, got %s", m5.activeTab)
	}

	m5.input = "tab strategy"
	model, _ = m5.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m6 := model.(Model)
	strategyView := m6.View()
	if !strings.Contains(strategyView, "Strategy Plan") || !strings.Contains(strategyView, "Recent Plans") || !strings.Contains(strategyView, "Health") {
		t.Fatalf("expected strategy panel on strategy tab: %q", strategyView)
	}
	if strings.Contains(strategyView, "Copilot Chat") {
		t.Fatalf("strategy tab should not render chat panel: %q", strategyView)
	}

	m6.input = "tab chat"
	model, _ = m6.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m7 := model.(Model)
	chatView := m7.View()
	if !strings.Contains(chatView, "Copilot Chat") {
		t.Fatalf("expected chat panel on chat tab: %q", chatView)
	}

	m7.input = "tab system"
	model, _ = m7.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m8 := model.(Model)
	systemView := m8.View()
	if !strings.Contains(systemView, "Runtime") || !strings.Contains(systemView, "watchlist") {
		t.Fatalf("expected system panel on system tab: %q", systemView)
	}
	if !strings.Contains(systemView, "requests") {
		t.Fatalf("expected request counters in system panel: %q", systemView)
	}
	if !strings.Contains(systemView, "strategy") {
		t.Fatalf("expected strategy summary in system panel: %q", systemView)
	}
}

func TestTabCommandTargetsStrategy(t *testing.T) {
	m := New(newTestEngine())
	m.input = "tab strategy"
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if m1.activeTab != tabStrategy {
		t.Fatalf("expected active tab strategy, got %s", m1.activeTab)
	}
	view := m1.View()
	if !strings.Contains(view, "Strategy Plan") {
		t.Fatalf("expected strategy tab content, got %q", view)
	}
	if strings.Contains(view, "Copilot Chat") {
		t.Fatalf("strategy tab should not include chat panel, got %q", view)
	}
}

func TestTabCommandTargetsChat(t *testing.T) {
	m := New(newTestEngine())
	m.input = "tab chat"
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if m1.activeTab != tabChat {
		t.Fatalf("expected active tab chat, got %s", m1.activeTab)
	}
	view := m1.View()
	if !strings.Contains(view, "Copilot Chat") {
		t.Fatalf("expected chat tab content, got %q", view)
	}
}

func TestStrategyRunCommandInvokesHandler(t *testing.T) {
	triggered := false
	m := New(newTestEngine()).WithStrategyRunHandler(func() error {
		triggered = true
		return nil
	})
	m.input = "strategy run"
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if !triggered {
		t.Fatalf("expected strategy run handler to be invoked")
	}
	if cmd == nil {
		t.Fatalf("expected refresh command after strategy run request")
	}
	if m1.statusError || !strings.Contains(m1.status, "strategy run") {
		t.Fatalf("unexpected status: %q", m1.status)
	}
}

func TestStrategyRunLoadingCompletesOnPlanCreatedEvent(t *testing.T) {
	m := New(newTestEngine()).WithStrategyRunHandler(func() error { return nil })
	m.input = "strategy run"
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if !m1.strategyBusy {
		t.Fatalf("expected strategy busy state after run request")
	}
	m1.snapshot.Events = append(m1.snapshot.Events, domain.Event{
		Time:    time.Now().UTC(),
		Type:    "strategy_plan_created",
		Details: "id=42",
	})
	model, _ = m1.Update(refreshMsg{snapshot: m1.snapshot})
	m2 := model.(Model)
	if m2.strategyBusy {
		t.Fatalf("expected strategy busy state to clear after plan created")
	}
	if m2.statusError || !strings.Contains(m2.status, "plan created") {
		t.Fatalf("unexpected status after completion: %q", m2.status)
	}
}

func TestStrategyRunLoadingCompletesOnPlanUnchangedEvent(t *testing.T) {
	m := New(newTestEngine()).WithStrategyRunHandler(func() error { return nil })
	m.input = "strategy run"
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if !m1.strategyBusy {
		t.Fatalf("expected strategy busy state after run request")
	}
	m1.snapshot.Events = append(m1.snapshot.Events, domain.Event{
		Time:    time.Now().UTC(),
		Type:    "strategy_plan_unchanged",
		Details: "id=7",
	})
	model, _ = m1.Update(refreshMsg{snapshot: m1.snapshot})
	m2 := model.(Model)
	if m2.strategyBusy {
		t.Fatalf("expected strategy busy state to clear after unchanged event")
	}
	if m2.statusError || !strings.Contains(m2.status, "no changes") {
		t.Fatalf("unexpected status after unchanged completion: %q", m2.status)
	}
}

func TestStrategyStatusControlsInvokeHandlers(t *testing.T) {
	var approved, rejected, archived uint
	m := New(newTestEngine()).
		WithStrategyApproveHandler(func(id uint) error {
			approved = id
			return nil
		}).
		WithStrategyRejectHandler(func(id uint) error {
			rejected = id
			return nil
		}).
		WithStrategyArchiveHandler(func(id uint) error {
			archived = id
			return nil
		})

	m.input = "strategy approve 11"
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if approved != 11 {
		t.Fatalf("expected approve handler with id=11, got %d", approved)
	}
	if cmd == nil {
		t.Fatalf("expected refresh command for approve")
	}
	if m1.statusError || !strings.Contains(m1.status, "strategy approve #11") {
		t.Fatalf("unexpected approve status: %q", m1.status)
	}

	m1.input = "strategy reject 12"
	model, cmd = m1.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := model.(Model)
	if rejected != 12 {
		t.Fatalf("expected reject handler with id=12, got %d", rejected)
	}
	if cmd == nil {
		t.Fatalf("expected refresh command for reject")
	}
	if m2.statusError || !strings.Contains(m2.status, "strategy reject #12") {
		t.Fatalf("unexpected reject status: %q", m2.status)
	}

	m2.input = "strategy archive 13"
	model, cmd = m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := model.(Model)
	if archived != 13 {
		t.Fatalf("expected archive handler with id=13, got %d", archived)
	}
	if cmd == nil {
		t.Fatalf("expected refresh command for archive")
	}
	if m3.statusError || !strings.Contains(m3.status, "strategy archive #13") {
		t.Fatalf("unexpected archive status: %q", m3.status)
	}
}

func TestStrategyChatCommands(t *testing.T) {
	var createdTitle string
	var sentThreadID uint
	var sentMessage string

	m := New(newTestEngine()).
		WithStrategyChatCreateHandler(func(title string) (uint, error) {
			createdTitle = title
			return 9, nil
		}).
		WithStrategyChatSendHandler(func(threadID uint, message string) error {
			sentThreadID = threadID
			sentMessage = message
			return nil
		})
	m.strategy = StrategySnapshot{
		Chat: StrategyChatView{
			ActiveThreadID: 2,
			Threads: []StrategyChatThreadView{
				{ID: 2, Title: "Main"},
				{ID: 3, Title: "Swing"},
			},
			Messages: []StrategyChatMessageView{
				{ID: 1, ThreadID: 2, Role: "user", Content: "hello"},
			},
		},
	}
	m.strategyThreadID = 2

	m.input = "strategy chat status"
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 := model.(Model)
	if m1.statusError || !strings.Contains(m1.status, "thread #2") {
		t.Fatalf("unexpected strategy chat status output: %q", m1.status)
	}

	m1.input = "strategy chat list"
	model, _ = m1.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := model.(Model)
	if m2.statusError || !strings.Contains(m2.status, "#2 Main") {
		t.Fatalf("unexpected strategy chat list output: %q", m2.status)
	}

	m2.input = "strategy chat use 3"
	model, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := model.(Model)
	if cmd == nil {
		t.Fatalf("expected refresh command when selecting strategy chat thread")
	}
	if m3.strategyThreadID != 3 {
		t.Fatalf("expected selected thread id 3, got %d", m3.strategyThreadID)
	}

	m3.input = "strategy chat say rotate into energy"
	model, cmd = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4 := model.(Model)
	if cmd == nil {
		t.Fatalf("expected async command for strategy chat say")
	}
	model, _ = m4.Update(cmd())
	m5 := model.(Model)
	if m5.statusError {
		t.Fatalf("unexpected strategy chat say error: %q", m5.status)
	}
	if sentThreadID != 3 || sentMessage != "rotate into energy" {
		t.Fatalf("unexpected strategy chat send inputs: thread=%d message=%q", sentThreadID, sentMessage)
	}

	m5.input = "strategy chat new Swing Plan"
	model, cmd = m5.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m6 := model.(Model)
	if cmd == nil {
		t.Fatalf("expected async command for strategy chat new")
	}
	model, _ = m6.Update(cmd())
	m7 := model.(Model)
	if createdTitle != "Swing Plan" {
		t.Fatalf("unexpected created title: %q", createdTitle)
	}
	if m7.strategyThreadID != 9 {
		t.Fatalf("expected selected thread id 9 after create, got %d", m7.strategyThreadID)
	}
}

func TestRenderTwoColumnPanelsWidthAlignment(t *testing.T) {
	total := 120
	row := renderTwoColumnPanels([]string{"L"}, []string{"R"}, 59, 60, total, 1)
	if got := lipgloss.Width(row); got != total {
		t.Fatalf("expected two-column row width %d, got %d", total, got)
	}
}

func TestWatchlistStateUsesSeenTime(t *testing.T) {
	m := New(newTestEngine(), "AAPL")
	m.quotes["AAPL"] = domain.Quote{
		Symbol: "AAPL",
		Last:   100,
		Bid:    99.5,
		Ask:    100.5,
		Time:   time.Now().UTC().Add(-10 * time.Minute),
	}
	m.quoteSeenAt["AAPL"] = time.Now().UTC()
	rows := m.watchlistTableRows()
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if got := rows[0][6]; !strings.Contains(got, "ok") {
		t.Fatalf("expected ok state for recently seen quote, got %q", got)
	}

	m.quoteSeenAt["AAPL"] = time.Now().UTC().Add(-(watchlistStateStaleAfter + time.Second))
	rows = m.watchlistTableRows()
	if got := rows[0][6]; !strings.Contains(got, "stale") {
		t.Fatalf("expected stale state for old seen quote, got %q", got)
	}
}

func TestWatchlistChangeCellIncludesPct(t *testing.T) {
	m := New(newTestEngine(), "AAPL")
	m.prevLast["AAPL"] = 100
	m.quotes["AAPL"] = domain.Quote{
		Symbol: "AAPL",
		Last:   101,
		Bid:    100.5,
		Ask:    101.5,
		Time:   time.Now().UTC(),
	}
	m.quoteSeenAt["AAPL"] = time.Now().UTC()
	rows := m.watchlistTableRows()
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if got := rows[0][5]; !strings.Contains(got, "+1.00%") {
		t.Fatalf("expected pct change cell, got %q", got)
	}
}

func TestEventPageSizeExpandsOnLogsTab(t *testing.T) {
	m := New(newTestEngine())
	m.height = 40
	m.activeTab = tabOverview
	if got := m.eventPageSize(); got != 8 {
		t.Fatalf("expected overview page size 8, got %d", got)
	}
	m.activeTab = tabLogs
	if got := m.eventPageSize(); got <= 8 {
		t.Fatalf("expected expanded logs page size, got %d", got)
	}
}

func TestLogsWrapLongEventLines(t *testing.T) {
	m := New(newTestEngine())
	m.width = 120
	m.height = 32
	m.snapshot.Events = []domain.Event{
		{
			Time:    time.Now().UTC(),
			Type:    "agent_cycle_error",
			Details: strings.Repeat("this is a very long detail segment ", 10),
		},
	}
	m.syncWidgets()
	if got := m.eventsViewport.TotalLineCount(); got <= 1 {
		t.Fatalf("expected wrapped event content to span multiple lines, got %d", got)
	}
	content := strings.Split(strings.TrimSpace(m.eventsViewport.View()), "\n")
	for _, line := range content {
		if lipgloss.Width(line) > m.eventsViewport.Width {
			t.Fatalf("line exceeds viewport width: width=%d max=%d line=%q", lipgloss.Width(line), m.eventsViewport.Width, line)
		}
	}
}

func TestLogsFilterPersistStatsNoise(t *testing.T) {
	m := New(newTestEngine())
	m.width = 120
	m.height = 32
	now := time.Now().UTC()
	m.snapshot.Events = []domain.Event{
		{Time: now, Type: "event_persist_stats", Details: "queue=0"},
		{Time: now.Add(time.Second), Type: "sync", Details: "ok"},
	}
	m.syncWidgets()
	view := m.eventsViewport.View()
	if strings.Contains(view, "event_persist_stats") {
		t.Fatalf("expected event_persist_stats to be filtered from logs viewport, got %q", view)
	}
	if !strings.Contains(view, "sync") {
		t.Fatalf("expected non-filtered events to remain visible, got %q", view)
	}
}

func TestEventHintStaysSingleLineAtNarrowWidth(t *testing.T) {
	m := New(newTestEngine())
	m.width = 74
	m.height = 24
	m.activeTab = tabLogs
	now := time.Now().UTC()
	for i := 0; i < 23; i++ {
		m.snapshot.Events = append(m.snapshot.Events, domain.Event{
			Time:    now.Add(time.Duration(i) * time.Second),
			Type:    "sync",
			Details: "ok",
		})
	}
	m.syncWidgets()
	rows := m.buildEventRows()
	if len(rows) == 0 {
		t.Fatalf("expected event rows")
	}
	last := stripANSI(rows[len(rows)-1])
	maxWidth := panelInnerWidth(m.eventsPanelWidth())
	if got := lipgloss.Width(last); got > maxWidth {
		t.Fatalf("expected hint width <= %d, got %d: %q", maxWidth, got, last)
	}
}

func TestLogsViewHeightDoesNotOverflowWindow(t *testing.T) {
	m := New(newTestEngine())
	m.width = 128
	m.height = 26
	m.activeTab = tabLogs
	m.snapshot.Events = []domain.Event{
		{
			Time:    time.Now().UTC(),
			Type:    "compliance_posture",
			Details: "enabled=true account_type=cash avoid_pdt=true avoid_gfv=false pdt=true day_trades=10 max_day_trades_5d=3 equity=99852.17 min_equity_for_pdt=25000.00 local_unsettled=0.00 broker_unsettled=0.00 drift_detected=false",
		},
		{
			Time:    time.Now().UTC().Add(time.Second),
			Type:    "agent_cycle_complete",
			Details: "generated=0 attempted=0 executed=0 rejected=0 approvals=0 dry_run=0 skipped=0 reason=low_power_state",
		},
	}
	m.syncWidgets()
	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("expected logs view height <= window height (%d), got %d", m.height, got)
	}
}

func TestOrderTableColumnsLeaveGapAfterOrderID(t *testing.T) {
	cols := orderTableColumns(40)
	if len(cols) < 2 {
		t.Fatalf("unexpected columns: %#v", cols)
	}
	if cols[1].Width < 8 {
		t.Fatalf("expected Order ID column width >= 8 for spacing, got %d", cols[1].Width)
	}
}

func TestOrderTableIncludesLimitColumn(t *testing.T) {
	cols := orderTableColumns(48)
	if len(cols) != 7 {
		t.Fatalf("expected 7 order columns, got %d", len(cols))
	}
	if cols[5].Title != "Limit" {
		t.Fatalf("expected Limit column at index 5, got %q", cols[5].Title)
	}
	sum := 0
	for _, c := range cols {
		sum += c.Width
	}
	rendered := sum + (len(cols) - 1)
	if rendered > 48 {
		t.Fatalf("expected order columns to fit width, rendered=%d total=48", rendered)
	}
}

func TestOrderTableRowsIncludeLimitValue(t *testing.T) {
	limit := 12.34
	rows := orderTableRows([]domain.Order{
		{ID: "ord-1", Side: domain.SideBuy, Symbol: "AAPL", Qty: 5, Status: domain.OrderStatusNew, LimitPrice: &limit},
		{ID: "ord-2", Side: domain.SideSell, Symbol: "MSFT", Qty: 3, Status: domain.OrderStatusAccepted},
	})
	if got := rows[0][5]; got != "12.34" {
		t.Fatalf("expected limit value 12.34, got %q", got)
	}
	if got := rows[1][5]; got != "-" {
		t.Fatalf("expected '-' for market/non-limit order, got %q", got)
	}
}

func TestPositionTableIncludesUPNLColumn(t *testing.T) {
	cols := positionTableColumns(56)
	if len(cols) != 5 {
		t.Fatalf("expected 5 position columns, got %d", len(cols))
	}
	if cols[4].Title != "uPnL" {
		t.Fatalf("expected uPnL as last column, got %q", cols[4].Title)
	}
}

func TestViewShowsMinSizePanelWhenTooSmall(t *testing.T) {
	m := New(newTestEngine())
	m.width = minUIWidth - 1
	m.height = minUIHeight - 1
	view := m.View()
	if !strings.Contains(view, "Terminal Size") {
		t.Fatalf("expected min-size warning panel, got %q", view)
	}
	if !strings.Contains(view, "Minimum recommended") {
		t.Fatalf("expected minimum size text, got %q", view)
	}
}

func TestColorizeTableColumnsPreservesLayout(t *testing.T) {
	view := strings.Join([]string{
		"Side  Chg    State   ",
		"BUY   +1.00% ok      ",
	}, "\n")
	cols := []table.Column{
		{Title: "Side", Width: 6},
		{Title: "Chg", Width: 7},
		{Title: "State", Width: 8},
	}
	colored := colorizeTableColumns(view, cols, map[int]func(string) string{
		0: colorizeOrderSideCell,
		1: colorizeWatchChangeCell,
		2: colorizeWatchStateCell,
	})
	if stripANSI(colored) != view {
		t.Fatalf("colorized table should preserve printable layout\nwant:\n%q\ngot:\n%q", view, stripANSI(colored))
	}
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

func newTestEngine() *engine.Engine {
	b := paper.New(10000)
	gate := engine.NewRiskGate(engine.Policy{
		AllowMarketOrders: true,
	})
	e := engine.New(b, gate)
	_ = e.Sync(context.Background())
	return e
}
