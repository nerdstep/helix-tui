package configfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/domain"
)

func TestDefaultAndToAppConfig(t *testing.T) {
	cfg := Default()
	appCfg := cfg.ToAppConfig()
	if appCfg.Broker != cfg.Broker {
		t.Fatalf("broker mismatch: app=%q cfg=%q", appCfg.Broker, cfg.Broker)
	}
	if appCfg.Mode != domain.Mode(cfg.Mode) {
		t.Fatalf("mode mismatch: app=%q cfg=%q", appCfg.Mode, cfg.Mode)
	}
	if appCfg.AgentInterval != cfg.Agent.Interval {
		t.Fatalf("agent interval mismatch: app=%s cfg=%s", appCfg.AgentInterval, cfg.Agent.Interval)
	}
	if appCfg.LLMTimeout != cfg.Agent.LLM.Timeout {
		t.Fatalf("llm timeout mismatch: app=%s cfg=%s", appCfg.LLMTimeout, cfg.Agent.LLM.Timeout)
	}
	if appCfg.LLMContextLog != cfg.Agent.LLM.ContextLog {
		t.Fatalf("llm context log mismatch: app=%q cfg=%q", appCfg.LLMContextLog, cfg.Agent.LLM.ContextLog)
	}
	if appCfg.AgentLowPowerEnabled != cfg.Agent.LowPower.Enabled {
		t.Fatalf("agent low power enabled mismatch: app=%t cfg=%t", appCfg.AgentLowPowerEnabled, cfg.Agent.LowPower.Enabled)
	}
	if appCfg.AgentClosedPollInterval != cfg.Agent.LowPower.ClosedPollInterval {
		t.Fatalf("agent low power closed poll interval mismatch: app=%s cfg=%s", appCfg.AgentClosedPollInterval, cfg.Agent.LowPower.ClosedPollInterval)
	}
	if appCfg.AgentPreOpenWarmup != cfg.Agent.LowPower.PreOpenWarmup {
		t.Fatalf("agent low power pre-open warmup mismatch: app=%s cfg=%s", appCfg.AgentPreOpenWarmup, cfg.Agent.LowPower.PreOpenWarmup)
	}
	if appCfg.ComplianceEnabled != cfg.Compliance.Enabled {
		t.Fatalf("compliance enabled mismatch: app=%t cfg=%t", appCfg.ComplianceEnabled, cfg.Compliance.Enabled)
	}
	if appCfg.ComplianceSettlementDays != cfg.Compliance.SettlementDays {
		t.Fatalf("compliance settlement days mismatch: app=%d cfg=%d", appCfg.ComplianceSettlementDays, cfg.Compliance.SettlementDays)
	}
}

func TestLoad_MissingOptionalFile(t *testing.T) {
	cfg := Default()
	before := cfg

	err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"), &cfg, false)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !reflect.DeepEqual(before, cfg) {
		t.Fatalf("config changed when optional file was missing")
	}
}

func TestLoad_MissingRequiredFile(t *testing.T) {
	cfg := Default()
	err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"), &cfg, true)
	if err == nil {
		t.Fatalf("expected error for missing required file")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected read config error, got: %v", err)
	}
}

