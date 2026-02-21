package runtime

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/app"
	"helix-tui/internal/domain"
	"helix-tui/internal/storage"
	"helix-tui/internal/symbols"
	"helix-tui/internal/tui"
)

func createSystem(cfg app.Config) (*app.System, error) {
	system, err := app.NewSystem(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create system: %w", err)
	}
	return system, nil
}

func startRunner(ctx context.Context, system *app.System) {
	if system.Runner == nil {
	} else {
		go func() {
			if err := system.Runner.Run(ctx); err != nil && err != context.Canceled {
				system.Engine.AddEvent("agent_runner_error", err.Error())
			}
		}()
	}
	if system.StrategyRunner == nil {
		return
	}
	go func() {
		if err := system.StrategyRunner.Run(ctx); err != nil && err != context.Canceled {
			system.Engine.AddEvent("strategy_runner_error", err.Error())
		}
	}()
}

func runTUI(system *app.System, store *storage.Store, updateQuoteStream func([]string)) error {
	if updateQuoteStream == nil {
		updateQuoteStream = func([]string) {}
	}
	onWatchlistChanged := func(nextWatchlist []string) error {
		nextWatchlist = symbols.Normalize(nextWatchlist)
		if system.SyncWatchlist != nil {
			if err := system.SyncWatchlist(nextWatchlist); err != nil {
				return err
			}
		}
		if system.Runner != nil {
			system.Runner.SetWatchlist(nextWatchlist)
		}
		if system.StrategyRunner != nil {
			system.StrategyRunner.SetWatchlist(nextWatchlist)
		}
		updateQuoteStream(nextWatchlist)
		return nil
	}
	onWatchlistSync := func(nextWatchlist []string) ([]string, error) {
		nextWatchlist = symbols.Normalize(nextWatchlist)
		if system.PullWatchlist != nil {
			remote, err := system.PullWatchlist()
			if err != nil {
				return nil, err
			}
			nextWatchlist = symbols.Merge(nextWatchlist, remote)
		}
		if system.Runner != nil {
			system.Runner.SetWatchlist(nextWatchlist)
		}
		if system.StrategyRunner != nil {
			system.StrategyRunner.SetWatchlist(nextWatchlist)
		}
		updateQuoteStream(nextWatchlist)
		return nextWatchlist, nil
	}

	model := tui.New(system.Engine, system.Watchlist...).
		WithWatchlistChangeHandler(onWatchlistChanged)

	if store != nil {
		dbPoints, loadErr := store.EquityHistory().List()
		if loadErr != nil {
			system.Engine.AddEvent("equity_history_error", loadErr.Error())
		}

		equityHistory := make([]tui.EquityPoint, 0, len(dbPoints))
		for _, point := range dbPoints {
			equityHistory = append(equityHistory, tui.EquityPoint{
				Time:   point.Time,
				Equity: point.Equity,
			})
		}
		if len(equityHistory) > 0 {
			system.Engine.AddEvent("equity_history_loaded", fmt.Sprintf("points=%d db=%s", len(equityHistory), store.Path()))
		}

		appendPoint := func(point tui.EquityPoint) error {
			return store.EquityHistory().Append(storage.EquityPoint{
				Time:   point.Time,
				Equity: point.Equity,
			})
		}
		model = model.WithEquityHistory(equityHistory, appendPoint)
		model = model.WithStrategyLoader(func() (tui.StrategySnapshot, error) {
			return loadStrategySnapshot(store.Strategy())
		})
	}

	if system.PullWatchlist != nil {
		model = model.WithWatchlistSyncHandler(onWatchlistSync)
	}
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func RunHeadless(ctx context.Context, eng interface{ Snapshot() domain.Snapshot }) {
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
