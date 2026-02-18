package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/app"
	"helix-tui/internal/domain"
	"helix-tui/internal/tui"
)

func main() {
	cfg := app.DefaultConfig()

	var allowSymbols string
	var watchlist string
	var mode string
	var headless bool
	flag.StringVar(&cfg.Broker, "broker", cfg.Broker, "broker adapter: paper or alpaca-paper")
	flag.StringVar(&cfg.AlpacaAPIKey, "alpaca-key", os.Getenv("APCA_API_KEY_ID"), "alpaca API key")
	flag.StringVar(&cfg.AlpacaAPISecret, "alpaca-secret", os.Getenv("APCA_API_SECRET_KEY"), "alpaca API secret")
	flag.Float64Var(&cfg.MaxNotionalPerTrade, "max-trade", cfg.MaxNotionalPerTrade, "max notional per trade")
	flag.Float64Var(&cfg.MaxNotionalPerDay, "max-day", cfg.MaxNotionalPerDay, "max notional per day")
	flag.StringVar(&allowSymbols, "allow", strings.Join(cfg.AllowSymbols, ","), "comma-separated symbol allowlist")
	flag.StringVar(&mode, "mode", string(cfg.Mode), "runtime mode: manual | assist | auto")
	flag.StringVar(&watchlist, "watchlist", strings.Join(cfg.Watchlist, ","), "comma-separated symbols used by the agent")
	flag.DurationVar(&cfg.AgentInterval, "agent-interval", cfg.AgentInterval, "agent cycle interval")
	flag.Float64Var(&cfg.AgentOrderQty, "agent-qty", cfg.AgentOrderQty, "agent order quantity per intent")
	flag.Float64Var(&cfg.AgentMovePct, "agent-move-pct", cfg.AgentMovePct, "agent trigger threshold (0.01 = 1%)")
	flag.IntVar(&cfg.MaxAgentIntents, "agent-max-intents", cfg.MaxAgentIntents, "max intents executed per cycle")
	flag.BoolVar(&cfg.AgentDryRun, "dry-run", false, "run full autonomous flow without submitting orders")
	flag.BoolVar(&headless, "headless", false, "run without TUI; useful for autonomous mode")
	flag.Parse()

	cfg.Mode = domain.Mode(strings.ToLower(strings.TrimSpace(mode)))
	if allowSymbols != "" {
		cfg.AllowSymbols = splitSymbols(allowSymbols)
	}
	if watchlist != "" {
		cfg.Watchlist = splitSymbols(watchlist)
	}

	system, err := app.NewSystem(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create system: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if system.Runner != nil {
		go func() {
			if err := system.Runner.Run(ctx); err != nil && err != context.Canceled {
				system.Engine.AddEvent("agent_runner_error", err.Error())
			}
		}()
	}

	if headless {
		runHeadless(ctx, system.Engine)
		return
	}

	program := tea.NewProgram(tui.New(system.Engine), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
		os.Exit(1)
	}
}

func splitSymbols(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, s := range parts {
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
