package runtime

import (
	"fmt"
	"time"

	"helix-tui/internal/engine"
	"helix-tui/internal/storage"
)

type complianceStateAdapter struct {
	repo *storage.ComplianceStateRepository
}

func (a complianceStateAdapter) LoadUnsettledSellProceeds(asOf time.Time) ([]engine.UnsettledSellProceeds, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("compliance state repository is not initialized")
	}
	lots, err := a.repo.ListUnsettledSells(asOf)
	if err != nil {
		return nil, err
	}
	out := make([]engine.UnsettledSellProceeds, 0, len(lots))
	for _, lot := range lots {
		out = append(out, engine.UnsettledSellProceeds{
			Amount:    lot.Amount,
			SettlesAt: lot.SettlesAt,
		})
	}
	return out, nil
}

func (a complianceStateAdapter) AppendUnsettledSellProceeds(lot engine.UnsettledSellProceeds, createdAt time.Time) error {
	if a.repo == nil {
		return fmt.Errorf("compliance state repository is not initialized")
	}
	return a.repo.AppendUnsettledSell(storage.ComplianceUnsettledSell{
		Amount:    lot.Amount,
		SettlesAt: lot.SettlesAt,
		CreatedAt: createdAt,
	})
}

func (a complianceStateAdapter) PruneSettledSellProceeds(asOf time.Time) error {
	if a.repo == nil {
		return fmt.Errorf("compliance state repository is not initialized")
	}
	return a.repo.PruneSettledSells(asOf)
}
