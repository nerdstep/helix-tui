package configfile

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	tomlparser "github.com/knadh/koanf/parsers/toml/v2"
	fileprovider "github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"helix-tui/internal/app"
	"helix-tui/internal/domain"
	"helix-tui/internal/symbols"
)

const DefaultPath = "config.toml"

type Config struct {
	Broker     string           `koanf:"broker"`
	Mode       string           `koanf:"mode"`
	Identity   IdentityConfig   `koanf:"identity"`
	Alpaca     AlpacaConfig     `koanf:"alpaca"`
	Keyring    Keyring          `koanf:"keyring"`
	Risk       RiskConfig       `koanf:"risk"`
	Compliance ComplianceConfig `koanf:"compliance"`
	Agent      AgentConfig      `koanf:"agent"`
	Strategy   StrategyConfig   `koanf:"strategy"`
	Logging    Logging          `koanf:"logging"`
	Database   Database         `koanf:"database"`
}

type IdentityConfig struct {
	HumanName  string `koanf:"human_name"`
	HumanAlias string `koanf:"human_alias"`
	AgentName  string `koanf:"agent_name"`
}

type AlpacaConfig struct {
	Env       string `koanf:"env"`
	BaseURL   string `koanf:"base_url"`
	APIKey    string `koanf:"api_key"`
	APISecret string `koanf:"api_secret"`
	DataURL   string `koanf:"data_url"`
	Feed      string `koanf:"feed"`
}

type Keyring struct {
	Use     bool   `koanf:"use"`
	Save    bool   `koanf:"save"`
	Service string `koanf:"service"`
	User    string `koanf:"user"`
}

type RiskConfig struct {
	MaxTradeNotional float64 `koanf:"max_trade_notional"`
	MaxDayNotional   float64 `koanf:"max_day_notional"`
}

type ComplianceConfig struct {
	Enabled         bool    `koanf:"enabled"`
	AccountType     string  `koanf:"account_type"`
	AvoidPDT        bool    `koanf:"avoid_pdt"`
	MaxDayTrades5D  int     `koanf:"max_day_trades_5d"`
	MinEquityForPDT float64 `koanf:"min_equity_for_pdt"`
	AvoidGoodFaith  bool    `koanf:"avoid_gfv"`
	SettlementDays  int     `koanf:"settlement_days"`
}

type AgentConfig struct {
	Type         string        `koanf:"type"`
	Watchlist    []string      `koanf:"watchlist"`
	Interval     time.Duration `koanf:"interval"`
	SyncTimeout  time.Duration `koanf:"sync_timeout"`
	OrderTimeout time.Duration `koanf:"order_timeout"`
	Qty          float64       `koanf:"qty"`
	MovePct      float64       `koanf:"move_pct"`
	MinGainPct   float64       `koanf:"min_gain_pct"`
	MaxIntents   int           `koanf:"max_intents"`
	DryRun       bool          `koanf:"dry_run"`
	LowPower     LowPower      `koanf:"low_power"`
	LLM          LLMConfig     `koanf:"llm"`
}

type LowPower struct {
	Enabled            bool          `koanf:"enabled"`
	AllowAfterHours    bool          `koanf:"allow_after_hours"`
	ClosedPollInterval time.Duration `koanf:"closed_poll_interval"`
	PreOpenWarmup      time.Duration `koanf:"pre_open_warmup"`
}

type LLMConfig struct {
	APIKey       string        `koanf:"api_key"`
	BaseURL      string        `koanf:"base_url"`
	Model        string        `koanf:"model"`
	Timeout      time.Duration `koanf:"timeout"`
	SystemPrompt string        `koanf:"system_prompt"`
	ContextLog   string        `koanf:"context_log"`
}

type StrategyConfig struct {
	Enabled            bool              `koanf:"enabled"`
	Interval           time.Duration     `koanf:"interval"`
	AutoActivate       bool              `koanf:"auto_activate"`
	MaxRecommendations int               `koanf:"max_recommendations"`
	Objective          string            `koanf:"objective"`
	LLM                StrategyLLMConfig `koanf:"llm"`
}

type StrategyLLMConfig struct {
	Model         string        `koanf:"model"`
	Timeout       time.Duration `koanf:"timeout"`
	SystemPrompt  string        `koanf:"system_prompt"`
	PromptVersion string        `koanf:"prompt_version"`
}

type Logging struct {
	File  string `koanf:"file"`
	Mode  string `koanf:"mode"`
	Level string `koanf:"level"`
}

type Database struct {
	Path string `koanf:"path"`
}

