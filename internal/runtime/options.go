package runtime

import (
	"flag"
	"io"
	"strings"

	"helix-tui/internal/app"
	"helix-tui/internal/domain"
)

type runOptions struct {
	cfg      app.Config
	headless bool
}

func parseRunOptions(args []string, cfg app.Config, configPath string, stderr io.Writer) (runOptions, error) {
	var allowSymbols string
	var watchlistArg string
	var mode string
	headless := false

	fs := newFlagSet(stderr)
	fs.StringVar(&configPath, "config", configPath, "path to TOML config file")
	fs.StringVar(&cfg.Broker, "broker", cfg.Broker, "broker adapter: paper or alpaca")
	fs.StringVar(&cfg.AlpacaEnv, "alpaca-env", cfg.AlpacaEnv, "alpaca trading environment: paper|live")
	fs.StringVar(&cfg.AlpacaBaseURL, "alpaca-base-url", cfg.AlpacaBaseURL, "alpaca trading API base URL override (optional)")
	fs.StringVar(&cfg.AlpacaAPIKey, "alpaca-key", cfg.AlpacaAPIKey, "alpaca API key")
	fs.StringVar(&cfg.AlpacaAPISecret, "alpaca-secret", cfg.AlpacaAPISecret, "alpaca API secret")
	fs.StringVar(&cfg.AlpacaDataURL, "alpaca-data-url", cfg.AlpacaDataURL, "alpaca market data API base URL")
	fs.StringVar(&cfg.AlpacaFeed, "alpaca-feed", cfg.AlpacaFeed, "alpaca stock feed for latest quotes (iex|sip|delayed_sip|boats|overnight)")
	fs.BoolVar(&cfg.UseKeyring, "use-keyring", cfg.UseKeyring, "load Alpaca credentials from OS keyring when flags/env are missing")
	fs.BoolVar(&cfg.SaveToKeyring, "save-keyring", cfg.SaveToKeyring, "save provided Alpaca credentials into OS keyring")
	fs.StringVar(&cfg.KeyringService, "keyring-service", cfg.KeyringService, "OS keyring service name")
	fs.StringVar(&cfg.KeyringUser, "keyring-user", cfg.KeyringUser, "OS keyring account namespace")
	fs.Float64Var(&cfg.MaxNotionalPerTrade, "max-trade", cfg.MaxNotionalPerTrade, "max notional per trade")
	fs.Float64Var(&cfg.MaxNotionalPerDay, "max-day", cfg.MaxNotionalPerDay, "max notional per day")
	fs.StringVar(&allowSymbols, "allow", strings.Join(cfg.AllowSymbols, ","), "comma-separated symbol allowlist")
	fs.StringVar(&mode, "mode", string(cfg.Mode), "runtime mode: manual | assist | auto")
	fs.StringVar(&watchlistArg, "watchlist", strings.Join(cfg.Watchlist, ","), "comma-separated symbols used by the agent")
	fs.StringVar(&cfg.AgentType, "agent-type", cfg.AgentType, "agent implementation: heuristic | llm")
	fs.DurationVar(&cfg.AgentInterval, "agent-interval", cfg.AgentInterval, "agent cycle interval")
	fs.Float64Var(&cfg.AgentOrderQty, "agent-qty", cfg.AgentOrderQty, "agent order quantity per intent")
	fs.Float64Var(&cfg.AgentMovePct, "agent-move-pct", cfg.AgentMovePct, "agent trigger threshold (0.01 = 1%)")
	fs.IntVar(&cfg.MaxAgentIntents, "agent-max-intents", cfg.MaxAgentIntents, "max intents executed per cycle")
	fs.BoolVar(&cfg.AgentDryRun, "dry-run", cfg.AgentDryRun, "run full autonomous flow without submitting orders")
	fs.StringVar(&cfg.LLMAPIKey, "llm-key", cfg.LLMAPIKey, "LLM API key (used when -agent-type=llm)")
	fs.StringVar(&cfg.LLMBaseURL, "llm-base-url", cfg.LLMBaseURL, "LLM API base URL (OpenAI-compatible)")
	fs.StringVar(&cfg.LLMModel, "llm-model", cfg.LLMModel, "LLM model name")
	fs.DurationVar(&cfg.LLMTimeout, "llm-timeout", cfg.LLMTimeout, "LLM request timeout")
	fs.StringVar(&cfg.LLMSystemPrompt, "llm-system-prompt", cfg.LLMSystemPrompt, "override system prompt used by the LLM agent")
	fs.BoolVar(&headless, "headless", false, "run without TUI; useful for autonomous mode")
	if err := fs.Parse(args); err != nil {
		return runOptions{}, err
	}

	cfg.Mode = domain.Mode(strings.ToLower(strings.TrimSpace(mode)))
	cfg.AgentType = strings.ToLower(strings.TrimSpace(cfg.AgentType))
	cfg.LLMBaseURL = strings.TrimSpace(cfg.LLMBaseURL)
	cfg.LLMModel = strings.TrimSpace(cfg.LLMModel)
	cfg.LLMAPIKey = strings.TrimSpace(cfg.LLMAPIKey)
	cfg.LLMSystemPrompt = strings.TrimSpace(cfg.LLMSystemPrompt)
	if allowSymbols != "" {
		cfg.AllowSymbols = SplitSymbols(allowSymbols)
	}
	if watchlistArg != "" {
		cfg.Watchlist = SplitSymbols(watchlistArg)
	}

	return runOptions{
		cfg:      cfg,
		headless: headless,
	}, nil
}

func newFlagSet(stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("helix", flag.ContinueOnError)
	if stderr == nil {
		stderr = io.Discard
	}
	fs.SetOutput(stderr)
	return fs
}
