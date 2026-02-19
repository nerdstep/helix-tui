package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"helix-tui/internal/domain"
)

type snapshotProvider interface {
	Snapshot() domain.Snapshot
}

type eventTailState struct {
	initialized bool
	last        domain.Event
}

func startEventFileLogger(ctx context.Context, eng snapshotProvider, path, mode string, stderr io.Writer) (func(), error) {
	path = strings.TrimSpace(path)
	if path == "" || eng == nil {
		return func() {}, nil
	}
	openFlags, err := logFileOpenFlags(mode)
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

	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		state := eventTailState{}
		initialEvents := eng.Snapshot().Events
		if err := writeBatch(file, eventsSince(state, initialEvents)); err != nil {
			writeStderr(stderr, fmt.Sprintf("event logger write failed: %v", err))
			return
		}
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
				if err := writeBatch(file, newEvents); err != nil {
					writeStderr(stderr, fmt.Sprintf("event logger write failed: %v", err))
					return
				}
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

func writeBatch(w io.Writer, events []domain.Event) error {
	for _, event := range events {
		line := fmt.Sprintf(
			"%s [%s] %s\n",
			event.Time.Local().Format("2006-01-02 15:04:05.000 MST"),
			event.Type,
			event.Details,
		)
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	return nil
}

func writeStderr(w io.Writer, line string) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintln(w, line)
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
	return a.Time.Equal(b.Time) && a.Type == b.Type && a.Details == b.Details
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
