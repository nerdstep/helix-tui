package runtime

import (
	"sync"
	"testing"
	"time"

	"helix-tui/internal/domain"
)

type fakeTradeEventAppender struct {
	mu     sync.Mutex
	events []domain.Event
	err    error
}

func (f *fakeTradeEventAppender) AppendMany(events []domain.Event) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, events...)
	return nil
}

func (f *fakeTradeEventAppender) snapshot() []domain.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.Event, len(f.events))
	copy(out, f.events)
	return out
}

func TestTradeEventPersistorFiltersAndPersists(t *testing.T) {
	appender := &fakeTradeEventAppender{}
	reporter := &fakePersistReporter{}
	persistor := newTradeEventPersistor(appender, reporter.Handle)

	persistor.HandleEvent(domain.Event{Type: "agent_cycle_start", Details: "mode=auto watchlist=3"})
	persistor.HandleEvent(domain.Event{Type: "order_placed", Details: "buy AAPL 1.00 (ord-1)"})
	persistor.HandleEvent(domain.Event{
		Type:            "agent_intent_rejected",
		Details:         "sell AAPL qty=1.00 type=limit conf=0.30 gain=0.10%",
		RejectionReason: "expected gain below minimum",
	})
	persistor.Close()

	got := appender.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected 2 persisted events, got %#v", got)
	}
	if got[0].Type != "order_placed" {
		t.Fatalf("expected first persisted event order_placed, got %#v", got[0])
	}
	if got[1].Type != "agent_intent_rejected" || got[1].RejectionReason != "expected gain below minimum" {
		t.Fatalf("unexpected rejected event payload: %#v", got[1])
	}
	if !hasEventType(reporter.snapshot(), "event_persist_stats") {
		t.Fatalf("expected stats event from persistor reporter")
	}
}

func TestTradeEventPersistorFlushesOnTicker(t *testing.T) {
	appender := &fakeTradeEventAppender{}
	persistor := newTradeEventPersistor(appender, nil)

	persistor.HandleEvent(domain.Event{Type: "order_canceled", Details: "ord-1"})
	time.Sleep(tradeEventPersistFlush + 100*time.Millisecond)
	persistor.Close()

	got := appender.snapshot()
	if len(got) != 1 || got[0].Type != "order_canceled" {
		t.Fatalf("expected periodic flush to persist order_canceled, got %#v", got)
	}
}

func TestTradeEventPersistorReportsFlushError(t *testing.T) {
	appender := &fakeTradeEventAppender{err: assertErr{}}
	reporter := &fakePersistReporter{}
	persistor := newTradeEventPersistor(appender, reporter.Handle)
	persistor.HandleEvent(domain.Event{Type: "order_placed", Details: "buy AAPL 1.00 (ord-1)"})
	persistor.Close()

	got := reporter.snapshot()
	if !hasEventType(got, "event_persist_error") {
		t.Fatalf("expected event_persist_error, got %#v", got)
	}
}

type fakePersistReporter struct {
	mu     sync.Mutex
	events []domain.Event
}

func (r *fakePersistReporter) Handle(event domain.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *fakePersistReporter) snapshot() []domain.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Event, len(r.events))
	copy(out, r.events)
	return out
}

type assertErr struct{}

func (assertErr) Error() string { return "append failed" }

func hasEventType(events []domain.Event, eventType string) bool {
	for _, e := range events {
		if e.Type == eventType {
			return true
		}
	}
	return false
}
