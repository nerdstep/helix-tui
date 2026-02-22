package runtime

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"

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

func TestNormalizedLogLevel(t *testing.T) {
	if got := normalizedLogLevel("  "); got != "info" {
		t.Fatalf("expected default info level, got %q", got)
	}
	if got := normalizedLogLevel(" WARN "); got != "warn" {
		t.Fatalf("expected lowercased warn level, got %q", got)
	}
}

func TestLogLevelFromString(t *testing.T) {
	level, err := logLevelFromString("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if level != zerolog.InfoLevel {
		t.Fatalf("expected info level, got %s", level)
	}

	level, err = logLevelFromString(" WARN ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if level != zerolog.WarnLevel {
		t.Fatalf("expected warn level, got %s", level)
	}

	_, err = logLevelFromString("verbose")
	if err == nil {
		t.Fatalf("expected invalid level error")
	}
}

func TestEventLogLevel(t *testing.T) {
	tests := []struct {
		eventType string
		want      zerolog.Level
	}{
		{eventType: "sync", want: zerolog.DebugLevel},
		{eventType: "agent_cycle_complete", want: zerolog.DebugLevel},
		{eventType: "agent_heartbeat", want: zerolog.DebugLevel},
		{eventType: "agent_context_changed", want: zerolog.DebugLevel},
		{eventType: "agent_context_unchanged", want: zerolog.DebugLevel},
		{eventType: "agent_context_summary", want: zerolog.DebugLevel},
		{eventType: "quote_stream_start", want: zerolog.DebugLevel},
		{eventType: "quote_stream_stop", want: zerolog.DebugLevel},
		{eventType: "event_persist_stats", want: zerolog.DebugLevel},
		{eventType: "compliance_posture", want: zerolog.DebugLevel},
		{eventType: "compliance_drift_cleared", want: zerolog.DebugLevel},
		{eventType: "agent_context_payload", want: zerolog.TraceLevel},
		{eventType: "agent_intent_rejected", want: zerolog.WarnLevel},
		{eventType: "compliance_rejected", want: zerolog.WarnLevel},
		{eventType: "compliance_drift_detected", want: zerolog.WarnLevel},
		{eventType: "watchlist_sync_error", want: zerolog.WarnLevel},
		{eventType: "agent_cycle_error", want: zerolog.ErrorLevel},
		{eventType: "quote_stream_error", want: zerolog.ErrorLevel},
		{eventType: "agent_intent_executed", want: zerolog.InfoLevel},
	}
	for _, tt := range tests {
		if got := eventLogLevel(tt.eventType); got != tt.want {
			t.Fatalf("eventLogLevel(%q) = %s, want %s", tt.eventType, got, tt.want)
		}
	}
}
