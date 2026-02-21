package tui

import (
	"fmt"
	"strings"
	"time"

	"helix-tui/internal/domain"
)

var trackedCoreLoadingCommands = map[string]struct{}{
	"sync":    {},
	"flatten": {},
	"cancel":  {},
	"buy":     {},
	"sell":    {},
}

func (m Model) isLoading() bool {
	return m.commandBusy || m.strategyBusy
}

func (m *Model) startCommandLoading(label string) {
	m.commandBusy = true
	m.commandBusyLabel = strings.TrimSpace(label)
	if m.commandBusyLabel == "" {
		m.commandBusyLabel = "working"
	}
	m.status = m.commandBusyLabel + "..."
	m.statusError = false
}

func (m *Model) clearCommandLoading() {
	m.commandBusy = false
	m.commandBusyLabel = ""
}

func (m *Model) startStrategyLoading() {
	m.strategyBusy = true
	m.strategyBusySince = time.Now().UTC()
	m.status = "strategy run queued..."
	m.statusError = false
}

func (m *Model) clearStrategyLoading() {
	m.strategyBusy = false
	m.strategyBusySince = time.Time{}
}

func (m *Model) reconcileStrategyLoading(now time.Time) {
	if !m.strategyBusy {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	threshold := m.strategyBusySince.Add(-2 * time.Second)
	terminal := latestEventAfterAny(m.snapshot.Events, []string{
		"strategy_plan_created",
		"strategy_plan_unchanged",
		"strategy_plan_empty",
		"strategy_cycle_error",
	}, threshold)
	if terminal != nil {
		switch terminal.Type {
		case "strategy_plan_created":
			m.status = "strategy run complete: plan created"
			m.statusError = false
		case "strategy_plan_unchanged":
			m.status = "strategy run complete: no changes"
			m.statusError = false
		case "strategy_plan_empty":
			m.status = "strategy run complete: no new plan"
			m.statusError = false
		case "strategy_cycle_error":
			m.status = "strategy run failed: " + strings.TrimSpace(terminal.Details)
			m.statusError = true
		}
		m.clearStrategyLoading()
		return
	}

	lastStart := latestEventByType(m.snapshot.Events, "strategy_cycle_start")
	phase := "queued"
	if lastStart != nil && !lastStart.Time.Before(threshold) {
		phase = "running"
	}
	age := now.Sub(m.strategyBusySince)
	if age < 0 {
		age = 0
	}
	m.status = fmt.Sprintf("strategy run %s (%s elapsed)", phase, age.Round(time.Second))
	m.statusError = false
}

func latestEventAfterAny(events []domain.Event, eventTypes []string, threshold time.Time) *domain.Event {
	if len(events) == 0 || len(eventTypes) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(eventTypes))
	for _, t := range eventTypes {
		allowed[strings.TrimSpace(t)] = struct{}{}
	}
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Time.Before(threshold) {
			continue
		}
		if _, ok := allowed[e.Type]; ok {
			out := e
			return &out
		}
	}
	return nil
}

func shouldTrackCoreCommandLoading(raw string) bool {
	verb := coreCommandLabel(raw)
	_, ok := trackedCoreLoadingCommands[verb]
	return ok
}

func coreCommandLabel(raw string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	if len(fields) == 0 {
		return "command"
	}
	return fields[0]
}
