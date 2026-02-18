package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/broker/paper"
	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
)

func TestNewAndInit(t *testing.T) {
	m := New(newTestEngine())
	if m.status == "" {
		t.Fatalf("expected initial status")
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
		{name: "help", raw: "help", wantSub: "buy/sell/cancel/flatten/sync"},
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
	m := New(e)
	msg := m.refreshCmd()()
	r, ok := msg.(refreshMsg)
	if !ok {
		t.Fatalf("expected refreshMsg, got %#v", msg)
	}
	if r.err != nil {
		t.Fatalf("unexpected refresh error: %v", r.err)
	}

	m.snapshot = r.snapshot
	view := m.View()
	if !strings.Contains(view, "helix-tui") || !strings.Contains(view, "Commands:") {
		t.Fatalf("unexpected view output: %q", view)
	}
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
