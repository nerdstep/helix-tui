package storage

import (
	"path/filepath"
	"testing"
)

func TestStrategyRepositoryCreateAndListWatchlistProposal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-proposals-watchlist.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	thread, err := store.Strategy().CreateChatThread("General")
	if err != nil {
		t.Fatalf("create chat thread: %v", err)
	}

	created, err := store.Strategy().CreateProposal(StrategyProposalInput{
		ThreadID:      thread.ID,
		Source:        "copilot",
		Kind:          StrategyProposalKindWatchlist,
		Rationale:     "Rotate into stronger momentum names.",
		AddSymbols:    []string{"aapl", "msft", "MSFT"},
		RemoveSymbols: []string{"tsla", "AAPL"},
	})
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected proposal id")
	}
	if created.Status != StrategyProposalStatusPending {
		t.Fatalf("expected pending status, got %q", created.Status)
	}
	if len(created.AddSymbols) != 2 || created.AddSymbols[0] != "AAPL" || created.AddSymbols[1] != "MSFT" {
		t.Fatalf("unexpected add symbols: %#v", created.AddSymbols)
	}
	// AAPL is dropped from remove list because it overlaps add symbols.
	if len(created.RemoveSymbols) != 1 || created.RemoveSymbols[0] != "TSLA" {
		t.Fatalf("unexpected remove symbols: %#v", created.RemoveSymbols)
	}
	if created.Hash == "" {
		t.Fatalf("expected non-empty proposal hash")
	}

	listed, err := store.Strategy().ListProposals(10)
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(listed))
	}
	if listed[0].ID != created.ID {
		t.Fatalf("unexpected listed proposal id: got %d want %d", listed[0].ID, created.ID)
	}
}

func TestStrategyRepositoryCreateAndApplySteeringProposal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-proposals-steering.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	thread, err := store.Strategy().CreateChatThread("Risk")
	if err != nil {
		t.Fatalf("create chat thread: %v", err)
	}

	created, err := store.Strategy().CreateProposal(StrategyProposalInput{
		ThreadID:            thread.ID,
		Source:              "copilot",
		Kind:                StrategyProposalKindSteering,
		Rationale:           "Reduce exposure and raise confidence threshold.",
		RiskProfile:         "Conservative",
		MinConfidence:       0.7,
		MaxPositionNotional: 2500,
		Horizon:             "swing",
		Objective:           "Prefer high-liquidity names.",
		PreferredSymbols:    []string{"msft", "aapl"},
		ExcludedSymbols:     []string{"gme", "MSFT"},
	})
	if err != nil {
		t.Fatalf("create steering proposal: %v", err)
	}
	if created.Kind != StrategyProposalKindSteering {
		t.Fatalf("expected steering kind, got %q", created.Kind)
	}
	if len(created.ExcludedSymbols) != 1 || created.ExcludedSymbols[0] != "GME" {
		t.Fatalf("unexpected excluded symbols after overlap normalization: %#v", created.ExcludedSymbols)
	}

	if err := store.Strategy().SetProposalStatus(created.ID, StrategyProposalStatusApplied); err != nil {
		t.Fatalf("set proposal status: %v", err)
	}
	loaded, err := store.Strategy().GetProposal(created.ID)
	if err != nil {
		t.Fatalf("get proposal: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected loaded proposal")
	}
	if loaded.Status != StrategyProposalStatusApplied {
		t.Fatalf("expected applied status, got %q", loaded.Status)
	}
}

func TestStrategyRepositoryCreateProposalValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-proposals-validation.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	thread, err := store.Strategy().CreateChatThread("Validation")
	if err != nil {
		t.Fatalf("create chat thread: %v", err)
	}

	if _, err := store.Strategy().CreateProposal(StrategyProposalInput{
		ThreadID: thread.ID,
		Kind:     StrategyProposalKindWatchlist,
	}); err == nil {
		t.Fatalf("expected validation error for empty watchlist proposal")
	}

	if _, err := store.Strategy().CreateProposal(StrategyProposalInput{
		ThreadID: thread.ID,
		Kind:     StrategyProposalKindSteering,
	}); err == nil {
		t.Fatalf("expected validation error for empty steering proposal")
	}

	if err := store.Strategy().SetProposalStatus(99999, StrategyProposalStatusRejected); err == nil {
		t.Fatalf("expected not found error for status update")
	}
}
