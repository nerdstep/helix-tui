package app

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/broker/paper"
	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
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
	if cfg.AgentType != "heuristic" {
		t.Fatalf("unexpected agent type default: %q", cfg.AgentType)
	}
	if cfg.AgentInterval <= 0 || cfg.AgentOrderQty <= 0 || cfg.AgentMovePct <= 0 {
		t.Fatalf("unexpected agent defaults: %#v", cfg)
	}
	if cfg.AgentMinGainPct != 0 {
		t.Fatalf("unexpected default min gain pct: %f", cfg.AgentMinGainPct)
	}
	if cfg.SyncTimeout <= 0 || cfg.OrderTimeout <= 0 {
		t.Fatalf("unexpected runtime timeout defaults: sync=%s order=%s", cfg.SyncTimeout, cfg.OrderTimeout)
	}
	if cfg.LogMode != "append" {
		t.Fatalf("unexpected log mode default: %q", cfg.LogMode)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("unexpected log level default: %q", cfg.LogLevel)
	}
	if cfg.DatabasePath == "" {
		t.Fatalf("expected default database path")
	}
	if cfg.LLMModel == "" || cfg.LLMBaseURL == "" || cfg.LLMTimeout <= 0 {
		t.Fatalf("unexpected llm defaults: model=%q base=%q timeout=%s", cfg.LLMModel, cfg.LLMBaseURL, cfg.LLMTimeout)
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

func TestBuildBrokerPaper(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "paper"

	spec, err := buildBroker(cfg)
	if err != nil {
		t.Fatalf("buildBroker failed: %v", err)
	}
	if spec.label != "paper" {
		t.Fatalf("unexpected label: %q", spec.label)
	}
	if spec.isAlpaca {
		t.Fatalf("expected non-alpaca broker")
	}
	if spec.broker == nil {
		t.Fatalf("expected broker")
	}
	if spec.watchlistSyncBroker != nil {
		t.Fatalf("paper broker should not expose watchlist sync broker")
	}
}

func TestBuildBrokerAlpacaWithDirectCredentials(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "alpaca"
	cfg.UseKeyring = false
	cfg.SaveToKeyring = false
	cfg.AlpacaAPIKey = "key"
	cfg.AlpacaAPISecret = "secret"

	spec, err := buildBroker(cfg)
	if err != nil {
		t.Fatalf("buildBroker failed: %v", err)
	}
	if spec.label != "alpaca" {
		t.Fatalf("unexpected label: %q", spec.label)
	}
	if !spec.isAlpaca {
		t.Fatalf("expected alpaca broker")
	}
	if spec.broker == nil {
		t.Fatalf("expected broker")
	}
	if spec.watchlistSyncBroker == nil {
		t.Fatalf("expected watchlist sync broker for alpaca")
	}
	if spec.credentialSource == "" {
		t.Fatalf("expected credential source")
	}
}

func TestBuildBrokerUnsupported(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "invalid"

	_, err := buildBroker(cfg)
	if err == nil {
		t.Fatalf("expected unsupported broker error")
	}
	if !strings.Contains(err.Error(), "unsupported broker") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWatchlistWithoutSyncBroker(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Watchlist = []string{"aapl", " AAPL ", "msft", ""}

	got, err := resolveWatchlist(cfg, nil)
	if err != nil {
		t.Fatalf("resolveWatchlist failed: %v", err)
	}
	want := []string{"AAPL", "MSFT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("watchlist mismatch: got %#v want %#v", got, want)
	}
}

func TestBuildAllowSymbolsUsesWatchlistOnly(t *testing.T) {
	got := buildAllowSymbols([]string{"aapl", " msft ", "AAPL", "msft", "nvda", " NVDA "})
	want := map[string]struct{}{
		"AAPL": {},
		"MSFT": {},
		"NVDA": {},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("allowlist mismatch: got %#v want %#v", got, want)
	}
}

func TestBuildWatchlistHandlersWithoutBroker(t *testing.T) {
	pull, sync := buildWatchlistHandlers(nil)
	if pull != nil {
		t.Fatalf("expected nil pull handler")
	}
	if sync != nil {
		t.Fatalf("expected nil sync handler")
	}
}

func TestBuildEngineSyncs(t *testing.T) {
	cfg := DefaultConfig()
	e, err := buildEngine(cfg, paper.New(100000), map[string]struct{}{"AAPL": {}})
	if err != nil {
		t.Fatalf("buildEngine failed: %v", err)
	}
	if e == nil {
		t.Fatalf("expected engine")
	}
	if !hasEventType(e.Snapshot().Events, "sync") {
		t.Fatalf("expected sync event")
	}
}

func TestAddAlpacaConfigEvent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Broker = "alpaca"
	cfg.AlpacaEnv = "live"
	cfg.AlpacaBaseURL = "https://example-alpaca.local"
	cfg.AlpacaFeed = "SIP"
	e := newTestEngine(t)

	addAlpacaConfigEvent(e, cfg, "flags")
	events := e.Snapshot().Events
	if !hasEventType(events, "alpaca_config") {
		t.Fatalf("expected alpaca_config event")
	}
	if !containsEventDetail(events, "alpaca_config", "env=live") {
		t.Fatalf("expected env detail")
	}
	if !containsEventDetail(events, "alpaca_config", "endpoint=https://example-alpaca.local") {
		t.Fatalf("expected endpoint detail")
	}
	if !containsEventDetail(events, "alpaca_config", "feed=sip") {
		t.Fatalf("expected feed detail")
	}
	if !containsEventDetail(events, "alpaca_config", "credentials=flags") {
		t.Fatalf("expected credential source detail")
	}
}

func TestBuildRunnerManualReturnsNil(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = domain.ModeManual
	e := newTestEngine(t)

	runner, mode, agentType, err := buildRunner(cfg, paper.New(100000), e, []string{"AAPL"})
	if err != nil {
		t.Fatalf("buildRunner failed: %v", err)
	}
	if runner != nil {
		t.Fatalf("expected nil runner in manual mode")
	}
	if mode != domain.ModeManual {
		t.Fatalf("unexpected mode: %q", mode)
	}
	if agentType != "" {
		t.Fatalf("expected empty agent type in manual mode")
	}
}

func TestBuildRunnerAssistCreatesRunner(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = domain.ModeAssist
	cfg.AgentInterval = time.Second
	e := newTestEngine(t)

	runner, mode, agentType, err := buildRunner(cfg, paper.New(100000), e, []string{"AAPL"})
	if err != nil {
		t.Fatalf("buildRunner failed: %v", err)
	}
	if runner == nil {
		t.Fatalf("expected runner in assist mode")
	}
	if mode != domain.ModeAssist {
		t.Fatalf("unexpected mode: %q", mode)
	}
	if agentType != "heuristic" {
		t.Fatalf("unexpected agent type: %q", agentType)
	}
}

func TestBuildRunnerLLMRequiresAPIKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = domain.ModeAssist
	cfg.AgentType = "llm"
	cfg.LLMAPIKey = ""
	cfg.UseKeyring = false
	e := newTestEngine(t)

	_, _, _, err := buildRunner(cfg, paper.New(100000), e, []string{"AAPL"})
	if err == nil {
		t.Fatalf("expected llm api key error")
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

func newTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	risk := engine.NewRiskGate(engine.Policy{
		MaxNotionalPerTrade: 5000,
		MaxNotionalPerDay:   20000,
		AllowMarketOrders:   true,
		AllowSymbols: map[string]struct{}{
			"AAPL": {},
		},
	})
	return engine.New(paper.New(100000), risk)
}
