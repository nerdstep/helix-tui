package runtime

import (
	"os"
	"reflect"
	"testing"
	"time"

	"helix-tui/internal/domain"
)

func TestEventsSinceInitialStateReturnsAll(t *testing.T) {
	now := time.Now().UTC()
	events := []domain.Event{
		{Time: now, Type: "a", Details: "1"},
		{Time: now.Add(time.Second), Type: "b", Details: "2"},
	}
	got := eventsSince(eventTailState{}, events)
	if !reflect.DeepEqual(got, events) {
		t.Fatalf("unexpected events: %#v", got)
	}
}

func TestEventsSinceReturnsOnlyNewEvents(t *testing.T) {
	now := time.Now().UTC()
	base := []domain.Event{
		{Time: now, Type: "a", Details: "1"},
		{Time: now.Add(time.Second), Type: "b", Details: "2"},
	}
	state := advanceTail(eventTailState{}, base)
	next := append(base, domain.Event{Time: now.Add(2 * time.Second), Type: "c", Details: "3"})

	got := eventsSince(state, next)
	want := []domain.Event{{Time: now.Add(2 * time.Second), Type: "c", Details: "3"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected events: %#v", got)
	}
}

func TestEventsSinceWhenTailMissingReturnsAll(t *testing.T) {
	now := time.Now().UTC()
	state := eventTailState{
		initialized: true,
		last:        domain.Event{Time: now, Type: "old", Details: "old"},
	}
	events := []domain.Event{
		{Time: now.Add(time.Second), Type: "a", Details: "1"},
	}
	got := eventsSince(state, events)
	if !reflect.DeepEqual(got, events) {
		t.Fatalf("unexpected events: %#v", got)
	}
}

func TestLogFileOpenFlags(t *testing.T) {
	flags, err := logFileOpenFlags("append")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags != (os.O_CREATE | os.O_APPEND | os.O_WRONLY) {
		t.Fatalf("unexpected append flags: %d", flags)
	}

	flags, err = logFileOpenFlags("truncate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flags != (os.O_CREATE | os.O_TRUNC | os.O_WRONLY) {
		t.Fatalf("unexpected truncate flags: %d", flags)
	}

	_, err = logFileOpenFlags("rotate")
	if err == nil {
		t.Fatalf("expected invalid mode error")
	}
}
