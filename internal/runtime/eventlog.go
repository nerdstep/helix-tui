package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"helix-tui/internal/domain"
)

type snapshotProvider interface {
	Snapshot() domain.Snapshot
}

type eventTailState struct {
	initialized bool
	last        domain.Event
}

func startEventFileLogger(ctx context.Context, eng snapshotProvider, path, mode, level string) (func(), error) {
	path = strings.TrimSpace(path)
	if path == "" || eng == nil {
		return func() {}, nil
	}
	openFlags, err := logFileOpenFlags(mode)
	if err != nil {
		return nil, err
	}
	logLevel, err := logLevelFromString(level)
	if err != nil {
		return nil, err
	}

	if err := ensureParentDir(path); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	file, err := os.OpenFile(path, openFlags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	logger := zerolog.New(file).
		Level(logLevel).
		With().
		Timestamp().
		Str("component", "eventlog").
		Logger()

	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		state := eventTailState{}
		initialEvents := eng.Snapshot().Events
		writeBatch(logger, eventsSince(state, initialEvents))
		state = advanceTail(state, initialEvents)

		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				events := eng.Snapshot().Events
				newEvents := eventsSince(state, events)
				writeBatch(logger, newEvents)
				state = advanceTail(state, events)
			}
		}
	}()

	stopFn := func() {
		close(stop)
		<-done
		_ = file.Close()
	}
	return stopFn, nil
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func writeBatch(logger zerolog.Logger, events []domain.Event) {
	for _, event := range events {
		entry := logger.
			WithLevel(eventLogLevel(event.Type)).
			Str("event_type", event.Type).
			Time("event_time", event.Time.Local())
		if strings.TrimSpace(event.RejectionReason) != "" {
			entry = entry.Str("rejection_reason", strings.TrimSpace(event.RejectionReason))
		}
		entry.Msg(event.Details)
	}
}

func eventsSince(state eventTailState, events []domain.Event) []domain.Event {
	if len(events) == 0 {
		return nil
	}
	if !state.initialized {
		out := make([]domain.Event, len(events))
		copy(out, events)
		return out
	}

	for i := len(events) - 1; i >= 0; i-- {
		if sameEvent(events[i], state.last) {
			if i+1 >= len(events) {
				return nil
			}
			out := make([]domain.Event, len(events[i+1:]))
			copy(out, events[i+1:])
			return out
		}
	}

	// If the previous tail isn't present (buffer wrapped), write the full snapshot.
	out := make([]domain.Event, len(events))
	copy(out, events)
	return out
}

func advanceTail(state eventTailState, events []domain.Event) eventTailState {
	if len(events) == 0 {
		return state
	}
	state.initialized = true
	state.last = events[len(events)-1]
	return state
}

func sameEvent(a, b domain.Event) bool {
	return a.Time.Equal(b.Time) &&
		a.Type == b.Type &&
		a.Details == b.Details &&
		a.RejectionReason == b.RejectionReason
}

func logFileOpenFlags(mode string) (int, error) {
	switch normalizedLogMode(mode) {
	case "append":
		return os.O_CREATE | os.O_APPEND | os.O_WRONLY, nil
	case "truncate":
		return os.O_CREATE | os.O_TRUNC | os.O_WRONLY, nil
	default:
		return 0, fmt.Errorf("invalid log mode %q (expected append or truncate)", strings.TrimSpace(mode))
	}
}

func normalizedLogMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "append"
	}
	return mode
}

func logLevelFromString(level string) (zerolog.Level, error) {
	raw := strings.TrimSpace(level)
	level = normalizedLogLevel(level)
	parsed, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.InfoLevel, fmt.Errorf("invalid log level %q (expected trace|debug|info|warn|error|fatal|panic|disabled)", raw)
	}
	return parsed, nil
}

func normalizedLogLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		return "info"
	}
	return level
}

func eventLogLevel(eventType string) zerolog.Level {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	switch eventType {
	case "agent_intent_rejected", "trade_update_unknown_order", "watchlist_sync_error", "compliance_rejected", "compliance_drift_detected":
		return zerolog.WarnLevel
	case "sync", "trade_update", "agent_cycle_start", "agent_cycle_idle", "agent_proposal", "agent_cycle_complete", "agent_heartbeat", "agent_context_changed", "agent_context_unchanged", "agent_context_summary", "quote_stream_start", "quote_stream_stop", "event_persist_stats", "strategy_cycle_skipped", "compliance_posture", "compliance_drift_cleared":
		return zerolog.DebugLevel
	case "agent_context_payload":
		return zerolog.TraceLevel
	}
	if strings.HasSuffix(eventType, "_error") {
		return zerolog.ErrorLevel
	}
	return zerolog.InfoLevel
}
