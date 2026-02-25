package util

import "testing"

func TestMaxFloat(t *testing.T) {
	if got := MaxFloat(1, 2); got != 2 {
		t.Fatalf("MaxFloat(1,2)=%v want 2", got)
	}
}

func TestClamp01(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{-1, 0},
		{0.2, 0.2},
		{3, 1},
	}
	for _, tc := range cases {
		if got := Clamp01(tc.in); got != tc.want {
			t.Fatalf("Clamp01(%v)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestMaxInt(t *testing.T) {
	if got := MaxInt(1, 2); got != 2 {
		t.Fatalf("MaxInt(1,2)=%v want 2", got)
	}
}

func TestMinInt(t *testing.T) {
	if got := MinInt(1, 2); got != 1 {
		t.Fatalf("MinInt(1,2)=%v want 1", got)
	}
}
