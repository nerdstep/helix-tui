package util

import (
	"testing"
	"time"
)

func TestDateAtUTCMidnight(t *testing.T) {
	in := time.Date(2026, time.February, 25, 20, 15, 1, 0, time.FixedZone("X", -5*3600))
	got := DateAtUTCMidnight(in)
	want := time.Date(2026, time.February, 26, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("DateAtUTCMidnight mismatch: got %v want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Fatalf("DateAtUTCMidnight location: got %v want UTC", got.Location())
	}
}
