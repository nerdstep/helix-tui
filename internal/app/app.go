package app

import (
	"fmt"
	"strings"
	"time"

	"helix-tui/internal/autonomy"
	"helix-tui/internal/broker/alpaca"
	"helix-tui/internal/credentials"
	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
)

type Config struct {
	Broker              string
	AlpacaAPIKey        string
	AlpacaAPISecret     string
	AlpacaEnv           string
	AlpacaBaseURL       string
	AlpacaDataURL       string
	AlpacaFeed          string
	UseKeyring          bool
	SaveToKeyring       bool
	KeyringService      string
	KeyringUser         string
	MaxNotionalPerTrade float64
	MaxNotionalPerDay   float64
	AllowSymbols        []string
	Mode                domain.Mode
	Watchlist           []string
	AgentType           string
	AgentInterval       time.Duration
	AgentOrderQty       float64
	AgentMovePct        float64
	MaxAgentIntents     int
	AgentDryRun         bool
	AgentObjective      string
	LLMAPIKey           string
	LLMBaseURL          string
	LLMModel            string
	LLMTimeout          time.Duration
	LLMSystemPrompt     string
}

type System struct {
	Engine             *engine.Engine
	Runner             *autonomy.Runner
	Watchlist          []string
	PullWatchlist      func() ([]string, error)
	SyncWatchlist      func([]string) error
	DefaultBrokerLabel string
}

const defaultAlpacaWatchlistName = "helix-tui"

func DefaultConfig() Config {
	return Config{
		Broker:              "paper",
		AlpacaEnv:           alpaca.EnvPaper,
		AlpacaFeed:          "iex",
		UseKeyring:          true,
		SaveToKeyring:       true,
		KeyringService:      credentials.DefaultService,
		KeyringUser:         credentials.DefaultUser,
		MaxNotionalPerTrade: 5000,
		MaxNotionalPerDay:   20000,
		AllowSymbols: []string{
			"AAPL",
			"MSFT",
			"TSLA",
			"NVDA",
			"AMZN",
			"GOOGL",
		},
		Mode:            domain.ModeManual,
		Watchlist:       []string{"AAPL", "MSFT", "TSLA", "NVDA"},
		AgentType:       "heuristic",
		AgentInterval:   10 * time.Second,
		AgentOrderQty:   1,
		AgentMovePct:    0.01,
		MaxAgentIntents: 1,
		AgentObjective:  "Generate conservative, risk-aware trade intents.",
		LLMBaseURL:      "https://api.openai.com/v1",
		LLMModel:        "gpt-4.1-mini",
		LLMTimeout:      20 * time.Second,
	}
}

func NewSystem(cfg Config) (*System, error) {
	brokerSpec, err := buildBroker(cfg)
	if err != nil {
		return nil, err
	}
	watchlist, watchlistPullErr := resolveWatchlist(cfg, brokerSpec.watchlistSyncBroker)
	allowSymbols := buildAllowSymbols(cfg.AllowSymbols, watchlist)
	e, err := buildEngine(cfg, brokerSpec.broker, allowSymbols)
	if err != nil {
		return nil, err
	}
	if brokerSpec.isAlpaca {
		addAlpacaConfigEvent(e, cfg, brokerSpec.credentialSource)
	}
	if watchlistPullErr != nil {
		e.AddEvent("watchlist_sync_error", fmt.Sprintf("pull: %v", watchlistPullErr))
	}

	system := &System{
		Engine:             e,
		Watchlist:          watchlist,
		DefaultBrokerLabel: brokerSpec.label,
	}
	system.PullWatchlist, system.SyncWatchlist = buildWatchlistHandlers(brokerSpec.watchlistSyncBroker)

	runner, mode, agentType, err := buildRunner(cfg, brokerSpec.broker, e, watchlist)
	if err != nil {
		return nil, err
	}
	if runner != nil {
		system.Runner = runner
		e.AddEvent("agent_mode", fmt.Sprintf("mode=%s agent=%s watchlist=%s", mode, agentType, strings.Join(watchlist, ",")))
	}
	return system, nil
}

func effectiveAlpacaEnv(cfg Config) string {
	return alpaca.NormalizeEnv(cfg.AlpacaEnv)
}

func newAlpacaBroker(cfg Config, apiKey, apiSecret string) *alpaca.Broker {
	return alpaca.NewForEnv(
		effectiveAlpacaEnv(cfg),
		apiKey,
		apiSecret,
		cfg.AlpacaBaseURL,
		cfg.AlpacaDataURL,
		cfg.AlpacaFeed,
	)
}

func NewEngine(cfg Config) (*engine.Engine, error) {
	system, err := NewSystem(cfg)
	if err != nil {
		return nil, err
	}
	return system.Engine, nil
}

func normalizeMode(mode domain.Mode) domain.Mode {
	switch domain.Mode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case domain.ModeManual:
		return domain.ModeManual
	case domain.ModeAssist:
		return domain.ModeAssist
	case domain.ModeAuto:
		return domain.ModeAuto
	default:
		return domain.ModeManual
	}
}
