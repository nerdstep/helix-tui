package markethours

import (
	"fmt"
	"testing"
	"time"
)

type stubTradingDayChecker struct {
	result bool
	err    error
}

func (s stubTradingDayChecker) IsTradingDay(time.Time) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.result, nil
}

func TestPhaseAt(t *testing.T) {
	loc := NewYorkLocation()
	tests := []struct {
		name string
		at   time.Time
		want Phase
	}{
		{name: "weekend", at: time.Date(2026, time.February, 21, 10, 0, 0, 0, loc), want: PhaseClosed},
		{name: "premarket", at: time.Date(2026, time.February, 20, 8, 15, 0, 0, loc), want: PhasePremarket},
		{name: "regular", at: time.Date(2026, time.February, 20, 10, 0, 0, 0, loc), want: PhaseRegular},
		{name: "afterhours", at: time.Date(2026, time.February, 20, 17, 30, 0, 0, loc), want: PhaseAfterHours},
		{name: "overnight", at: time.Date(2026, time.February, 20, 2, 30, 0, 0, loc), want: PhaseClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PhaseAt(tt.at, nil); got != tt.want {
				t.Fatalf("PhaseAt(%s) = %s, want %s", tt.at, got, tt.want)
			}
		})
	}
}

func TestSessionLabel_HolidayFromChecker(t *testing.T) {
	loc := NewYorkLocation()
	label, open := SessionLabel(time.Date(2026, time.February, 17, 10, 0, 0, 0, loc), stubTradingDayChecker{result: false})
	if label != "CLOSED (holiday)" || open {
		t.Fatalf("unexpected session label/open: %q %v", label, open)
	}
}

func TestPhaseAt_CheckerErrorFallsBackToWeekdayHours(t *testing.T) {
	loc := NewYorkLocation()
	at := time.Date(2026, time.February, 17, 10, 0, 0, 0, loc)
	if got := PhaseAt(at, stubTradingDayChecker{err: fmt.Errorf("calendar down")}); got != PhaseRegular {
		t.Fatalf("expected regular phase fallback on checker error, got %s", got)
	}
}
