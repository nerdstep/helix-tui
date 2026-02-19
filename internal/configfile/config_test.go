package configfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/app"
	"helix-tui/internal/domain"
)

func TestLoad_MissingOptionalFile(t *testing.T) {
	cfg := app.DefaultConfig()
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
	cfg := app.DefaultConfig()
	err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"), &cfg, true)
	if err == nil {
		t.Fatalf("expected error for missing required file")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected read config error, got: %v", err)
	}
}

func TestLoad_AppliesConfigValues(t *testing.T) {
	cfg := app.DefaultConfig()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
broker = "alpaca"
mode = "AUTO"
allow_symbols = ["aapl", "AAPL", " msft "]

[alpaca]
env = " live "
base_url = " https://api.alpaca.markets "
api_key = "  key123  "
api_secret = "  sec123  "
data_url = " https://data.alpaca.markets "
feed = "sip"

[keyring]
use = false
save = false
service = " helix "
user = " paper "

[risk]
max_trade_notional = 1111
max_day_notional = 2222

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

[agent.llm]
api_key = "  llm-key  "
base_url = " https://api.openai.com/v1 "
model = " gpt-4.1-mini "
timeout = "30s"
system_prompt = "  be conservative  "

[logging]
file = " logs/helix-debug.log "
mode = "truncate"
level = "debug"

[database]
path = " data/helix.db "
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
	if cfg.Mode != domain.ModeAuto {
		t.Fatalf("unexpected mode: %q", cfg.Mode)
	}
	if !reflect.DeepEqual(cfg.AllowSymbols, []string{"AAPL", "MSFT"}) {
		t.Fatalf("unexpected allow symbols: %#v", cfg.AllowSymbols)
	}
	if cfg.AlpacaAPIKey != "key123" || cfg.AlpacaAPISecret != "sec123" {
		t.Fatalf("unexpected alpaca credentials: %q / %q", cfg.AlpacaAPIKey, cfg.AlpacaAPISecret)
	}
	if cfg.AlpacaEnv != "live" {
		t.Fatalf("unexpected alpaca env: %q", cfg.AlpacaEnv)
	}
	if cfg.AlpacaBaseURL != "https://api.alpaca.markets" {
		t.Fatalf("unexpected alpaca base URL: %q", cfg.AlpacaBaseURL)
	}
	if cfg.AlpacaDataURL != "https://data.alpaca.markets" || cfg.AlpacaFeed != "sip" {
		t.Fatalf("unexpected alpaca data config: %q / %q", cfg.AlpacaDataURL, cfg.AlpacaFeed)
	}
	if cfg.UseKeyring || cfg.SaveToKeyring {
		t.Fatalf("expected keyring flags to be false")
	}
	if cfg.KeyringService != "helix" || cfg.KeyringUser != "paper" {
		t.Fatalf("unexpected keyring settings: %q / %q", cfg.KeyringService, cfg.KeyringUser)
	}
	if cfg.MaxNotionalPerTrade != 1111 || cfg.MaxNotionalPerDay != 2222 {
		t.Fatalf("unexpected risk settings: %f / %f", cfg.MaxNotionalPerTrade, cfg.MaxNotionalPerDay)
	}
	if !reflect.DeepEqual(cfg.Watchlist, []string{"TSLA", "NVDA"}) {
		t.Fatalf("unexpected watchlist: %#v", cfg.Watchlist)
	}
	if cfg.AgentInterval != 15*time.Second {
		t.Fatalf("unexpected agent interval: %s", cfg.AgentInterval)
	}
	if cfg.SyncTimeout != 18*time.Second || cfg.OrderTimeout != 22*time.Second {
		t.Fatalf("unexpected runtime timeouts: sync=%s order=%s", cfg.SyncTimeout, cfg.OrderTimeout)
	}
	if cfg.AgentOrderQty != 2.5 || cfg.AgentMovePct != 0.03 || cfg.MaxAgentIntents != 4 {
		t.Fatalf("unexpected agent numeric settings")
	}
	if cfg.AgentMinGainPct != 1.25 {
		t.Fatalf("unexpected min gain pct: %f", cfg.AgentMinGainPct)
	}
	if !cfg.AgentDryRun {
		t.Fatalf("expected dry run to be true")
	}
	if cfg.AgentType != "llm" {
		t.Fatalf("unexpected agent type: %q", cfg.AgentType)
	}
	if cfg.LLMAPIKey != "llm-key" {
		t.Fatalf("unexpected llm api key: %q", cfg.LLMAPIKey)
	}
	if cfg.LLMBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected llm base url: %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "gpt-4.1-mini" {
		t.Fatalf("unexpected llm model: %q", cfg.LLMModel)
	}
	if cfg.LLMTimeout != 30*time.Second {
		t.Fatalf("unexpected llm timeout: %s", cfg.LLMTimeout)
	}
	if cfg.LLMSystemPrompt != "be conservative" {
		t.Fatalf("unexpected llm system prompt: %q", cfg.LLMSystemPrompt)
	}
	if cfg.LogFile != "logs/helix-debug.log" {
		t.Fatalf("unexpected log file: %q", cfg.LogFile)
	}
	if cfg.LogMode != "truncate" {
		t.Fatalf("unexpected log mode: %q", cfg.LogMode)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("unexpected log level: %q", cfg.LogLevel)
	}
	if cfg.DatabasePath != "data/helix.db" {
		t.Fatalf("unexpected database path: %q", cfg.DatabasePath)
	}
}

func TestLoad_InvalidAgentInterval(t *testing.T) {
	cfg := app.DefaultConfig()
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
	if !strings.Contains(err.Error(), "agent.interval") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_InvalidAgentLLMTimeout(t *testing.T) {
	cfg := app.DefaultConfig()
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
	if !strings.Contains(err.Error(), "agent.llm.timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_InvalidAgentSyncTimeout(t *testing.T) {
	cfg := app.DefaultConfig()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
[agent]
sync_timeout = "nope"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := Load(path, &cfg, true)
	if err == nil {
		t.Fatalf("expected sync timeout parsing error")
	}
	if !strings.Contains(err.Error(), "agent.sync_timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_InvalidLoggingMode(t *testing.T) {
	cfg := app.DefaultConfig()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
[logging]
mode = "rotate"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := Load(path, &cfg, true)
	if err == nil {
		t.Fatalf("expected logging mode validation error")
	}
	if !strings.Contains(err.Error(), "logging.mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_InvalidLoggingLevel(t *testing.T) {
	cfg := app.DefaultConfig()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
[logging]
level = "verbose"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := Load(path, &cfg, true)
	if err == nil {
		t.Fatalf("expected logging level validation error")
	}
	if !strings.Contains(err.Error(), "logging.level") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_InvalidAgentMinGainPct(t *testing.T) {
	cfg := app.DefaultConfig()
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
[agent]
min_gain_pct = -1
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	err := Load(path, &cfg, true)
	if err == nil {
		t.Fatalf("expected min gain pct validation error")
	}
	if !strings.Contains(err.Error(), "agent.min_gain_pct") {
		t.Fatalf("unexpected error: %v", err)
	}
}
