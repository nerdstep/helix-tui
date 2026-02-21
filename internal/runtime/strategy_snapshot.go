package runtime

import (
	"fmt"

	"helix-tui/internal/storage"
	"helix-tui/internal/tui"
)

func loadStrategySnapshot(repo *storage.StrategyRepository) (tui.StrategySnapshot, error) {
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
		Objective:     plan.Objective,
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
