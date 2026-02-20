package tui

import (
	"reflect"
	"testing"
)

func TestNormalizeSparklineValues(t *testing.T) {
	values := []float64{100000.0, 100002.5, 99999.0, 100001.0}
	got, maxValue := normalizeSparklineValues(values)

	want := []float64{1.0, 3.5, 0.0, 2.0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized values mismatch: got %#v want %#v", got, want)
	}
	if maxValue != 3.5 {
		t.Fatalf("unexpected maxValue: got %f want 3.5", maxValue)
	}
}

func TestNormalizeSparklineValues_FlatSeries(t *testing.T) {
	values := []float64{100000.0, 100000.0, 100000.0}
	got, maxValue := normalizeSparklineValues(values)

	want := []float64{0.5, 0.5, 0.5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flat normalized values mismatch: got %#v want %#v", got, want)
	}
	if maxValue != 1 {
		t.Fatalf("unexpected maxValue for flat series: got %f want 1", maxValue)
	}
}
