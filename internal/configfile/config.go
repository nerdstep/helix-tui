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
	Broker       string       `koanf:"broker"`
	Mode         string       `koanf:"mode"`
	AllowSymbols []string     `koanf:"allow_symbols"`
	Alpaca       AlpacaConfig `koanf:"alpaca"`
	Keyring      Keyring      `koanf:"keyring"`
	Risk         RiskConfig   `koanf:"risk"`
	Agent        AgentConfig  `koanf:"agent"`
	Logging      Logging      `koanf:"logging"`
	Database     Database     `koanf:"database"`
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
	LLM          LLMConfig     `koanf:"llm"`
}

type LLMConfig struct {
	APIKey       string        `koanf:"api_key"`
	BaseURL      string        `koanf:"base_url"`
	Model        string        `koanf:"model"`
	Timeout      time.Duration `koanf:"timeout"`
	SystemPrompt string        `koanf:"system_prompt"`
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
		Broker:       d.Broker,
		Mode:         string(d.Mode),
		AllowSymbols: cloneStrings(d.AllowSymbols),
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
			LLM: LLMConfig{
				APIKey:       d.LLMAPIKey,
				BaseURL:      d.LLMBaseURL,
				Model:        d.LLMModel,
				Timeout:      d.LLMTimeout,
				SystemPrompt: d.LLMSystemPrompt,
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
		Broker:              c.Broker,
		AlpacaAPIKey:        c.Alpaca.APIKey,
		AlpacaAPISecret:     c.Alpaca.APISecret,
		AlpacaEnv:           c.Alpaca.Env,
		AlpacaBaseURL:       c.Alpaca.BaseURL,
		AlpacaDataURL:       c.Alpaca.DataURL,
		AlpacaFeed:          c.Alpaca.Feed,
		UseKeyring:          c.Keyring.Use,
		SaveToKeyring:       c.Keyring.Save,
		KeyringService:      c.Keyring.Service,
		KeyringUser:         c.Keyring.User,
		MaxNotionalPerTrade: c.Risk.MaxTradeNotional,
		MaxNotionalPerDay:   c.Risk.MaxDayNotional,
		AllowSymbols:        cloneStrings(c.AllowSymbols),
		Mode:                domain.Mode(c.Mode),
		Watchlist:           cloneStrings(c.Agent.Watchlist),
		AgentType:           c.Agent.Type,
		AgentInterval:       c.Agent.Interval,
		AgentOrderQty:       c.Agent.Qty,
		AgentMovePct:        c.Agent.MovePct,
		AgentMinGainPct:     c.Agent.MinGainPct,
		MaxAgentIntents:     c.Agent.MaxIntents,
		AgentDryRun:         c.Agent.DryRun,
		SyncTimeout:         c.Agent.SyncTimeout,
		OrderTimeout:        c.Agent.OrderTimeout,
		LogFile:             c.Logging.File,
		LogMode:             c.Logging.Mode,
		LogLevel:            c.Logging.Level,
		DatabasePath:        c.Database.Path,
		LLMAPIKey:           c.Agent.LLM.APIKey,
		LLMBaseURL:          c.Agent.LLM.BaseURL,
		LLMModel:            c.Agent.LLM.Model,
		LLMTimeout:          c.Agent.LLM.Timeout,
		LLMSystemPrompt:     c.Agent.LLM.SystemPrompt,
	}
}

func (c *Config) Normalize() {
	c.Broker = strings.TrimSpace(c.Broker)
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	c.AllowSymbols = symbols.Normalize(c.AllowSymbols)

	c.Alpaca.Env = strings.TrimSpace(c.Alpaca.Env)
	c.Alpaca.BaseURL = strings.TrimSpace(c.Alpaca.BaseURL)
	c.Alpaca.APIKey = strings.TrimSpace(c.Alpaca.APIKey)
	c.Alpaca.APISecret = strings.TrimSpace(c.Alpaca.APISecret)
	c.Alpaca.DataURL = strings.TrimSpace(c.Alpaca.DataURL)
	c.Alpaca.Feed = strings.TrimSpace(c.Alpaca.Feed)

	c.Keyring.Service = strings.TrimSpace(c.Keyring.Service)
	c.Keyring.User = strings.TrimSpace(c.Keyring.User)

	c.Agent.Type = strings.ToLower(strings.TrimSpace(c.Agent.Type))
	c.Agent.Watchlist = symbols.Normalize(c.Agent.Watchlist)
	c.Agent.LLM.APIKey = strings.TrimSpace(c.Agent.LLM.APIKey)
	c.Agent.LLM.BaseURL = strings.TrimSpace(c.Agent.LLM.BaseURL)
	c.Agent.LLM.Model = strings.TrimSpace(c.Agent.LLM.Model)
	c.Agent.LLM.SystemPrompt = strings.TrimSpace(c.Agent.LLM.SystemPrompt)

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
