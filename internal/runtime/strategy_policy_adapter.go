package runtime

import (
	"strings"

	"helix-tui/internal/autonomy"
	"helix-tui/internal/storage"
)

type strategyPolicyAdapter struct {
	repo *storage.StrategyRepository
}

func (a strategyPolicyAdapter) GetActiveStrategyPolicy() (*autonomy.ActiveStrategyPolicy, error) {
	if a.repo == nil {
		return nil, nil
	}
	active, err := a.repo.GetActivePlan()
	if err != nil {
		return nil, err
	}
	if active == nil {
		return nil, nil
	}
	recs := make([]autonomy.StrategyConstraint, 0, len(active.Recommendations))
	for _, rec := range active.Recommendations {
		recs = append(recs, autonomy.StrategyConstraint{
			Symbol:      strings.ToUpper(strings.TrimSpace(rec.Symbol)),
			Bias:        strings.ToLower(strings.TrimSpace(rec.Bias)),
			MaxNotional: rec.MaxNotional,
		})
	}
	return &autonomy.ActiveStrategyPolicy{
		PlanID:          active.Plan.ID,
		GeneratedAt:     active.Plan.GeneratedAt,
		Recommendations: recs,
	}, nil
}