func Default() Config {
	d := app.DefaultConfig()
	return Config{
		Broker: d.Broker,
		Mode:   string(d.Mode),
		Identity: IdentityConfig{
			HumanName:  d.HumanName,
			HumanAlias: d.HumanAlias,
			AgentName:  d.AgentName,
		},
		Alpaca: AlpacaConfig{
			Env:       d.AlpacaEnv,
			BaseURL:   d.AlpacaBaseURL,
			APIKey:    d.AlpacaAPIKey,
			APISecret: d.AlpacaAPISecret,
			DataURL:   d.AlpacaDataURL,
			Feed:      d.AlpacaFeed,
		},
		Keyring: Keyring{
			Use:     d.UseKeyring,
			Save:    d.SaveToKeyring,
			Service: d.KeyringService,
			User:    d.KeyringUser,
		},
		Risk: RiskConfig{
			MaxTradeNotional: d.MaxNotionalPerTrade,
			MaxDayNotional:   d.MaxNotionalPerDay,
		},
		Compliance: ComplianceConfig{
			Enabled:         d.ComplianceEnabled,
			AccountType:     d.ComplianceAccountType,
			AvoidPDT:        d.ComplianceAvoidPDT,
			MaxDayTrades5D:  d.ComplianceMaxDayTrades5D,
			MinEquityForPDT: d.ComplianceMinEquityForPDT,
			AvoidGoodFaith:  d.ComplianceAvoidGoodFaith,
			SettlementDays:  d.ComplianceSettlementDays,
		},
		Agent: AgentConfig{
			Type:         d.AgentType,
			Watchlist:    cloneStrings(d.Watchlist),
			Interval:     d.AgentInterval,
			SyncTimeout:  d.SyncTimeout,
			OrderTimeout: d.OrderTimeout,
			Qty:          d.AgentOrderQty,
			MovePct:      d.AgentMovePct,
			MinGainPct:   d.AgentMinGainPct,
			MaxIntents:   d.MaxAgentIntents,
			DryRun:       d.AgentDryRun,
			LowPower: LowPower{
				Enabled:            d.AgentLowPowerEnabled,
				AllowAfterHours:    d.AgentAllowAfterHours,
				ClosedPollInterval: d.AgentClosedPollInterval,
				PreOpenWarmup:      d.AgentPreOpenWarmup,
			},
			LLM: LLMConfig{
				APIKey:       d.LLMAPIKey,
				BaseURL:      d.LLMBaseURL,
				Model:        d.LLMModel,
				Timeout:      d.LLMTimeout,
				SystemPrompt: d.LLMSystemPrompt,
				ContextLog:   d.LLMContextLog,
			},
		},
		Strategy: StrategyConfig{
			Enabled:            d.StrategyEnabled,
			Interval:           d.StrategyInterval,
			AutoActivate:       d.StrategyAutoActivate,
			MaxRecommendations: d.StrategyMaxRecommendations,
			Objective:          d.StrategyObjective,
			LLM: StrategyLLMConfig{
				Model:         d.StrategyModel,
				Timeout:       d.StrategyTimeout,
				SystemPrompt:  d.StrategySystemPrompt,
				PromptVersion: d.StrategyPromptVersion,
			},
		},
		Logging: Logging{
			File:  d.LogFile,
			Mode:  d.LogMode,
			Level: d.LogLevel,
		},
		Database: Database{
			Path: d.DatabasePath,
		},
	}
}

func (c Config) ToAppConfig() app.Config {
	return app.Config{
		Broker:                     c.Broker,
		HumanName:                  c.Identity.HumanName,
		HumanAlias:                 c.Identity.HumanAlias,
		AgentName:                  c.Identity.AgentName,
		AlpacaAPIKey:               c.Alpaca.APIKey,
		AlpacaAPISecret:            c.Alpaca.APISecret,
		AlpacaEnv:                  c.Alpaca.Env,
		AlpacaBaseURL:              c.Alpaca.BaseURL,
		AlpacaDataURL:              c.Alpaca.DataURL,
		AlpacaFeed:                 c.Alpaca.Feed,
		UseKeyring:                 c.Keyring.Use,
		SaveToKeyring:              c.Keyring.Save,
		KeyringService:             c.Keyring.Service,
		KeyringUser:                c.Keyring.User,
		MaxNotionalPerTrade:        c.Risk.MaxTradeNotional,
		MaxNotionalPerDay:          c.Risk.MaxDayNotional,
		ComplianceEnabled:          c.Compliance.Enabled,
		ComplianceAccountType:      c.Compliance.AccountType,
		ComplianceAvoidPDT:         c.Compliance.AvoidPDT,
		ComplianceMaxDayTrades5D:   c.Compliance.MaxDayTrades5D,
		ComplianceMinEquityForPDT:  c.Compliance.MinEquityForPDT,
		ComplianceAvoidGoodFaith:   c.Compliance.AvoidGoodFaith,
		ComplianceSettlementDays:   c.Compliance.SettlementDays,
		Mode:                       domain.Mode(c.Mode),
		Watchlist:                  cloneStrings(c.Agent.Watchlist),
		AgentType:                  c.Agent.Type,
		AgentInterval:              c.Agent.Interval,
		AgentOrderQty:              c.Agent.Qty,
		AgentMovePct:               c.Agent.MovePct,
		AgentMinGainPct:            c.Agent.MinGainPct,
		MaxAgentIntents:            c.Agent.MaxIntents,
		AgentDryRun:                c.Agent.DryRun,
		AgentLowPowerEnabled:       c.Agent.LowPower.Enabled,
		AgentAllowAfterHours:       c.Agent.LowPower.AllowAfterHours,
		AgentClosedPollInterval:    c.Agent.LowPower.ClosedPollInterval,
		AgentPreOpenWarmup:         c.Agent.LowPower.PreOpenWarmup,
		SyncTimeout:                c.Agent.SyncTimeout,
		OrderTimeout:               c.Agent.OrderTimeout,
		LogFile:                    c.Logging.File,
		LogMode:                    c.Logging.Mode,
		LogLevel:                   c.Logging.Level,
		DatabasePath:               c.Database.Path,
		LLMAPIKey:                  c.Agent.LLM.APIKey,
		LLMBaseURL:                 c.Agent.LLM.BaseURL,
		LLMModel:                   c.Agent.LLM.Model,
		LLMTimeout:                 c.Agent.LLM.Timeout,
		LLMSystemPrompt:            c.Agent.LLM.SystemPrompt,
		LLMContextLog:              c.Agent.LLM.ContextLog,
		StrategyEnabled:            c.Strategy.Enabled,
		StrategyInterval:           c.Strategy.Interval,
		StrategyAutoActivate:       c.Strategy.AutoActivate,
		StrategyMaxRecommendations: c.Strategy.MaxRecommendations,
		StrategyObjective:          c.Strategy.Objective,
		StrategyModel:              c.Strategy.LLM.Model,
		StrategyTimeout:            c.Strategy.LLM.Timeout,
		StrategySystemPrompt:       c.Strategy.LLM.SystemPrompt,
		StrategyPromptVersion:      c.Strategy.LLM.PromptVersion,
	}
}

