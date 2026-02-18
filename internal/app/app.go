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
	Engine *engine.Engine
	Runner *autonomy.Runner
}

func DefaultConfig() Config {
	return Config{
		Broker:              "paper",
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
	credentialSource := ""
	switch strings.ToLower(strings.TrimSpace(cfg.Broker)) {
	case "paper":
		broker = paper.New(100000)
	case "alpaca-paper":
		apiKey, secret, source, err := credentials.ResolveAlpacaCredentials(
			cfg.AlpacaAPIKey,
			cfg.AlpacaAPISecret,
			credentials.KeyringConfig{
				Enabled: cfg.UseKeyring,
				Save:    cfg.SaveToKeyring,
				Service: cfg.KeyringService,
				User:    cfg.KeyringUser,
			},
		)
		if err != nil {
			return nil, err
		}
		broker = alpaca.NewPaper(apiKey, secret, cfg.AlpacaDataURL, cfg.AlpacaFeed)
		credentialSource = source
	default:
		return nil, fmt.Errorf("unsupported broker: %s", cfg.Broker)
	}

	allow := make(map[string]struct{}, len(cfg.AllowSymbols))
	for _, s := range cfg.AllowSymbols {
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
	if strings.EqualFold(strings.TrimSpace(cfg.Broker), "alpaca-paper") {
		e.AddEvent("alpaca_config", fmt.Sprintf("feed=%s credentials=%s", strings.ToLower(strings.TrimSpace(cfg.AlpacaFeed)), credentialSource))
	}

	mode := normalizeMode(cfg.Mode)
	system := &System{Engine: e}
	if mode != domain.ModeManual {
		agent := heuristic.New(broker, cfg.AgentMovePct, cfg.AgentOrderQty)
		runner := autonomy.NewRunner(
			e,
			agent,
			mode,
			normalizeSymbols(cfg.Watchlist),
			cfg.AgentInterval,
			cfg.MaxAgentIntents,
			cfg.AgentDryRun,
			cfg.AgentObjective,
		)
		system.Runner = runner
		e.AddEvent("agent_mode", fmt.Sprintf("mode=%s watchlist=%s", mode, strings.Join(normalizeSymbols(cfg.Watchlist), ",")))
	}
	return system, nil
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
