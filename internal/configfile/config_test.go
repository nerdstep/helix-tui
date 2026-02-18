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
broker = "alpaca-paper"
mode = "AUTO"
allow_symbols = ["aapl", "AAPL", " msft "]

[alpaca]
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
watchlist = ["tsla", "TSLA", " nvda "]
interval = "15s"
qty = 2.5
move_pct = 0.03
max_intents = 4
dry_run = true
objective = "  test objective  "
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := Load(path, &cfg, true); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Broker != "alpaca-paper" {
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
	if cfg.AgentOrderQty != 2.5 || cfg.AgentMovePct != 0.03 || cfg.MaxAgentIntents != 4 {
		t.Fatalf("unexpected agent numeric settings")
	}
	if !cfg.AgentDryRun {
		t.Fatalf("expected dry run to be true")
	}
	if cfg.AgentObjective != "test objective" {
		t.Fatalf("unexpected objective: %q", cfg.AgentObjective)
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
