package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"helix-tui/internal/agent/heuristic"
	"helix-tui/internal/agent/llm"
	"helix-tui/internal/autonomy"
	"helix-tui/internal/broker/alpaca"
	"helix-tui/internal/broker/paper"
	"helix-tui/internal/credentials"
	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
	"helix-tui/internal/symbols"
)

type brokerSpec struct {
	label               string
	isAlpaca            bool
	broker              domain.Broker
	quoteStreamer       domain.QuoteStreamer
	watchlistSyncBroker *alpaca.Broker
	credentialSource    string
}

func buildBroker(cfg Config) (brokerSpec, error) {
	spec := brokerSpec{
		label: strings.ToLower(strings.TrimSpace(cfg.Broker)),
	}

	switch spec.label {
	case "paper":
		spec.broker = paper.New(100000)
		return spec, nil
	case "alpaca":
		spec.isAlpaca = true
	default:
		return brokerSpec{}, fmt.Errorf("unsupported broker: %s", cfg.Broker)
	}

	keyringCfg := credentials.KeyringConfig{
		Enabled: cfg.UseKeyring,
		Save:    cfg.SaveToKeyring,
		Service: cfg.KeyringService,
		User:    cfg.KeyringUser,
	}
	apiKey, apiSecret, source, err := credentials.ResolveAlpacaCredentials(
		cfg.AlpacaAPIKey,
		cfg.AlpacaAPISecret,
		keyringCfg,
	)
	if err != nil {
		return brokerSpec{}, err
	}

	alpacaBroker := newAlpacaBroker(cfg, apiKey, apiSecret)
	spec.broker = alpacaBroker
	spec.quoteStreamer = alpacaBroker
	spec.watchlistSyncBroker = alpacaBroker
	spec.credentialSource = source
	return spec, nil
}

func resolveWatchlist(cfg Config, watchlistSyncBroker *alpaca.Broker) ([]string, error) {
	watchlist := symbols.Normalize(cfg.Watchlist)
	if watchlistSyncBroker == nil {
		return watchlist, nil
	}

	// In alpaca mode the remote watchlist is the source of truth.
	remote, err := watchlistSyncBroker.GetWatchlistSymbols(defaultAlpacaWatchlistName)
	if err != nil {
		return watchlist, err
	}
	return remote, nil
}

func buildAllowSymbols(watchlist []string) map[string]struct{} {
	normalized := symbols.Normalize(watchlist)
	allow := make(map[string]struct{}, len(normalized))
	for _, symbol := range normalized {
		allow[symbol] = struct{}{}
	}
	return allow
}

func buildEngine(cfg Config, broker domain.Broker, allowSymbols map[string]struct{}) (*engine.Engine, error) {
	risk := engine.NewRiskGate(engine.Policy{
		MaxNotionalPerTrade: cfg.MaxNotionalPerTrade,
		MaxNotionalPerDay:   cfg.MaxNotionalPerDay,
		AllowMarketOrders:   true,
		AllowSymbols:        allowSymbols,
	})

	e := engine.New(broker, risk)
	e.SetComplianceGate(engine.NewComplianceGate(engine.CompliancePolicy{
		Enabled:         cfg.ComplianceEnabled,
		AccountType:     cfg.ComplianceAccountType,
		AvoidPDT:        cfg.ComplianceAvoidPDT,
		MaxDayTrades5D:  cfg.ComplianceMaxDayTrades5D,
		MinEquityForPDT: cfg.ComplianceMinEquityForPDT,
		AvoidGoodFaith:  cfg.ComplianceAvoidGoodFaith,
	}))
	syncTimeout := cfg.SyncTimeout
	if syncTimeout <= 0 {
		syncTimeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	if err := e.Sync(ctx); err != nil {
		return nil, err
	}
	if cfg.ComplianceEnabled {
		e.AddEvent(
			"compliance_config",
			fmt.Sprintf(
				"enabled=true account_type=%s avoid_pdt=%t max_day_trades_5d=%d min_equity_for_pdt=%.2f avoid_gfv=%t",
				strings.ToLower(strings.TrimSpace(cfg.ComplianceAccountType)),
				cfg.ComplianceAvoidPDT,
				cfg.ComplianceMaxDayTrades5D,
				cfg.ComplianceMinEquityForPDT,
				cfg.ComplianceAvoidGoodFaith,
			),
		)
	}
	if err := e.StartTradeUpdateLoop(context.Background()); err != nil {
		// Streaming is optional in this scaffold. Sync still works without it.
	}
	return e, nil
}

func addAlpacaConfigEvent(e *engine.Engine, cfg Config, credentialSource string) {
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

func buildWatchlistHandlers(watchlistSyncBroker *alpaca.Broker) (func() ([]string, error), func([]string) error) {
	if watchlistSyncBroker == nil {
		return nil, nil
	}
	pull := func() ([]string, error) {
		return watchlistSyncBroker.GetWatchlistSymbols(defaultAlpacaWatchlistName)
	}
	sync := func(next []string) error {
		return watchlistSyncBroker.UpsertWatchlistSymbols(defaultAlpacaWatchlistName, symbols.Normalize(next))
	}
	return pull, sync
}

func buildRunner(cfg Config, broker domain.Broker, e *engine.Engine, watchlist []string) (*autonomy.Runner, domain.Mode, string, error) {
	mode := normalizeMode(cfg.Mode)
	if mode == domain.ModeManual {
		return nil, mode, "", nil
	}

	agent, agentType, err := buildAgent(cfg, broker)
	if err != nil {
		return nil, mode, "", err
	}
	runner := autonomy.NewRunner(
		e,
		agent,
		mode,
		watchlist,
		cfg.AgentInterval,
		cfg.SyncTimeout,
		cfg.OrderTimeout,
		cfg.MaxAgentIntents,
		cfg.AgentMinGainPct,
		cfg.AgentDryRun,
		contextLogModeForAgent(cfg, agentType),
	)
	return runner, mode, agentType, nil
}

func buildAgent(cfg Config, broker domain.Broker) (domain.Agent, string, error) {
	agentType := normalizeAgentType(cfg.AgentType)
	switch agentType {
	case "heuristic":
		return heuristic.New(broker, cfg.AgentMovePct, cfg.AgentOrderQty), agentType, nil
	case "llm":
		keyringCfg := credentials.KeyringConfig{
			Enabled: cfg.UseKeyring,
			Save:    cfg.SaveToKeyring,
			Service: cfg.KeyringService,
			User:    cfg.KeyringUser,
		}
		llmKey, _, err := credentials.ResolveOpenAICredentials(cfg.LLMAPIKey, keyringCfg)
		if err != nil {
			return nil, "", err
		}
		agent, err := llm.New(broker, llm.Config{
			APIKey:           llmKey,
			BaseURL:          cfg.LLMBaseURL,
			Model:            cfg.LLMModel,
			Timeout:          cfg.LLMTimeout,
			SystemPrompt:     cfg.LLMSystemPrompt,
			MaxTradeNotional: cfg.MaxNotionalPerTrade,
			MaxDayNotional:   cfg.MaxNotionalPerDay,
			MinGainPct:       cfg.AgentMinGainPct,
		})
		if err != nil {
			return nil, "", err
		}
		return agent, agentType, nil
	default:
		return nil, "", fmt.Errorf("unsupported agent type: %s", cfg.AgentType)
	}
}

func normalizeAgentType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "heuristic":
		return "heuristic"
	case "llm":
		return "llm"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func contextLogModeForAgent(cfg Config, agentType string) string {
	if agentType != "llm" {
		return "off"
	}
	return cfg.LLMContextLog
}
