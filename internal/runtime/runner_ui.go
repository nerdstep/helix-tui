package runtime

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/app"
	"helix-tui/internal/domain"
	"helix-tui/internal/storage"
	"helix-tui/internal/strategy"
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
		system.Watchlist = append([]string{}, nextWatchlist...)
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
		system.Watchlist = append([]string{}, nextWatchlist...)
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
		WithWatchlistChangeHandler(onWatchlistChanged).
		WithTradingDayChecker(system.TradingDayChecker)

	if system.StrategyRunner != nil {
		model = model.WithStrategyRunHandler(func() error {
			if queued := system.StrategyRunner.TriggerNow("tui_command"); !queued {
				return fmt.Errorf("strategy run request already pending")
			}
			return nil
		})
	}

	if store != nil {
		strategyRepo := store.Strategy()
		if strategyRepo != nil {
			model = model.
				WithStrategyApproveHandler(func(planID uint) error {
					if err := strategyRepo.SetPlanStatus(planID, storage.StrategyPlanStatusActive); err != nil {
						return err
					}
					system.Engine.AddEvent("strategy_plan_approved", fmt.Sprintf("id=%d status=active source=tui", planID))
					return nil
				}).
				WithStrategyRejectHandler(func(planID uint) error {
					if err := strategyRepo.SetPlanStatus(planID, storage.StrategyPlanStatusSuperseded); err != nil {
						return err
					}
					system.Engine.AddEvent("strategy_plan_rejected", fmt.Sprintf("id=%d status=superseded source=tui", planID))
					return nil
				}).
				WithStrategyArchiveHandler(func(planID uint) error {
					if err := strategyRepo.SetPlanStatus(planID, storage.StrategyPlanStatusArchived); err != nil {
						return err
					}
					system.Engine.AddEvent("strategy_plan_archived", fmt.Sprintf("id=%d status=archived source=tui", planID))
					return nil
				}).
				WithStrategyChatCreateHandler(func(title string) (uint, error) {
					thread, err := strategyRepo.CreateChatThread(title)
					if err != nil {
						return 0, err
					}
					system.Engine.AddEvent("strategy_chat_thread_created", fmt.Sprintf("thread_id=%d title=%q source=tui", thread.ID, thread.Title))
					return thread.ID, nil
				}).
				WithStrategyChatSendHandler(func(threadID uint, message string) error {
					if _, err := strategyRepo.AppendChatMessage(threadID, "user", message, ""); err != nil {
						return err
					}
					system.Engine.AddEvent("strategy_chat_message", fmt.Sprintf("thread_id=%d role=user chars=%d source=tui", threadID, len(message)))

					if system.StrategyCopilot == nil {
						return fmt.Errorf("strategy copilot is not configured")
					}

					activePlan, err := strategyRepo.GetActivePlan()
					if err != nil {
						return fmt.Errorf("load active strategy plan for chat: %w", err)
					}
					if activePlan == nil {
						activePlan, err = strategyRepo.GetLatestPlan()
						if err != nil {
							return fmt.Errorf("load latest strategy plan for chat: %w", err)
						}
					}

					msgRecords, err := strategyRepo.ListChatMessages(threadID, 80)
					if err != nil {
						return fmt.Errorf("load strategy chat messages: %w", err)
					}
					events := system.Engine.Snapshot().Events
					if store != nil && store.Events() != nil {
						if persistedEvents, listErr := store.Events().ListRecent(80); listErr == nil {
							events = persistedEvents
						}
					}
					callCtx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
					defer cancel()
					snapshot := system.Engine.Snapshot()
					replyStart := time.Now().UTC()
					reply, err := system.StrategyCopilot.Reply(callCtx, strategy.ChatInput{
						GeneratedAt:  time.Now().UTC(),
						Watchlist:    symbols.Normalize(system.Watchlist),
						Snapshot:     snapshot,
						Quotes:       collectChatQuotes(callCtx, system.Engine, system.Watchlist),
						CurrentPlan:  toStrategyCurrentPlan(activePlan),
						Messages:     toStrategyChatMessages(msgRecords),
						RecentEvents: events,
					})
					if err != nil {
						return err
					}
					if _, err := strategyRepo.AppendChatMessage(threadID, "assistant", reply.Content, reply.Model); err != nil {
						return err
					}
					system.Engine.AddEvent(
						"strategy_chat_message",
						fmt.Sprintf(
							"thread_id=%d role=assistant chars=%d source=llm model=%s latency_ms=%d",
							threadID,
							len(reply.Content),
							reply.Model,
							time.Since(replyStart).Milliseconds(),
						),
					)

					queued := false
					if system.StrategyRunner != nil {
						queued = system.StrategyRunner.TriggerNow("strategy_chat")
					}
					if queued {
						system.Engine.AddEvent("strategy_cycle_requested", "reason=strategy_chat")
					}
					return nil
				})
		}

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
		model = model.WithStrategyLoader(func(threadID uint) (tui.StrategySnapshot, error) {
			return loadStrategySnapshotForThread(strategyRepo, threadID)
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

func collectChatQuotes(ctx context.Context, eng interface {
	GetQuote(context.Context, string) (domain.Quote, error)
}, watchlist []string) []domain.Quote {
	symbolsList := symbols.Normalize(watchlist)
	out := make([]domain.Quote, 0, len(symbolsList))
	for _, symbol := range symbolsList {
		quote, err := eng.GetQuote(ctx, symbol)
		if err != nil {
			continue
		}
		out = append(out, quote)
	}
	return out
}

func toStrategyCurrentPlan(bundle *storage.StrategyPlanWithRecommendations) *strategy.CurrentPlan {
	if bundle == nil {
		return nil
	}
	recs := make([]strategy.Recommendation, 0, len(bundle.Recommendations))
	for _, rec := range bundle.Recommendations {
		recs = append(recs, strategy.Recommendation{
			Symbol:       rec.Symbol,
			Bias:         rec.Bias,
			Confidence:   rec.Confidence,
			EntryMin:     rec.EntryMin,
			EntryMax:     rec.EntryMax,
			TargetPrice:  rec.TargetPrice,
			StopPrice:    rec.StopPrice,
			MaxNotional:  rec.MaxNotional,
			Thesis:       rec.Thesis,
			Invalidation: rec.Invalidation,
			Priority:     rec.Priority,
		})
	}
	return &strategy.CurrentPlan{
		ID:              bundle.Plan.ID,
		GeneratedAt:     bundle.Plan.GeneratedAt,
		Status:          string(bundle.Plan.Status),
		Summary:         bundle.Plan.Summary,
		Confidence:      bundle.Plan.Confidence,
		Recommendations: recs,
	}
}

func toStrategyChatMessages(in []storage.StrategyChatMessage) []strategy.ChatMessage {
	out := make([]strategy.ChatMessage, 0, len(in))
	for _, msg := range in {
		out = append(out, strategy.ChatMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			CreatedAt: msg.CreatedAt,
		})
	}
	return out
}
