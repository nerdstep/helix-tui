package app

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/domain"
	"helix-tui/internal/symbols"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Broker != "paper" {
		t.Fatalf("unexpected broker default: %q", cfg.Broker)
	}
	if cfg.AlpacaFeed != "iex" {
		t.Fatalf("unexpected feed default: %q", cfg.AlpacaFeed)
	}
	if cfg.AlpacaEnv != "paper" {
		t.Fatalf("unexpected alpaca env default: %q", cfg.AlpacaEnv)
	}
	if !cfg.UseKeyring || !cfg.SaveToKeyring {
		t.Fatalf("expected keyring defaults enabled")
	}
	if cfg.Mode != domain.ModeManual {
		t.Fatalf("unexpected mode default: %q", cfg.Mode)
	}
	if cfg.AgentInterval <= 0 || cfg.AgentOrderQty <= 0 || cfg.AgentMovePct <= 0 {
		t.Fatalf("unexpected agent defaults: %#v", cfg)
	}
}

func TestNewSystem_PaperManual(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "paper"
	cfg.Mode = domain.ModeManual

	sys, err := NewSystem(cfg)
	if err != nil {
		t.Fatalf("NewSystem failed: %v", err)
	}
	if sys.Engine == nil {
		t.Fatalf("expected engine")
	}
	if sys.Runner != nil {
		t.Fatalf("runner should be nil in manual mode")
	}
	if !hasEventType(sys.Engine.Snapshot().Events, "sync") {
		t.Fatalf("expected sync event")
	}
}

func TestNewSystem_PaperAssistCreatesRunner(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "paper"
	cfg.Mode = domain.ModeAssist
	cfg.Watchlist = []string{"aapl", " AAPL ", "msft"}
	cfg.AgentInterval = time.Second

	sys, err := NewSystem(cfg)
	if err != nil {
		t.Fatalf("NewSystem failed: %v", err)
	}
	if sys.Runner == nil {
		t.Fatalf("expected runner in assist mode")
	}
	events := sys.Engine.Snapshot().Events
	if !hasEventType(events, "agent_mode") {
		t.Fatalf("expected agent_mode event")
	}
	if !containsEventDetail(events, "agent_mode", "watchlist=AAPL,MSFT") {
		t.Fatalf("expected normalized watchlist in agent_mode event")
	}
}

func TestNewSystem_UnsupportedBroker(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "not-real"
	_, err := NewSystem(cfg)
	if err == nil {
		t.Fatalf("expected unsupported broker error")
	}
	if !strings.Contains(err.Error(), "unsupported broker") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewSystem_AlpacaMissingCredentials(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "alpaca"
	cfg.UseKeyring = false
	cfg.AlpacaAPIKey = ""
	cfg.AlpacaAPISecret = ""

	_, err := NewSystem(cfg)
	if err == nil {
		t.Fatalf("expected credential error")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEffectiveAlpacaEnv(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlpacaEnv = "live"
	if got := effectiveAlpacaEnv(cfg); got != "live" {
		t.Fatalf("expected alpaca broker env to honor config, got %q", got)
	}
}

func TestNewEngine(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "paper"
	e, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	if e == nil {
		t.Fatalf("expected engine")
	}
}

func TestNormalizeMode(t *testing.T) {
	tests := []struct {
		in   domain.Mode
		want domain.Mode
	}{
		{in: "manual", want: domain.ModeManual},
		{in: "ASSIST", want: domain.ModeAssist},
		{in: " auto ", want: domain.ModeAuto},
		{in: "unknown", want: domain.ModeManual},
	}
	for _, tt := range tests {
		if got := normalizeMode(tt.in); got != tt.want {
			t.Fatalf("normalizeMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeSymbols(t *testing.T) {
	got := symbols.Normalize([]string{"aapl", " AAPL ", "msft", "", "MSFT"})
	want := []string{"AAPL", "MSFT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize mismatch: got %#v want %#v", got, want)
	}
	if symbols.Normalize(nil) != nil {
		t.Fatalf("expected nil result for nil input")
	}
}

func TestNewSystem_AutoRunnerCanRunOneCycle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "paper"
	cfg.Mode = domain.ModeAuto
	cfg.AgentInterval = time.Millisecond
	cfg.Watchlist = []string{"AAPL"}
	cfg.MaxAgentIntents = 1
	cfg.AgentDryRun = true

	sys, err := NewSystem(cfg)
	if err != nil {
		t.Fatalf("NewSystem failed: %v", err)
	}
	if sys.Runner == nil {
		t.Fatalf("expected runner in auto mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_ = sys.Runner.Run(ctx)

	if !hasEventType(sys.Engine.Snapshot().Events, "agent_cycle_complete") {
		t.Fatalf("expected agent cycle completion event")
	}
}

func TestNewSystem_WatchlistSymbolsAreAllowlisted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "paper"
	cfg.Mode = domain.ModeManual
	cfg.AllowSymbols = []string{"AAPL"}
	cfg.Watchlist = []string{"MSFT"}

	sys, err := NewSystem(cfg)
	if err != nil {
		t.Fatalf("NewSystem failed: %v", err)
	}
	_, err = sys.Engine.PlaceOrder(context.Background(), domain.OrderRequest{
		Symbol: "MSFT",
		Side:   domain.SideBuy,
		Qty:    1,
		Type:   domain.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("expected watchlist symbol to be allowlisted, got %v", err)
	}
}

func TestNewSystem_PaperHasNoRemoteWatchlistHandlers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "paper"
	cfg.Mode = domain.ModeManual

	sys, err := NewSystem(cfg)
	if err != nil {
		t.Fatalf("NewSystem failed: %v", err)
	}
	if sys.PullWatchlist != nil {
		t.Fatalf("expected no pull watchlist handler for paper broker")
	}
	if sys.SyncWatchlist != nil {
		t.Fatalf("expected no sync watchlist handler for paper broker")
	}
}

func hasEventType(events []domain.Event, want string) bool {
	for _, e := range events {
		if e.Type == want {
			return true
		}
	}
	return false
}

func containsEventDetail(events []domain.Event, eventType, needle string) bool {
	for _, e := range events {
		if e.Type == eventType && strings.Contains(e.Details, needle) {
			return true
		}
	}
	return false
}