func (c *Config) Normalize() {
	c.Broker = strings.TrimSpace(c.Broker)
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	c.Identity.HumanName = strings.TrimSpace(c.Identity.HumanName)
	c.Identity.HumanAlias = strings.TrimSpace(c.Identity.HumanAlias)
	c.Identity.AgentName = strings.TrimSpace(c.Identity.AgentName)

	c.Alpaca.Env = strings.TrimSpace(c.Alpaca.Env)
	c.Alpaca.BaseURL = strings.TrimSpace(c.Alpaca.BaseURL)
	c.Alpaca.APIKey = strings.TrimSpace(c.Alpaca.APIKey)
	c.Alpaca.APISecret = strings.TrimSpace(c.Alpaca.APISecret)
	c.Alpaca.DataURL = strings.TrimSpace(c.Alpaca.DataURL)
	c.Alpaca.Feed = strings.TrimSpace(c.Alpaca.Feed)

	c.Keyring.Service = strings.TrimSpace(c.Keyring.Service)
	c.Keyring.User = strings.TrimSpace(c.Keyring.User)
	c.Compliance.AccountType = strings.ToLower(strings.TrimSpace(c.Compliance.AccountType))

	c.Agent.Type = strings.ToLower(strings.TrimSpace(c.Agent.Type))
	c.Agent.Watchlist = symbols.Normalize(c.Agent.Watchlist)
	c.Agent.LLM.APIKey = strings.TrimSpace(c.Agent.LLM.APIKey)
	c.Agent.LLM.BaseURL = strings.TrimSpace(c.Agent.LLM.BaseURL)
	c.Agent.LLM.Model = strings.TrimSpace(c.Agent.LLM.Model)
	c.Agent.LLM.SystemPrompt = strings.TrimSpace(c.Agent.LLM.SystemPrompt)
	c.Agent.LLM.ContextLog = strings.ToLower(strings.TrimSpace(c.Agent.LLM.ContextLog))

	c.Strategy.Objective = strings.TrimSpace(c.Strategy.Objective)
	c.Strategy.LLM.Model = strings.TrimSpace(c.Strategy.LLM.Model)
	c.Strategy.LLM.SystemPrompt = strings.TrimSpace(c.Strategy.LLM.SystemPrompt)
	c.Strategy.LLM.PromptVersion = strings.TrimSpace(c.Strategy.LLM.PromptVersion)

	c.Logging.File = strings.TrimSpace(c.Logging.File)
	c.Logging.Mode = strings.ToLower(strings.TrimSpace(c.Logging.Mode))
	c.Logging.Level = strings.ToLower(strings.TrimSpace(c.Logging.Level))

	c.Database.Path = strings.TrimSpace(c.Database.Path)
}

func Load(path string, cfg *Config, required bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultPath
	}

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return nil
		}
		return fmt.Errorf("read config %q: %w", path, err)
	}

	k := koanf.New(".")
	if err := k.Load(fileprovider.Provider(path), tomlparser.Parser()); err != nil {
		return fmt.Errorf("parse config %q: %w", path, err)
	}

	if err := k.UnmarshalWithConf("", cfg, koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
			WeaklyTypedInput: true,
		},
	}); err != nil {
		return fmt.Errorf("decode config %q: %w", path, err)
	}
	return nil
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
