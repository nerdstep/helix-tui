package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStrategyChatRepositoryCreateListAppendAndSelect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-chat.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()
	if repo == nil {
		t.Fatalf("expected strategy repository")
	}

	thread, err := repo.CreateChatThread("Swing Ideas")
	if err != nil {
		t.Fatalf("CreateChatThread failed: %v", err)
	}
	if thread.ID == 0 {
		t.Fatalf("expected created thread ID")
	}
	if thread.Title != "Swing Ideas" {
		t.Fatalf("unexpected thread title: %q", thread.Title)
	}

	if _, err := repo.AppendChatMessage(thread.ID, "user", "Analyze BYND and RIVN swing setup.", ""); err != nil {
		t.Fatalf("AppendChatMessage user failed: %v", err)
	}
	if _, err := repo.AppendChatMessage(thread.ID, "assistant", "Momentum is mixed; focus on tighter entries.", "gpt-5"); err != nil {
		t.Fatalf("AppendChatMessage assistant failed: %v", err)
	}

	threads, err := repo.ListChatThreads(10)
	if err != nil {
		t.Fatalf("ListChatThreads failed: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0].LastMessageAt.Before(thread.LastMessageAt) {
		t.Fatalf("expected updated last_message_at after appends")
	}

	latest, err := repo.GetLatestChatThread()
	if err != nil {
		t.Fatalf("GetLatestChatThread failed: %v", err)
	}
	if latest == nil || latest.ID != thread.ID {
		t.Fatalf("expected latest thread id %d, got %#v", thread.ID, latest)
	}

	msgs, err := repo.ListChatMessages(thread.ID, 20)
	if err != nil {
		t.Fatalf("ListChatMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("unexpected message order/roles: %#v", msgs)
	}
}

func TestStrategyChatRepositoryEnsureThread(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-chat-ensure.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()

	first, err := repo.EnsureChatThread("")
	if err != nil {
		t.Fatalf("EnsureChatThread first failed: %v", err)
	}
	if first.ID == 0 {
		t.Fatalf("expected ensured thread ID")
	}
	if first.Title == "" {
		t.Fatalf("expected default title")
	}

	time.Sleep(5 * time.Millisecond)

	second, err := repo.EnsureChatThread("Ignored")
	if err != nil {
		t.Fatalf("EnsureChatThread second failed: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected same ensured thread, got first=%d second=%d", first.ID, second.ID)
	}
}

func TestStrategyChatRepositoryAppendRequiresValidInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy-chat-errors.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	repo := store.Strategy()
	thread, err := repo.CreateChatThread("Main")
	if err != nil {
		t.Fatalf("CreateChatThread failed: %v", err)
	}

	if _, err := repo.AppendChatMessage(0, "user", "hello", ""); err == nil {
		t.Fatalf("expected error for missing thread id")
	}
	if _, err := repo.AppendChatMessage(thread.ID, "nope", "hello", ""); err == nil {
		t.Fatalf("expected error for invalid role")
	}
	if _, err := repo.AppendChatMessage(thread.ID, "user", " ", ""); err == nil {
		t.Fatalf("expected error for empty content")
	}
	if _, err := repo.AppendChatMessage(thread.ID+1000, "user", "hello", ""); err == nil {
		t.Fatalf("expected error for missing thread")
	}
}
