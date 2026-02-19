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
	"helix-tui/internal/symbols"
)

const DefaultPath = "config.toml"

type fileConfig struct {
	Broker       *string        `toml:"broker"`
	Mode         *string        `toml:"mode"`
	AllowSymbols []string       `toml:"allow_symbols"`
	Alpaca       alpacaConfig   `toml:"alpaca"`
	Risk         riskConfig     `toml:"risk"`
	Agent        agentConfig    `toml:"agent"`
	Keyring      keyringConfig  `toml:"keyring"`
	Database     databaseConfig `toml:"database"`
	Logging      loggingConfig  `toml:"logging"`
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

type loggingConfig struct {
	File *string `toml:"file"`
	Mode *string `toml:"mode"`
}

type databaseConfig struct {
	Path *string `toml:"path"`
}

type agentConfig struct {
	Type         *string        `toml:"type"`
	Watchlist    []string       `toml:"watchlist"`
	Interval     *string        `toml:"interval"`
	SyncTimeout  *string        `toml:"sync_timeout"`
	OrderTimeout *string        `toml:"order_timeout"`
	Qty          *float64       `toml:"qty"`
	MovePct      *float64       `toml:"move_pct"`
	MinGainPct   *float64       `toml:"min_gain_pct"`
	MaxIntents   *int           `toml:"max_intents"`
	DryRun       *bool          `toml:"dry_run"`
	LLM          llmAgentConfig `toml:"llm"`
}

type llmAgentConfig struct {
	APIKey       *string `toml:"api_key"`
	BaseURL      *string `toml:"base_url"`
	Model        *string `toml:"model"`
	Timeout      *string `toml:"timeout"`
	SystemPrompt *string `toml:"system_prompt"`
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
		cfg.AllowSymbols = symbols.Normalize(in.AllowSymbols)
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
	if in.Database.Path != nil {
		cfg.DatabasePath = strings.TrimSpace(*in.Database.Path)
	}
	if in.Logging.File != nil {
		cfg.LogFile = strings.TrimSpace(*in.Logging.File)
	}
	if in.Logging.Mode != nil {
		mode := strings.ToLower(strings.TrimSpace(*in.Logging.Mode))
		switch mode {
		case "", "append", "truncate":
			if mode != "" {
				cfg.LogMode = mode
			}
		default:
			return fmt.Errorf("logging.mode must be append or truncate")
		}
	}

	if in.Agent.Watchlist != nil {
		cfg.Watchlist = symbols.Normalize(in.Agent.Watchlist)
	}
	if in.Agent.Type != nil {
		cfg.AgentType = strings.ToLower(strings.TrimSpace(*in.Agent.Type))
	}
	if in.Agent.Interval != nil {
		d, err := time.ParseDuration(strings.TrimSpace(*in.Agent.Interval))
		if err != nil {
			return fmt.Errorf("agent.interval must be a valid duration: %w", err)
		}
		cfg.AgentInterval = d
	}
	if in.Agent.SyncTimeout != nil {
		d, err := time.ParseDuration(strings.TrimSpace(*in.Agent.SyncTimeout))
		if err != nil {
			return fmt.Errorf("agent.sync_timeout must be a valid duration: %w", err)
		}
		cfg.SyncTimeout = d
	}
	if in.Agent.OrderTimeout != nil {
		d, err := time.ParseDuration(strings.TrimSpace(*in.Agent.OrderTimeout))
		if err != nil {
			return fmt.Errorf("agent.order_timeout must be a valid duration: %w", err)
		}
		cfg.OrderTimeout = d
	}
	if in.Agent.Qty != nil {
		cfg.AgentOrderQty = *in.Agent.Qty
	}
	if in.Agent.MovePct != nil {
		cfg.AgentMovePct = *in.Agent.MovePct
	}
	if in.Agent.MinGainPct != nil {
		if *in.Agent.MinGainPct < 0 {
			return fmt.Errorf("agent.min_gain_pct must be >= 0")
		}
		cfg.AgentMinGainPct = *in.Agent.MinGainPct
	}
	if in.Agent.MaxIntents != nil {
		cfg.MaxAgentIntents = *in.Agent.MaxIntents
	}
	if in.Agent.DryRun != nil {
		cfg.AgentDryRun = *in.Agent.DryRun
	}
	if in.Agent.LLM.APIKey != nil {
		cfg.LLMAPIKey = strings.TrimSpace(*in.Agent.LLM.APIKey)
	}
	if in.Agent.LLM.BaseURL != nil {
		cfg.LLMBaseURL = strings.TrimSpace(*in.Agent.LLM.BaseURL)
	}
	if in.Agent.LLM.Model != nil {
		cfg.LLMModel = strings.TrimSpace(*in.Agent.LLM.Model)
	}
	if in.Agent.LLM.Timeout != nil {
		d, err := time.ParseDuration(strings.TrimSpace(*in.Agent.LLM.Timeout))
		if err != nil {
			return fmt.Errorf("agent.llm.timeout must be a valid duration: %w", err)
		}
		cfg.LLMTimeout = d
	}
	if in.Agent.LLM.SystemPrompt != nil {
		cfg.LLMSystemPrompt = strings.TrimSpace(*in.Agent.LLM.SystemPrompt)
	}
	return nil
}
