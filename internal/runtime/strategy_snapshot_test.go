package runtime

import (
	"path/filepath"
	"testing"
	"time"

	"helix-tui/internal/storage"
)

func TestLoadStrategySnapshotForThread_IncludesSelectedChatThread(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-snapshot.db")
	store, err := storage.Open(storage.Config{Path: path})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()
	planBundle, err := repo.CreatePlan(storage.StrategyPlan{
		GeneratedAt:   time.Now().UTC(),
		Status:        storage.StrategyPlanStatusActive,
		AnalystModel:  "gpt-5",
		PromptVersion: "strategy-v1",
		Watchlist:     []string{"AAPL"},
		Summary:       "Test",
		Confidence:    0.7,
	}, []storage.StrategyRecommendation{
		{Symbol: "AAPL", Bias: "buy", Priority: 1},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if err := repo.SetPlanStatus(planBundle.Plan.ID, storage.StrategyPlanStatusActive); err != nil {
		t.Fatalf("activate plan: %v", err)
	}

	thread1, err := repo.CreateChatThread("General")
	if err != nil {
		t.Fatalf("create thread1: %v", err)
	}
	thread2, err := repo.CreateChatThread("Swing")
	if err != nil {
		t.Fatalf("create thread2: %v", err)
	}
	if _, err := repo.AppendChatMessage(thread1.ID, "user", "thread1 message", ""); err != nil {
		t.Fatalf("append thread1 message: %v", err)
	}
	if _, err := repo.AppendChatMessage(thread2.ID, "user", "thread2 message", ""); err != nil {
		t.Fatalf("append thread2 message: %v", err)
	}
	if _, err := repo.UpsertSteeringState(storage.StrategySteeringStateInput{
		Source:              "operator",
		RiskProfile:         "balanced",
		MinConfidence:       0.6,
		MaxPositionNotional: 2000,
		Horizon:             "swing",
		PreferredSymbols:    []string{"AAPL"},
		ExcludedSymbols:     []string{"AMC"},
		Objective:           "Focus on quality momentum names.",
	}); err != nil {
		t.Fatalf("upsert steering: %v", err)
	}
	if _, err := repo.CreateProposal(storage.StrategyProposalInput{
		ThreadID:      thread2.ID,
		Source:        "copilot",
		Kind:          storage.StrategyProposalKindWatchlist,
		Rationale:     "Rotate into semis.",
		AddSymbols:    []string{"NVDA"},
		RemoveSymbols: []string{"AAPL"},
	}); err != nil {
		t.Fatalf("create strategy proposal: %v", err)
	}

	snapshot, err := loadStrategySnapshotForThread(repo, thread2.ID)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snapshot.Active == nil || snapshot.Active.ID != planBundle.Plan.ID {
		t.Fatalf("expected active strategy plan in snapshot, got %#v", snapshot.Active)
	}
	if snapshot.Chat.ActiveThreadID != thread2.ID {
		t.Fatalf("expected active chat thread id %d, got %d", thread2.ID, snapshot.Chat.ActiveThreadID)
	}
	if len(snapshot.Chat.Messages) != 1 || snapshot.Chat.Messages[0].ThreadID != thread2.ID {
		t.Fatalf("expected selected thread messages only, got %#v", snapshot.Chat.Messages)
	}
	if snapshot.Steering == nil {
		t.Fatalf("expected steering state in snapshot")
	}
	if snapshot.Steering.Version != 1 {
		t.Fatalf("expected steering version 1, got %d", snapshot.Steering.Version)
	}
	if len(snapshot.Proposals) != 1 {
		t.Fatalf("expected one proposal in snapshot, got %#v", snapshot.Proposals)
	}
	if snapshot.Proposals[0].Kind != "watchlist" || snapshot.Proposals[0].Status != "pending" {
		t.Fatalf("unexpected proposal state in snapshot: %#v", snapshot.Proposals[0])
	}
}

func TestLoadStrategySnapshotForThread_SeedsDefaultChatThread(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-snapshot-seed.db")
	store, err := storage.Open(storage.Config{Path: path})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	snapshot, err := loadStrategySnapshotForThread(store.Strategy(), 0)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snapshot.Chat.ActiveThreadID == 0 {
		t.Fatalf("expected seeded active chat thread id")
	}
	if len(snapshot.Chat.Threads) == 0 {
		t.Fatalf("expected seeded chat thread list")
	}
}
