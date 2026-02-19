package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/app"
	"helix-tui/internal/configfile"
	"helix-tui/internal/domain"
	"helix-tui/internal/tui"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
	cfg := app.DefaultConfig()
	configPath, configExplicit, err := parseConfigPath(args)
	if err != nil {
		return fmt.Errorf("invalid config flag: %w", err)
	}
	if err := configfile.Load(configPath, &cfg, configExplicit); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	applyEnvOverrides(&cfg)

	var allowSymbols string
	var watchlistArg string
	var mode string
	var headless bool
	fs := flag.NewFlagSet("helix", flag.ContinueOnError)
	if stderr == nil {
		stderr = io.Discard
	}
	fs.SetOutput(stderr)
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
	fs.DurationVar(&cfg.AgentInterval, "agent-interval", cfg.AgentInterval, "agent cycle interval")
	fs.Float64Var(&cfg.AgentOrderQty, "agent-qty", cfg.AgentOrderQty, "agent order quantity per intent")
	fs.Float64Var(&cfg.AgentMovePct, "agent-move-pct", cfg.AgentMovePct, "agent trigger threshold (0.01 = 1%)")
	fs.IntVar(&cfg.MaxAgentIntents, "agent-max-intents", cfg.MaxAgentIntents, "max intents executed per cycle")
	fs.BoolVar(&cfg.AgentDryRun, "dry-run", cfg.AgentDryRun, "run full autonomous flow without submitting orders")
	fs.BoolVar(&headless, "headless", false, "run without TUI; useful for autonomous mode")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg.Mode = domain.Mode(strings.ToLower(strings.TrimSpace(mode)))
	if allowSymbols != "" {
		cfg.AllowSymbols = splitSymbols(allowSymbols)
	}
	if watchlistArg != "" {
		cfg.Watchlist = splitSymbols(watchlistArg)
	}

	system, err := app.NewSystem(cfg)
	if err != nil {
		return fmt.Errorf("failed to create system: %w", err)
	}

	if system.Runner != nil {
		go func() {
			if err := system.Runner.Run(ctx); err != nil && err != context.Canceled {
				system.Engine.AddEvent("agent_runner_error", err.Error())
			}
		}()
	}

	if headless {
		runHeadless(ctx, system.Engine)
		return nil
	}

	onWatchlistChanged := func(symbols []string) error {
		symbols = mergeSymbols(symbols)
		if system.SyncWatchlist != nil {
			if err := system.SyncWatchlist(symbols); err != nil {
				return err
			}
		}
		if system.Runner != nil {
			system.Runner.SetWatchlist(symbols)
		}
		return nil
	}
	onWatchlistSync := func(symbols []string) ([]string, error) {
		symbols = mergeSymbols(symbols)
		if system.PullWatchlist != nil {
			remote, err := system.PullWatchlist()
			if err != nil {
				return nil, err
			}
			symbols = mergeSymbols(symbols, remote)
		}
		if system.Runner != nil {
			system.Runner.SetWatchlist(symbols)
		}
		return symbols, nil
	}
	model := tui.New(system.Engine, system.Watchlist...).
		WithWatchlistChangeHandler(onWatchlistChanged)
	if system.PullWatchlist != nil {
		model = model.WithWatchlistSyncHandler(onWatchlistSync)
	}
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("runtime error: %w", err)
	}
	return nil
}

func parseConfigPath(args []string) (string, bool, error) {
	path := configfile.DefaultPath
	explicit := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-config" || arg == "--config":
			if i+1 >= len(args) {
				return "", false, fmt.Errorf("%s requires a path value", arg)
			}
			path = strings.TrimSpace(args[i+1])
			explicit = true
			i++
		case strings.HasPrefix(arg, "-config="):
			path = strings.TrimSpace(strings.TrimPrefix(arg, "-config="))
			explicit = true
		case strings.HasPrefix(arg, "--config="):
			path = strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
			explicit = true
		}
	}

	if path == "" {
		return "", false, fmt.Errorf("config path cannot be empty")
	}
	return path, explicit, nil
}

func applyEnvOverrides(cfg *app.Config) {
	if v := strings.TrimSpace(os.Getenv("APCA_API_KEY_ID")); v != "" {
		cfg.AlpacaAPIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("APCA_API_SECRET_KEY")); v != "" {
		cfg.AlpacaAPISecret = v
	}
	if v := strings.TrimSpace(os.Getenv("APCA_API_DATA_URL")); v != "" {
		cfg.AlpacaDataURL = v
	}
}

func splitSymbols(raw string) []string {
	return mergeSymbols(strings.Split(raw, ","))
}

func mergeSymbols(lists ...[]string) []string {
	total := 0
	for _, list := range lists {
		total += len(list)
	}
	out := make([]string, 0, total)
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

func runHeadless(ctx context.Context, eng interface{ Snapshot() domain.Snapshot }) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	fmt.Println("running in headless mode; press Ctrl+C to stop")
	for {
		select {
		case <-ctx.Done():
			fmt.Println("stopping headless mode")
			return
		case <-ticker.C:
			s := eng.Snapshot()
			fmt.Printf(
				"%s equity=%.2f cash=%.2f positions=%d open_orders=%d events=%d\n",
				time.Now().Format(time.RFC3339),
				s.Account.Equity,
				s.Account.Cash,
				len(s.Positions),
				len(s.Orders),
				len(s.Events),
			)
		}
	}
}
