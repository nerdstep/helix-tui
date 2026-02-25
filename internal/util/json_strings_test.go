package util

import (
	"reflect"
	"testing"
)

func TestEncodeStringListJSON(t *testing.T) {
	got, err := EncodeStringListJSON([]string{" b ", "a", "a"})
	if err != nil {
		t.Fatalf("EncodeStringListJSON error: %v", err)
	}
	want := "[\"a\",\"b\"]"
	if got != want {
		t.Fatalf("EncodeStringListJSON mismatch: got %q want %q", got, want)
	}
}

func TestDecodeStringListJSON(t *testing.T) {
	got, err := DecodeStringListJSON(" [\"b\", \" a \", \"b\", \"\"] ")
	if err != nil {
		t.Fatalf("DecodeStringListJSON error: %v", err)
	}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeStringListJSON mismatch: got %#v want %#v", got, want)
	}
}
