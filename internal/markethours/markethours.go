package markethours

import (
	"strings"
	"sync"
	"time"
)

type Phase string

const (
	PhaseClosed     Phase = "closed"
	PhasePremarket  Phase = "pre"
	PhaseRegular    Phase = "regular"
	PhaseAfterHours Phase = "after"
)

type TradingDayChecker interface {
	IsTradingDay(day time.Time) (bool, error)
}

var (
	newYorkOnce sync.Once
	newYorkLoc  *time.Location
)

func NewYorkLocation() *time.Location {
	newYorkOnce.Do(func() {
		loc, err := time.LoadLocation("America/New_York")
		if err != nil {
			newYorkLoc = time.Local
			return
		}
		newYorkLoc = loc
	})
	return newYorkLoc
}

func PhaseAt(now time.Time, checker TradingDayChecker) Phase {
	ny := now.In(NewYorkLocation())
	if !isTradingDay(ny, checker) {
		return PhaseClosed
	}
	minuteOfDay := ny.Hour()*60 + ny.Minute()
	switch {
	case minuteOfDay >= 9*60+30 && minuteOfDay < 16*60:
		return PhaseRegular
	case minuteOfDay >= 4*60 && minuteOfDay < 9*60+30:
		return PhasePremarket
	case minuteOfDay >= 16*60 && minuteOfDay < 20*60:
		return PhaseAfterHours
	default:
		return PhaseClosed
	}
}

func SessionLabel(now time.Time, checker TradingDayChecker) (label string, open bool) {
	ny := now.In(NewYorkLocation())
	if !isTradingDay(ny, checker) {
		if ny.Weekday() == time.Saturday || ny.Weekday() == time.Sunday {
			return "CLOSED (weekend)", false
		}
		return "CLOSED (holiday)", false
	}
	switch PhaseAt(now, checker) {
	case PhaseRegular:
		return "OPEN (regular)", true
	case PhasePremarket:
		return "PRE (04:00-09:30 ET)", false
	case PhaseAfterHours:
		return "AFTER (16:00-20:00 ET)", false
	default:
		return "CLOSED", false
	}
}

func inPreOpenWarmup(now time.Time, warmup time.Duration) bool {
	if warmup <= 0 {
		return false
	}
	ny := now.In(NewYorkLocation())
	openTime := time.Date(ny.Year(), ny.Month(), ny.Day(), 9, 30, 0, 0, ny.Location())
	warmupStart := openTime.Add(-warmup)
	return !ny.Before(warmupStart) && ny.Before(openTime)
}

func InPreOpenWarmup(now time.Time, warmup time.Duration, checker TradingDayChecker) bool {
	ny := now.In(NewYorkLocation())
	if !isTradingDay(ny, checker) {
		return false
	}
	return inPreOpenWarmup(now, warmup)
}

func isTradingDay(ny time.Time, checker TradingDayChecker) bool {
	if ny.Weekday() == time.Saturday || ny.Weekday() == time.Sunday {
		return false
	}
	if checker == nil {
		return true
	}
	day := time.Date(ny.Year(), ny.Month(), ny.Day(), 0, 0, 0, 0, time.UTC)
	ok, err := checker.IsTradingDay(day)
	if err != nil {
		return true
	}
	return ok
}

func IsAfterHoursLabel(label string) bool {
	label = strings.ToUpper(strings.TrimSpace(label))
	return strings.Contains(label, "PRE") || strings.Contains(label, "AFTER")
}
