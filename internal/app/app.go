package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"helix-tui/internal/agent/heuristic"
	"helix-tui/internal/autonomy"
	"helix-tui/internal/broker/alpaca"
	"helix-tui/internal/broker/paper"
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
	AgentInterval       time.Duration
	AgentOrderQty       float64
	AgentMovePct        float64
	MaxAgentIntents     int
	AgentDryRun         bool
	AgentObjective      string
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
		AgentInterval:   10 * time.Second,
		AgentOrderQty:   1,
		AgentMovePct:    0.01,
		MaxAgentIntents: 1,
		AgentObjective:  "Generate conservative, risk-aware trade intents.",
	}
}

func NewSystem(cfg Config) (*System, error) {
	var broker domain.Broker
	var watchlistSyncBroker *alpaca.Broker
	brokerLabel := strings.ToLower(strings.TrimSpace(cfg.Broker))
	isAlpacaBroker := brokerLabel == "alpaca"
	credentialSource := ""
	watchlistPullErr := error(nil)
	watchlist := normalizeSymbols(cfg.Watchlist)
	keyringCfg := credentials.KeyringConfig{
		Enabled: cfg.UseKeyring,
		Save:    cfg.SaveToKeyring,
		Service: cfg.KeyringService,
		User:    cfg.KeyringUser,
	}

	needsAlpacaCreds := isAlpacaBroker
	alpacaAPIKey := ""
	alpacaAPISecret := ""
	if needsAlpacaCreds {
		key, secret, source, err := credentials.ResolveAlpacaCredentials(
			cfg.AlpacaAPIKey,
			cfg.AlpacaAPISecret,
			keyringCfg,
		)
		if err != nil {
			return nil, err
		} else {
			alpacaAPIKey = key
			alpacaAPISecret = secret
			credentialSource = source
		}
	}
	switch brokerLabel {
	case "paper":
		broker = paper.New(100000)
	case "alpaca":
		alpacaBroker := newAlpacaBroker(cfg, alpacaAPIKey, alpacaAPISecret)
		broker = alpacaBroker
		watchlistSyncBroker = alpacaBroker
	default:
		return nil, fmt.Errorf("unsupported broker: %s", cfg.Broker)
	}
	if watchlistSyncBroker != nil {
		remote, err := watchlistSyncBroker.GetWatchlistSymbols(defaultAlpacaWatchlistName)
		if err != nil {
			watchlistPullErr = err
		} else {
			// In alpaca mode the remote watchlist is the source of truth.
			watchlist = remote
		}
	}

	allow := make(map[string]struct{}, len(cfg.AllowSymbols))
	for _, s := range mergeSymbols(cfg.AllowSymbols, watchlist) {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s != "" {
			allow[s] = struct{}{}
		}
	}

	risk := engine.NewRiskGate(engine.Policy{
		MaxNotionalPerTrade: cfg.MaxNotionalPerTrade,
		MaxNotionalPerDay:   cfg.MaxNotionalPerDay,
		AllowMarketOrders:   true,
		AllowSymbols:        allow,
	})

	e := engine.New(broker, risk)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.Sync(ctx); err != nil {
		return nil, err
	}
	if err := e.StartTradeUpdateLoop(context.Background()); err != nil {
		// Streaming is optional in this scaffold. Sync still works without it.
	}
	if isAlpacaBroker {
		env := effectiveAlpacaEnv(cfg)
		endpoint := strings.TrimSpace(cfg.AlpacaBaseURL)
		if endpoint == "" {
			endpoint = alpaca.BaseURLForEnv(env)
		}
		e.AddEvent(
			"alpaca_config",
			fmt.Sprintf(
				"env=%s endpoint=%s feed=%s credentials=%s",
				env,
				endpoint,
				strings.ToLower(strings.TrimSpace(cfg.AlpacaFeed)),
				credentialSource,
			),
		)
	}
	if watchlistPullErr != nil {
		e.AddEvent("watchlist_sync_error", fmt.Sprintf("pull: %v", watchlistPullErr))
	}
	system := &System{
		Engine:             e,
		Watchlist:          watchlist,
		DefaultBrokerLabel: brokerLabel,
	}
	if watchlistSyncBroker != nil {
		system.PullWatchlist = func() ([]string, error) {
			return watchlistSyncBroker.GetWatchlistSymbols(defaultAlpacaWatchlistName)
		}
		system.SyncWatchlist = func(symbols []string) error {
			return watchlistSyncBroker.UpsertWatchlistSymbols(defaultAlpacaWatchlistName, symbols)
		}
	}

	mode := normalizeMode(cfg.Mode)
	if mode != domain.ModeManual {
		agent := heuristic.New(broker, cfg.AgentMovePct, cfg.AgentOrderQty)
		runner := autonomy.NewRunner(
			e,
			agent,
			mode,
			watchlist,
			cfg.AgentInterval,
			cfg.MaxAgentIntents,
			cfg.AgentDryRun,
			cfg.AgentObjective,
		)
		system.Runner = runner
		e.AddEvent("agent_mode", fmt.Sprintf("mode=%s watchlist=%s", mode, strings.Join(watchlist, ",")))
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

func normalizeSymbols(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
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

func mergeSymbols(lists ...[]string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, list := range lists {
		for _, s := range list {
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
	}
	return out
}
