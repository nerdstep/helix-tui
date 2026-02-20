package storage

import (
	"path/filepath"
	"testing"
	"time"

	"helix-tui/internal/domain"
)

func TestTradeEventRepositoryAppendManyStoresStructuredFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.db")
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Now().UTC()
	err = store.Events().AppendMany([]domain.Event{
		{
			Time:    now,
			Type:    "order_placed",
			Details: "buy AAPL 10.00 (ord-1)",
		},
		{
			Time:    now.Add(time.Second),
			Type:    "trade_update",
			Details: "ord-1 status=filled filled=10.00",
		},
		{
			Time:    now.Add(2 * time.Second),
			Type:    "agent_intent_executed",
			Details: "buy AAPL qty=10.00 type=limit conf=0.80 gain=2.25% rationale=breakout setup",
		},
		{
			Time:            now.Add(3 * time.Second),
			Type:            "agent_intent_rejected",
			Details:         "sell AAPL qty=5.00 type=limit conf=0.30 gain=0.10%",
			RejectionReason: "expected gain below minimum",
		},
	})
	if err != nil {
		t.Fatalf("AppendMany failed: %v", err)
	}

	var records []tradeEventRecord
	if err := store.db.Order("id asc").Find(&records).Error; err != nil {
		t.Fatalf("query trade_events failed: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("expected 4 records, got %d", len(records))
	}

	if records[0].EventType != "order_placed" || records[0].OrderID != "ord-1" || records[0].Symbol != "AAPL" || records[0].Side != "buy" || records[0].Qty != 10 {
		t.Fatalf("unexpected structured order_placed row: %#v", records[0])
	}
	if records[1].EventType != "trade_update" || records[1].OrderID != "ord-1" || records[1].OrderStatus != "filled" || records[1].Qty != 10 {
		t.Fatalf("unexpected structured trade_update row: %#v", records[1])
	}
	if records[2].EventType != "agent_intent_executed" || records[2].Symbol != "AAPL" || records[2].OrderType != "limit" || records[2].Confidence != 0.8 || records[2].ExpectedGainPct != 2.25 {
		t.Fatalf("unexpected structured intent row: %#v", records[2])
	}
	if records[3].EventType != "agent_intent_rejected" || records[3].Symbol != "AAPL" || records[3].RejectionReason != "expected gain below minimum" {
		t.Fatalf("unexpected structured rejected intent row: %#v", records[3])
	}

	recent, err := store.Events().ListRecent(10)
	if err != nil {
		t.Fatalf("ListRecent failed: %v", err)
	}
	if len(recent) != 4 {
		t.Fatalf("expected 4 recent events, got %#v", recent)
	}
	last := recent[len(recent)-1]
	if last.Type != "agent_intent_rejected" || last.RejectionReason != "expected gain below minimum" {
		t.Fatalf("unexpected recent rejected event: %#v", last)
	}
	if last.Details != "sell AAPL qty=5.00 type=limit conf=0.30 gain=0.10%" {
		t.Fatalf("unexpected rejected details in recent list: %#v", last.Details)
	}
}
