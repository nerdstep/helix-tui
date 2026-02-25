package util

import (
	"reflect"
	"testing"
)

func TestDedupeSortedStrings(t *testing.T) {
	got := DedupeSortedStrings([]string{" b ", "a", "A", "", "a"})
	want := []string{"A", "a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DedupeSortedStrings mismatch: got %#v want %#v", got, want)
	}
}

func TestRemoveOverlappingStrings(t *testing.T) {
	got := RemoveOverlappingStrings([]string{"A", "B"}, []string{"B", "C"})
	want := []string{"C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RemoveOverlappingStrings mismatch: got %#v want %#v", got, want)
	}
}
