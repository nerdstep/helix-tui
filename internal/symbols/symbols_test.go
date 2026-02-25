package symbols

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	got := Normalize([]string{"aapl", " AAPL ", "", "msft"})
	want := []string{"AAPL", "MSFT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize mismatch: got %#v want %#v", got, want)
	}
}

func TestMerge(t *testing.T) {
	got := Merge([]string{"aapl", "msft"}, []string{"AAPL", " tsla "})
	want := []string{"AAPL", "MSFT", "TSLA"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Merge mismatch: got %#v want %#v", got, want)
	}
}

func TestNormalizeSorted(t *testing.T) {
	got := NormalizeSorted([]string{"msft", " aapl ", "TSLA", "aapl"})
	want := []string{"AAPL", "MSFT", "TSLA"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeSorted mismatch: got %#v want %#v", got, want)
	}
}
