package storage

import (
	"path/filepath"
	"testing"
)

func TestStrategyRepositoryUpsertAndGetSteeringState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-steering.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()
	state, err := repo.GetSteeringState()
	if err != nil {
		t.Fatalf("GetSteeringState failed: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil steering state before upsert, got %#v", state)
	}

	upserted, err := repo.UpsertSteeringState(StrategySteeringStateInput{
		Source:              "operator",
		RiskProfile:         "balanced",
		MinConfidence:       0.62,
		MaxPositionNotional: 2500,
		Horizon:             "swing_2w",
		Objective:           "Favor quality momentum names while reducing biotech exposure.",
		PreferredSymbols:    []string{"nvda", "aapl", "msft"},
		ExcludedSymbols:     []string{"gme", "amc"},
	})
	if err != nil {
		t.Fatalf("UpsertSteeringState failed: %v", err)
	}
	if upserted.ID != strategySteeringSingletonID {
		t.Fatalf("expected singleton id %d, got %d", strategySteeringSingletonID, upserted.ID)
	}
	if upserted.Version != 1 {
		t.Fatalf("expected version 1, got %d", upserted.Version)
	}
	if upserted.Hash == "" {
		t.Fatalf("expected non-empty steering hash")
	}
	if got := len(upserted.PreferredSymbols); got != 3 {
		t.Fatalf("expected 3 preferred symbols, got %d", got)
	}

	loaded, err := repo.GetSteeringState()
	if err != nil {
		t.Fatalf("GetSteeringState failed: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected persisted steering state")
	}
	if loaded.Version != 1 {
		t.Fatalf("expected loaded version 1, got %d", loaded.Version)
	}
	if loaded.Hash != upserted.Hash {
		t.Fatalf("expected hash %q, got %q", upserted.Hash, loaded.Hash)
	}
	if len(loaded.PreferredSymbols) != 3 || loaded.PreferredSymbols[0] != "AAPL" {
		t.Fatalf("unexpected preferred symbols: %#v", loaded.PreferredSymbols)
	}
	if len(loaded.ExcludedSymbols) != 2 || loaded.ExcludedSymbols[0] != "AMC" {
		t.Fatalf("unexpected excluded symbols: %#v", loaded.ExcludedSymbols)
	}
}

func TestStrategyRepositoryUpsertSteeringStateVersionIncrementsOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-steering-version.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()
	first, err := repo.UpsertSteeringState(StrategySteeringStateInput{
		Source:              "operator",
		RiskProfile:         "conservative",
		MinConfidence:       0.7,
		MaxPositionNotional: 1200,
		PreferredSymbols:    []string{"AAPL"},
	})
	if err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	second, err := repo.UpsertSteeringState(StrategySteeringStateInput{
		Source:              "operator",
		RiskProfile:         "conservative",
		MinConfidence:       0.7,
		MaxPositionNotional: 1200,
		PreferredSymbols:    []string{"AAPL"},
	})
	if err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if second.Version != first.Version {
		t.Fatalf("expected version unchanged on identical input, first=%d second=%d", first.Version, second.Version)
	}

	third, err := repo.UpsertSteeringState(StrategySteeringStateInput{
		Source:              "operator",
		RiskProfile:         "conservative",
		MinConfidence:       0.75,
		MaxPositionNotional: 1200,
		PreferredSymbols:    []string{"AAPL"},
	})
	if err != nil {
		t.Fatalf("third upsert failed: %v", err)
	}
	if third.Version != second.Version+1 {
		t.Fatalf("expected version increment on changed input, second=%d third=%d", second.Version, third.Version)
	}
	if third.Hash == second.Hash {
		t.Fatalf("expected changed hash after changed input")
	}
}
