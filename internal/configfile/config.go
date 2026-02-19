package configfile

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"helix-tui/internal/app"
	"helix-tui/internal/domain"
)

const DefaultPath = "config.toml"

type fileConfig struct {
	Broker       *string       `toml:"broker"`
	Mode         *string       `toml:"mode"`
	AllowSymbols []string      `toml:"allow_symbols"`
	Alpaca       alpacaConfig  `toml:"alpaca"`
	Risk         riskConfig    `toml:"risk"`
	Agent        agentConfig   `toml:"agent"`
	Keyring      keyringConfig `toml:"keyring"`
}

type alpacaConfig struct {
	Env       *string `toml:"env"`
	BaseURL   *string `toml:"base_url"`
	APIKey    *string `toml:"api_key"`
	APISecret *string `toml:"api_secret"`
	DataURL   *string `toml:"data_url"`
	Feed      *string `toml:"feed"`
}

type keyringConfig struct {
	Use     *bool   `toml:"use"`
	Save    *bool   `toml:"save"`
	Service *string `toml:"service"`
	User    *string `toml:"user"`
}

type riskConfig struct {
	MaxTradeNotional *float64 `toml:"max_trade_notional"`
	MaxDayNotional   *float64 `toml:"max_day_notional"`
}

type agentConfig struct {
	Watchlist  []string `toml:"watchlist"`
	Interval   *string  `toml:"interval"`
	Qty        *float64 `toml:"qty"`
	MovePct    *float64 `toml:"move_pct"`
	MaxIntents *int     `toml:"max_intents"`
	DryRun     *bool    `toml:"dry_run"`
	Objective  *string  `toml:"objective"`
}

func Load(path string, cfg *app.Config, required bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultPath
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return nil
		}
		return fmt.Errorf("read config %q: %w", path, err)
	}

	var in fileConfig
	if err := toml.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("parse config %q: %w", path, err)
	}

	if err := applyFileConfig(cfg, in); err != nil {
		return fmt.Errorf("apply config %q: %w", path, err)
	}
	return nil
}

func applyFileConfig(cfg *app.Config, in fileConfig) error {
	if in.Broker != nil {
		cfg.Broker = strings.TrimSpace(*in.Broker)
	}
	if in.Mode != nil {
		cfg.Mode = domain.Mode(strings.ToLower(strings.TrimSpace(*in.Mode)))
	}
	if in.AllowSymbols != nil {
		cfg.AllowSymbols = normalizeSymbols(in.AllowSymbols)
	}

	if in.Alpaca.APIKey != nil {
		cfg.AlpacaAPIKey = strings.TrimSpace(*in.Alpaca.APIKey)
	}
	if in.Alpaca.APISecret != nil {
		cfg.AlpacaAPISecret = strings.TrimSpace(*in.Alpaca.APISecret)
	}
	if in.Alpaca.Env != nil {
		cfg.AlpacaEnv = strings.TrimSpace(*in.Alpaca.Env)
	}
	if in.Alpaca.BaseURL != nil {
		cfg.AlpacaBaseURL = strings.TrimSpace(*in.Alpaca.BaseURL)
	}
	if in.Alpaca.DataURL != nil {
		cfg.AlpacaDataURL = strings.TrimSpace(*in.Alpaca.DataURL)
	}
	if in.Alpaca.Feed != nil {
		cfg.AlpacaFeed = strings.TrimSpace(*in.Alpaca.Feed)
	}

	if in.Keyring.Use != nil {
		cfg.UseKeyring = *in.Keyring.Use
	}
	if in.Keyring.Save != nil {
		cfg.SaveToKeyring = *in.Keyring.Save
	}
	if in.Keyring.Service != nil {
		cfg.KeyringService = strings.TrimSpace(*in.Keyring.Service)
	}
	if in.Keyring.User != nil {
		cfg.KeyringUser = strings.TrimSpace(*in.Keyring.User)
	}

	if in.Risk.MaxTradeNotional != nil {
		cfg.MaxNotionalPerTrade = *in.Risk.MaxTradeNotional
	}
	if in.Risk.MaxDayNotional != nil {
		cfg.MaxNotionalPerDay = *in.Risk.MaxDayNotional
	}

	if in.Agent.Watchlist != nil {
		cfg.Watchlist = normalizeSymbols(in.Agent.Watchlist)
	}
	if in.Agent.Interval != nil {
		d, err := time.ParseDuration(strings.TrimSpace(*in.Agent.Interval))
		if err != nil {
			return fmt.Errorf("agent.interval must be a valid duration: %w", err)
		}
		cfg.AgentInterval = d
	}
	if in.Agent.Qty != nil {
		cfg.AgentOrderQty = *in.Agent.Qty
	}
	if in.Agent.MovePct != nil {
		cfg.AgentMovePct = *in.Agent.MovePct
	}
	if in.Agent.MaxIntents != nil {
		cfg.MaxAgentIntents = *in.Agent.MaxIntents
	}
	if in.Agent.DryRun != nil {
		cfg.AgentDryRun = *in.Agent.DryRun
	}
	if in.Agent.Objective != nil {
		cfg.AgentObjective = strings.TrimSpace(*in.Agent.Objective)
	}
	return nil
}

func normalizeSymbols(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, s := range raw {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
