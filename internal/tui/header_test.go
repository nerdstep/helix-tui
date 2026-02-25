package tui

import (
	"strings"
	"testing"
	"time"

	"helix-tui/internal/domain"
)

type stubHeaderTradingDayChecker struct {
	result bool
	err    error
}

func (s stubHeaderTradingDayChecker) IsTradingDay(time.Time) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.result, nil
}

func TestMarketSession(t *testing.T) {
	loc := newYorkLocation()
	tests := []struct {
		name      string
		at        time.Time
		wantLabel string
		wantOpen  bool
	}{
		{
			name:      "weekend",
			at:        time.Date(2026, time.February, 21, 10, 0, 0, 0, loc), // Saturday
			wantLabel: "CLOSED (weekend)",
			wantOpen:  false,
		},
		{
			name:      "premarket",
			at:        time.Date(2026, time.February, 20, 8, 15, 0, 0, loc),
			wantLabel: "PRE (04:00-09:30 ET)",
			wantOpen:  false,
		},
		{
			name:      "regular",
			at:        time.Date(2026, time.February, 20, 10, 0, 0, 0, loc),
			wantLabel: "OPEN (regular)",
			wantOpen:  true,
		},
		{
			name:      "afterhours",
			at:        time.Date(2026, time.February, 20, 17, 30, 0, 0, loc),
			wantLabel: "AFTER (16:00-20:00 ET)",
			wantOpen:  false,
		},
		{
			name:      "overnight",
			at:        time.Date(2026, time.February, 20, 2, 30, 0, 0, loc),
			wantLabel: "CLOSED",
			wantOpen:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, open := marketSession(tt.at)
			if label != tt.wantLabel || open != tt.wantOpen {
				t.Fatalf("marketSession(%s) = (%q, %v), want (%q, %v)", tt.at, label, open, tt.wantLabel, tt.wantOpen)
			}
		})
	}
}

func TestMarketSession_HolidayChecker(t *testing.T) {
	loc := newYorkLocation()
	label, open := marketSessionWithChecker(
		time.Date(2026, time.February, 17, 10, 0, 0, 0, loc),
		stubHeaderTradingDayChecker{result: false},
	)
	if label != "CLOSED (holiday)" || open {
		t.Fatalf("expected holiday closure, got label=%q open=%v", label, open)
	}
}

func TestHeaderAgentStatusFromEvent(t *testing.T) {
	m := New(newTestEngine())
	m.snapshot.Events = append(m.snapshot.Events, domain.Event{
		Time:    time.Now().UTC(),
		Type:    "agent_mode",
		Details: "mode=auto agent=llm watchlist=AAPL,MSFT",
	})
	got := m.headerAgentStatus()
	if !strings.Contains(got, "auto/llm") {
		t.Fatalf("expected agent status to include auto/llm, got %q", got)
	}
}

func TestBuildAccountSummaryIncludesStatusLines(t *testing.T) {
	m := New(newTestEngine())
	m.snapshot.Events = append(m.snapshot.Events, domain.Event{
		Time:    time.Now().UTC(),
		Type:    "sync",
		Details: "reconciled account, positions, and orders",
	})
	m.snapshot.Events = append(m.snapshot.Events, domain.Event{
		Time:    time.Now().UTC(),
		Type:    "alpaca_config",
		Details: "env=paper endpoint=https://paper-api.alpaca.markets feed=iex credentials=env",
	})
	summary := m.buildAccountSummary()
	for _, needle := range []string{"Market:", "Clock:", "NY:", "Alpaca:", "Agent:", "Last Sync:"} {
		if !strings.Contains(summary, needle) {
			t.Fatalf("expected summary to contain %q, got %q", needle, summary)
		}
	}
}

func TestBuildAccountSummaryUsesThreeLines(t *testing.T) {
	m := New(newTestEngine())
	summary := m.buildAccountSummary()
	lines := strings.Split(summary, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 summary lines, got %d: %q", len(lines), summary)
	}
	if !strings.Contains(lines[0], "Cash:") || !strings.Contains(lines[0], "Buying Power:") || !strings.Contains(lines[0], "Equity:") {
		t.Fatalf("unexpected first line: %q", lines[0])
	}
	if !strings.Contains(lines[1], "Market:") || !strings.Contains(lines[1], "Clock:") || !strings.Contains(lines[1], "NY:") {
		t.Fatalf("unexpected second line: %q", lines[1])
	}
	if !strings.Contains(lines[2], "Alpaca:") || !strings.Contains(lines[2], "Agent:") || !strings.Contains(lines[2], "Last Sync:") {
		t.Fatalf("unexpected third line: %q", lines[2])
	}
}

func TestHeaderAlpacaEnvStatus(t *testing.T) {
	m := New(newTestEngine())
	if got := m.headerAlpacaEnvStatus(); !strings.Contains(got, "Unknown") {
		t.Fatalf("expected Unknown env with no event, got %q", got)
	}

	m.snapshot.Events = append(m.snapshot.Events, domain.Event{
		Time:    time.Now().UTC(),
		Type:    "alpaca_config",
		Details: "env=live endpoint=https://api.alpaca.markets feed=iex credentials=env",
	})
	if got := m.headerAlpacaEnvStatus(); !strings.Contains(got, "Live") {
		t.Fatalf("expected Live env, got %q", got)
	}

	m.snapshot.Events = append(m.snapshot.Events, domain.Event{
		Time:    time.Now().UTC(),
		Type:    "alpaca_config",
		Details: "env=paper endpoint=https://paper-api.alpaca.markets feed=iex credentials=env",
	})
	if got := m.headerAlpacaEnvStatus(); !strings.Contains(got, "Paper") {
		t.Fatalf("expected Paper env, got %q", got)
	}
}
