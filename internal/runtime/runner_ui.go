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
				WithStrategyProposalApplyHandler(func(proposalID uint) error {
					proposal, err := strategyRepo.GetProposal(proposalID)
					if err != nil {
						return err
					}
					if proposal == nil {
						return fmt.Errorf("strategy proposal %d not found", proposalID)
					}
					if proposal.Status != storage.StrategyProposalStatusPending {
						return fmt.Errorf("strategy proposal %d is already %s", proposalID, proposal.Status)
					}
					switch proposal.Kind {
					case storage.StrategyProposalKindWatchlist:
						nextWatchlist := applyWatchlistProposal(system.Watchlist, proposal.AddSymbols, proposal.RemoveSymbols)
						if len(nextWatchlist) == 0 {
							return fmt.Errorf("strategy proposal %d would result in an empty watchlist", proposalID)
						}
						if err := onWatchlistChanged(nextWatchlist); err != nil {
							return fmt.Errorf("apply watchlist proposal: %w", err)
						}
					case storage.StrategyProposalKindSteering:
						state, err := strategyRepo.UpsertSteeringState(storage.StrategySteeringStateInput{
							Source:              fmt.Sprintf("proposal:%d", proposal.ID),
							RiskProfile:         proposal.RiskProfile,
							MinConfidence:       proposal.MinConfidence,
							MaxPositionNotional: proposal.MaxPositionNotional,
							Horizon:             proposal.Horizon,
							Objective:           proposal.Objective,
							PreferredSymbols:    proposal.PreferredSymbols,
							ExcludedSymbols:     proposal.ExcludedSymbols,
						})
						if err != nil {
							return fmt.Errorf("apply steering proposal: %w", err)
						}
						system.Engine.AddEvent(
							"strategy_steering_updated",
							fmt.Sprintf(
								"source=proposal id=%d version=%d hash=%s",
								proposal.ID,
								state.Version,
								state.Hash,
							),
						)
					default:
						return fmt.Errorf("unsupported proposal kind %q", proposal.Kind)
					}

					if err := strategyRepo.SetProposalStatus(proposal.ID, storage.StrategyProposalStatusApplied); err != nil {
						return err
					}
					system.Engine.AddEvent(
						"strategy_proposal_applied",
						fmt.Sprintf("id=%d kind=%s source=tui", proposal.ID, proposal.Kind),
					)
					if system.StrategyRunner != nil {
						if queued := system.StrategyRunner.TriggerNow("strategy_proposal_apply"); queued {
							system.Engine.AddEvent("strategy_cycle_requested", "reason=strategy_proposal_apply")
						}
					}
					return nil
				}).
				WithStrategyProposalRejectHandler(func(proposalID uint) error {
					proposal, err := strategyRepo.GetProposal(proposalID)
					if err != nil {
						return err
					}
					if proposal == nil {
						return fmt.Errorf("strategy proposal %d not found", proposalID)
					}
					if proposal.Status != storage.StrategyProposalStatusPending {
						return fmt.Errorf("strategy proposal %d is already %s", proposalID, proposal.Status)
					}
					if err := strategyRepo.SetProposalStatus(proposal.ID, storage.StrategyProposalStatusRejected); err != nil {
						return err
					}
					system.Engine.AddEvent(
						"strategy_proposal_rejected",
						fmt.Sprintf("id=%d kind=%s source=tui", proposal.ID, proposal.Kind),
					)
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
					createdProposals, err := persistStrategyCopilotProposals(strategyRepo, threadID, reply.Proposals)
					if err != nil {
						return err
					}
					for _, proposal := range createdProposals {
						system.Engine.AddEvent(
							"strategy_proposal_created",
							fmt.Sprintf(
								"id=%d kind=%s status=%s source=%s hash=%s",
								proposal.ID,
								proposal.Kind,
								proposal.Status,
								proposal.Source,
								proposal.Hash,
							),
						)
					}
					system.Engine.AddEvent(
						"strategy_chat_message",
						fmt.Sprintf(
							"thread_id=%d role=assistant chars=%d source=llm model=%s latency_ms=%d proposals=%d",
							threadID,
							len(reply.Content),
							reply.Model,
							time.Since(replyStart).Milliseconds(),
							len(createdProposals),
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

func persistStrategyCopilotProposals(repo *storage.StrategyRepository, threadID uint, proposals []strategy.CopilotProposal) ([]storage.StrategyProposal, error) {
	if repo == nil || threadID == 0 || len(proposals) == 0 {
		return nil, nil
	}
	out := make([]storage.StrategyProposal, 0, len(proposals))
	for _, proposal := range proposals {
		input, ok := toStrategyProposalInput(threadID, proposal)
		if !ok {
			continue
		}
		created, err := repo.CreateProposal(input)
		if err != nil {
			return nil, fmt.Errorf("create strategy proposal: %w", err)
		}
		out = append(out, created)
	}
	return out, nil
}

func toStrategyProposalInput(threadID uint, proposal strategy.CopilotProposal) (storage.StrategyProposalInput, bool) {
	switch proposal.Kind {
	case strategy.CopilotProposalKindWatchlist:
		addSymbols := symbols.Normalize(proposal.AddSymbols)
		removeSymbols := symbols.Normalize(proposal.RemoveSymbols)
		if len(addSymbols) == 0 && len(removeSymbols) == 0 {
			return storage.StrategyProposalInput{}, false
		}
		return storage.StrategyProposalInput{
			ThreadID:      threadID,
			Source:        "copilot",
			Kind:          storage.StrategyProposalKindWatchlist,
			Rationale:     proposal.Rationale,
			AddSymbols:    addSymbols,
			RemoveSymbols: removeSymbols,
		}, true
	case strategy.CopilotProposalKindSteering:
		preferredSymbols := symbols.Normalize(proposal.PreferredSymbols)
		excludedSymbols := symbols.Normalize(proposal.ExcludedSymbols)
		return storage.StrategyProposalInput{
			ThreadID:            threadID,
			Source:              "copilot",
			Kind:                storage.StrategyProposalKindSteering,
			Rationale:           proposal.Rationale,
			RiskProfile:         proposal.RiskProfile,
			MinConfidence:       proposal.MinConfidence,
			MaxPositionNotional: proposal.MaxPositionNotional,
			Horizon:             proposal.Horizon,
			Objective:           proposal.Objective,
			PreferredSymbols:    preferredSymbols,
			ExcludedSymbols:     excludedSymbols,
		}, true
	default:
		return storage.StrategyProposalInput{}, false
	}
}

func applyWatchlistProposal(current, addSymbols, removeSymbols []string) []string {
	current = symbols.Normalize(current)
	addSymbols = symbols.Normalize(addSymbols)
	removeSymbols = symbols.Normalize(removeSymbols)
	if len(removeSymbols) > 0 {
		removeSet := make(map[string]struct{}, len(removeSymbols))
		for _, symbol := range removeSymbols {
			removeSet[symbol] = struct{}{}
		}
		filtered := make([]string, 0, len(current))
		for _, symbol := range current {
			if _, remove := removeSet[symbol]; remove {
				continue
			}
			filtered = append(filtered, symbol)
		}
		current = filtered
	}
	if len(addSymbols) > 0 {
		current = symbols.Merge(current, addSymbols)
	}
	return current
}
