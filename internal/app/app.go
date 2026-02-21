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
	"helix-tui/internal/storage"
)

type Config struct {
	Broker                    string
	AlpacaAPIKey              string
	AlpacaAPISecret           string
	AlpacaEnv                 string
	AlpacaBaseURL             string
	AlpacaDataURL             string
	AlpacaFeed                string
	UseKeyring                bool
	SaveToKeyring             bool
	KeyringService            string
	KeyringUser               string
	MaxNotionalPerTrade       float64
	MaxNotionalPerDay         float64
	ComplianceEnabled         bool
	ComplianceAccountType     string
	ComplianceAvoidPDT        bool
	ComplianceMaxDayTrades5D  int
	ComplianceMinEquityForPDT float64
	ComplianceAvoidGoodFaith  bool
	ComplianceSettlementDays  int
	Mode                      domain.Mode
	Watchlist                 []string
	AgentType                 string
	AgentInterval             time.Duration
	AgentOrderQty             float64
	AgentMovePct              float64
	AgentMinGainPct           float64
	MaxAgentIntents           int
	AgentDryRun               bool
	SyncTimeout               time.Duration
	OrderTimeout              time.Duration
	LogFile                   string
	LogMode                   string
	LogLevel                  string
	DatabasePath              string
	LLMAPIKey                 string
	LLMBaseURL                string
	LLMModel                  string
	LLMTimeout                time.Duration
	LLMSystemPrompt           string
	LLMContextLog             string
}

type System struct {
	Engine             *engine.Engine
	Runner             *autonomy.Runner
	Watchlist          []string
	PullWatchlist      func() ([]string, error)
	SyncWatchlist      func([]string) error
	QuoteStreamer      domain.QuoteStreamer
	SettlementCalendar engine.ComplianceSettlementCalendar
	DefaultBrokerLabel string
}

const defaultAlpacaWatchlistName = "helix-tui"

func DefaultConfig() Config {
	return Config{
		Broker:                    "paper",
		AlpacaEnv:                 alpaca.EnvPaper,
		AlpacaFeed:                "iex",
		UseKeyring:                true,
		SaveToKeyring:             true,
		KeyringService:            credentials.DefaultService,
		KeyringUser:               credentials.DefaultUser,
		MaxNotionalPerTrade:       5000,
		MaxNotionalPerDay:         20000,
		ComplianceEnabled:         false,
		ComplianceAccountType:     "auto",
		ComplianceAvoidPDT:        true,
		ComplianceMaxDayTrades5D:  3,
		ComplianceMinEquityForPDT: 25000,
		ComplianceAvoidGoodFaith:  false,
		ComplianceSettlementDays:  1,
		Mode:                      domain.ModeManual,
		Watchlist:                 []string{"AAPL", "MSFT", "TSLA", "NVDA"},
		AgentType:                 "heuristic",
		AgentInterval:             10 * time.Second,
		AgentOrderQty:             1,
		AgentMovePct:              0.01,
		AgentMinGainPct:           0,
		MaxAgentIntents:           1,
		SyncTimeout:               15 * time.Second,
		OrderTimeout:              15 * time.Second,
		LogMode:                   "append",
		LogLevel:                  "info",
		DatabasePath:              storage.DefaultPath,
		LLMBaseURL:                "https://api.openai.com/v1",
		LLMModel:                  "gpt-4.1-mini",
		LLMTimeout:                20 * time.Second,
		LLMContextLog:             "off",
	}
}

func NewSystem(cfg Config) (*System, error) {
	brokerSpec, err := buildBroker(cfg)
	if err != nil {
		return nil, err
	}
	watchlist, watchlistPullErr := resolveWatchlist(cfg, brokerSpec.watchlistSyncBroker)
	allowSymbols := buildAllowSymbols(watchlist)
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
		QuoteStreamer:      brokerSpec.quoteStreamer,
		SettlementCalendar: brokerSpec.settlementCalendar,
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
