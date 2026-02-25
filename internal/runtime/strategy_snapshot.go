package runtime

import (
	"fmt"

	"helix-tui/internal/storage"
	"helix-tui/internal/tui"
)

func loadStrategySnapshot(repo *storage.StrategyRepository) (tui.StrategySnapshot, error) {
	return loadStrategySnapshotForThread(repo, 0)
}

func loadStrategySnapshotForThread(repo *storage.StrategyRepository, threadID uint) (tui.StrategySnapshot, error) {
	if repo == nil {
		return tui.StrategySnapshot{}, fmt.Errorf("strategy repository is not configured")
	}
	active, err := repo.GetActivePlan()
	if err != nil {
		return tui.StrategySnapshot{}, err
	}
	recent, err := repo.ListRecentPlans(8)
	if err != nil {
		return tui.StrategySnapshot{}, err
	}
	out := tui.StrategySnapshot{
		Recent: make([]tui.StrategyPlanView, 0, len(recent)),
	}
	if active != nil {
		plan := toStrategyPlanView(active.Plan)
		plan.Recommendations = toStrategyRecommendationViews(active.Recommendations)
		out.Active = &plan
	}
	for _, plan := range recent {
		out.Recent = append(out.Recent, toStrategyPlanView(plan))
	}
	chat, err := loadStrategyChatSnapshot(repo, threadID)
	if err != nil {
		return tui.StrategySnapshot{}, err
	}
	steering, err := repo.GetSteeringState()
	if err != nil {
		return tui.StrategySnapshot{}, err
	}
	proposals, err := repo.ListProposals(20)
	if err != nil {
		return tui.StrategySnapshot{}, err
	}
	out.Chat = chat
	out.Steering = toStrategySteeringView(steering)
	out.Proposals = toStrategyProposalViews(proposals)
	return out, nil
}

func toStrategyPlanView(plan storage.StrategyPlan) tui.StrategyPlanView {
	return tui.StrategyPlanView{
		ID:            plan.ID,
		GeneratedAt:   plan.GeneratedAt,
		UpdatedAt:     plan.UpdatedAt,
		Status:        string(plan.Status),
		AnalystModel:  plan.AnalystModel,
		PromptVersion: plan.PromptVersion,
		Watchlist:     append([]string{}, plan.Watchlist...),
		Summary:       plan.Summary,
		Confidence:    plan.Confidence,
	}
}

func toStrategyRecommendationViews(recs []storage.StrategyRecommendation) []tui.StrategyRecommendationView {
	out := make([]tui.StrategyRecommendationView, 0, len(recs))
	for _, rec := range recs {
		out = append(out, tui.StrategyRecommendationView{
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
	return out
}

func loadStrategyChatSnapshot(repo *storage.StrategyRepository, requestedThreadID uint) (tui.StrategyChatView, error) {
	threads, err := repo.ListChatThreads(20)
	if err != nil {
		return tui.StrategyChatView{}, err
	}
	if len(threads) == 0 {
		seed, err := repo.EnsureChatThread("General")
		if err != nil {
			return tui.StrategyChatView{}, err
		}
		threads = []storage.StrategyChatThread{seed}
	}

	selected := resolveStrategyChatThread(threads, requestedThreadID)
	if selected.ID == 0 {
		return tui.StrategyChatView{}, nil
	}
	messages, err := repo.ListChatMessages(selected.ID, 160)
	if err != nil {
		return tui.StrategyChatView{}, err
	}

	return tui.StrategyChatView{
		ActiveThreadID: selected.ID,
		Threads:        toStrategyChatThreadViews(threads),
		Messages:       toStrategyChatMessageViews(messages),
	}, nil
}

func resolveStrategyChatThread(threads []storage.StrategyChatThread, requestedThreadID uint) storage.StrategyChatThread {
	if len(threads) == 0 {
		return storage.StrategyChatThread{}
	}
	if requestedThreadID != 0 {
		for _, thread := range threads {
			if thread.ID == requestedThreadID {
				return thread
			}
		}
	}
	return threads[0]
}

func toStrategyChatThreadViews(in []storage.StrategyChatThread) []tui.StrategyChatThreadView {
	out := make([]tui.StrategyChatThreadView, 0, len(in))
	for _, thread := range in {
		out = append(out, tui.StrategyChatThreadView{
			ID:            thread.ID,
			Title:         thread.Title,
			CreatedAt:     thread.CreatedAt,
			UpdatedAt:     thread.UpdatedAt,
			LastMessageAt: thread.LastMessageAt,
		})
	}
	return out
}

func toStrategyChatMessageViews(in []storage.StrategyChatMessage) []tui.StrategyChatMessageView {
	out := make([]tui.StrategyChatMessageView, 0, len(in))
	for _, msg := range in {
		out = append(out, tui.StrategyChatMessageView{
			ID:        msg.ID,
			ThreadID:  msg.ThreadID,
			Role:      msg.Role,
			Content:   msg.Content,
			Model:     msg.Model,
			CreatedAt: msg.CreatedAt,
		})
	}
	return out
}

func toStrategySteeringView(state *storage.StrategySteeringState) *tui.StrategySteeringView {
	if state == nil {
		return nil
	}
	return &tui.StrategySteeringView{
		Version:             state.Version,
		Source:              state.Source,
		RiskProfile:         state.RiskProfile,
		MinConfidence:       state.MinConfidence,
		MaxPositionNotional: state.MaxPositionNotional,
		Horizon:             state.Horizon,
		Objective:           state.Objective,
		PreferredSymbols:    append([]string{}, state.PreferredSymbols...),
		ExcludedSymbols:     append([]string{}, state.ExcludedSymbols...),
		Hash:                state.Hash,
		UpdatedAt:           state.UpdatedAt,
	}
}

func toStrategyProposalViews(in []storage.StrategyProposal) []tui.StrategyProposalView {
	out := make([]tui.StrategyProposalView, 0, len(in))
	for _, proposal := range in {
		out = append(out, tui.StrategyProposalView{
			ID:                  proposal.ID,
			ThreadID:            proposal.ThreadID,
			Source:              proposal.Source,
			Kind:                string(proposal.Kind),
			Status:              string(proposal.Status),
			Rationale:           proposal.Rationale,
			AddSymbols:          append([]string{}, proposal.AddSymbols...),
			RemoveSymbols:       append([]string{}, proposal.RemoveSymbols...),
			RiskProfile:         proposal.RiskProfile,
			MinConfidence:       proposal.MinConfidence,
			MaxPositionNotional: proposal.MaxPositionNotional,
			Horizon:             proposal.Horizon,
			Objective:           proposal.Objective,
			PreferredSymbols:    append([]string{}, proposal.PreferredSymbols...),
			ExcludedSymbols:     append([]string{}, proposal.ExcludedSymbols...),
			Hash:                proposal.Hash,
			CreatedAt:           proposal.CreatedAt,
			UpdatedAt:           proposal.UpdatedAt,
		})
	}
	return out
}