func TestLoad_AppliesConfigValues(t *testing.T) {
	cfg := Default()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
broker = "alpaca"
mode = "AUTO"

[alpaca]
env = "live"
base_url = "https://api.alpaca.markets"
api_key = "key123"
api_secret = "sec123"
data_url = "https://data.alpaca.markets"
feed = "sip"

[keyring]
use = false
save = false
service = "helix"
user = "paper"

[risk]
max_trade_notional = 1111
max_day_notional = 2222

[compliance]
enabled = true
account_type = "margin"
avoid_pdt = true
max_day_trades_5d = 3
min_equity_for_pdt = 25000
avoid_gfv = false
settlement_days = 1

[agent]
type = "llm"
watchlist = ["tsla", "TSLA", " nvda "]
interval = "15s"
sync_timeout = "18s"
order_timeout = "22s"
qty = 2.5
move_pct = 0.03
min_gain_pct = 1.25
max_intents = 4
dry_run = true

[agent.low_power]
enabled = true
allow_after_hours = false
closed_poll_interval = "2m"
pre_open_warmup = "12m"

[agent.llm]
api_key = "llm-key"
base_url = "https://api.openai.com/v1"
model = "gpt-4.1-mini"
timeout = "30s"
system_prompt = "be conservative"
context_log = "summary"

[strategy]
enabled = true
interval = "2h"
auto_activate = true
max_recommendations = 5
objective = "rotate towards higher momentum names"

[strategy.llm]
model = "gpt-5"
timeout = "75s"
prompt_version = "strategy-v2"
system_prompt = "deep strategy analysis"

[logging]
file = "logs/helix-debug.log"
mode = "truncate"
level = "debug"

[database]
path = "data/helix.db"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := Load(path, &cfg, true); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Broker != "alpaca" {
		t.Fatalf("unexpected broker: %q", cfg.Broker)
	}
	if cfg.Mode != "AUTO" {
		t.Fatalf("unexpected mode: %q", cfg.Mode)
	}
	if cfg.Alpaca.APIKey != "key123" || cfg.Alpaca.APISecret != "sec123" {
		t.Fatalf("unexpected alpaca credentials: %q / %q", cfg.Alpaca.APIKey, cfg.Alpaca.APISecret)
	}
	if cfg.Alpaca.Env != "live" {
		t.Fatalf("unexpected alpaca env: %q", cfg.Alpaca.Env)
	}
	if cfg.Alpaca.BaseURL != "https://api.alpaca.markets" {
		t.Fatalf("unexpected alpaca base URL: %q", cfg.Alpaca.BaseURL)
	}
	if cfg.Alpaca.DataURL != "https://data.alpaca.markets" || cfg.Alpaca.Feed != "sip" {
		t.Fatalf("unexpected alpaca data config: %q / %q", cfg.Alpaca.DataURL, cfg.Alpaca.Feed)
	}
	if cfg.Keyring.Use || cfg.Keyring.Save {
		t.Fatalf("expected keyring flags to be false")
	}
	if cfg.Keyring.Service != "helix" || cfg.Keyring.User != "paper" {
		t.Fatalf("unexpected keyring settings: %q / %q", cfg.Keyring.Service, cfg.Keyring.User)
	}
	if cfg.Risk.MaxTradeNotional != 1111 || cfg.Risk.MaxDayNotional != 2222 {
		t.Fatalf("unexpected risk settings: %f / %f", cfg.Risk.MaxTradeNotional, cfg.Risk.MaxDayNotional)
	}
	if !cfg.Compliance.Enabled || cfg.Compliance.AccountType != "margin" || !cfg.Compliance.AvoidPDT {
		t.Fatalf("unexpected compliance settings: %#v", cfg.Compliance)
	}
	if cfg.Compliance.MaxDayTrades5D != 3 || cfg.Compliance.MinEquityForPDT != 25000 {
		t.Fatalf("unexpected compliance limits: %#v", cfg.Compliance)
	}
	if cfg.Compliance.SettlementDays != 1 {
		t.Fatalf("unexpected compliance settlement days: %#v", cfg.Compliance)
	}
	if !reflect.DeepEqual(cfg.Agent.Watchlist, []string{"tsla", "TSLA", " nvda "}) {
		t.Fatalf("unexpected watchlist: %#v", cfg.Agent.Watchlist)
	}
	if cfg.Agent.Interval != 15*time.Second {
		t.Fatalf("unexpected agent interval: %s", cfg.Agent.Interval)
	}
	if cfg.Agent.SyncTimeout != 18*time.Second || cfg.Agent.OrderTimeout != 22*time.Second {
		t.Fatalf("unexpected runtime timeouts: sync=%s order=%s", cfg.Agent.SyncTimeout, cfg.Agent.OrderTimeout)
	}
	if cfg.Agent.Qty != 2.5 || cfg.Agent.MovePct != 0.03 || cfg.Agent.MaxIntents != 4 {
		t.Fatalf("unexpected agent numeric settings")
	}
	if cfg.Agent.MinGainPct != 1.25 {
		t.Fatalf("unexpected min gain pct: %f", cfg.Agent.MinGainPct)
	}
	if !cfg.Agent.DryRun {
		t.Fatalf("expected dry run to be true")
	}
	if !cfg.Agent.LowPower.Enabled || cfg.Agent.LowPower.AllowAfterHours {
		t.Fatalf("unexpected low power enabled/after-hours values: %#v", cfg.Agent.LowPower)
	}
	if cfg.Agent.LowPower.ClosedPollInterval != 2*time.Minute {
		t.Fatalf("unexpected low power closed poll interval: %s", cfg.Agent.LowPower.ClosedPollInterval)
	}
	if cfg.Agent.LowPower.PreOpenWarmup != 12*time.Minute {
		t.Fatalf("unexpected low power pre-open warmup: %s", cfg.Agent.LowPower.PreOpenWarmup)
	}
	if cfg.Agent.Type != "llm" {
		t.Fatalf("unexpected agent type: %q", cfg.Agent.Type)
	}
	if cfg.Agent.LLM.APIKey != "llm-key" {
		t.Fatalf("unexpected llm api key: %q", cfg.Agent.LLM.APIKey)
	}
	if cfg.Agent.LLM.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected llm base url: %q", cfg.Agent.LLM.BaseURL)
	}
	if cfg.Agent.LLM.Model != "gpt-4.1-mini" {
		t.Fatalf("unexpected llm model: %q", cfg.Agent.LLM.Model)
	}
	if cfg.Agent.LLM.Timeout != 30*time.Second {
		t.Fatalf("unexpected llm timeout: %s", cfg.Agent.LLM.Timeout)
	}
	if cfg.Agent.LLM.SystemPrompt != "be conservative" {
		t.Fatalf("unexpected llm system prompt: %q", cfg.Agent.LLM.SystemPrompt)
	}
	if cfg.Agent.LLM.ContextLog != "summary" {
		t.Fatalf("unexpected llm context log mode: %q", cfg.Agent.LLM.ContextLog)
	}
	if !cfg.Strategy.Enabled || cfg.Strategy.Interval != 2*time.Hour || !cfg.Strategy.AutoActivate {
		t.Fatalf("unexpected strategy settings: %#v", cfg.Strategy)
	}
	if cfg.Strategy.MaxRecommendations != 5 {
		t.Fatalf("unexpected strategy max recommendations: %d", cfg.Strategy.MaxRecommendations)
	}
	if cfg.Strategy.Objective != "rotate towards higher momentum names" {
		t.Fatalf("unexpected strategy objective: %q", cfg.Strategy.Objective)
	}
	if cfg.Strategy.LLM.Model != "gpt-5" || cfg.Strategy.LLM.Timeout != 75*time.Second {
		t.Fatalf("unexpected strategy llm settings: %#v", cfg.Strategy.LLM)
	}
	if cfg.Strategy.LLM.PromptVersion != "strategy-v2" || cfg.Strategy.LLM.SystemPrompt != "deep strategy analysis" {
		t.Fatalf("unexpected strategy llm prompt settings: %#v", cfg.Strategy.LLM)
	}
	if cfg.Logging.File != "logs/helix-debug.log" {
		t.Fatalf("unexpected log file: %q", cfg.Logging.File)
	}
	if cfg.Logging.Mode != "truncate" {
		t.Fatalf("unexpected log mode: %q", cfg.Logging.Mode)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("unexpected log level: %q", cfg.Logging.Level)
	}
	if cfg.Database.Path != "data/helix.db" {
		t.Fatalf("unexpected database path: %q", cfg.Database.Path)
	}
}

func TestLoad_InvalidAgentInterval(t *testing.T) {
	cfg := Default()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
[agent]
interval = "not-a-duration"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := Load(path, &cfg, true)
	if err == nil {
		t.Fatalf("expected interval parsing error")
	}
	if !strings.Contains(err.Error(), "decode config") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_InvalidAgentLLMTimeout(t *testing.T) {
	cfg := Default()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
[agent]
type = "llm"

[agent.llm]
timeout = "not-a-duration"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := Load(path, &cfg, true)
	if err == nil {
		t.Fatalf("expected llm timeout parsing error")
	}
	if !strings.Contains(err.Error(), "decode config") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("unexpected error: %v", err)
	}
}
