package util

import (
	"encoding/json"
	"testing"
)

func TestHashBytesSHA256Hex(t *testing.T) {
	got := HashBytesSHA256Hex([]byte("abc"))
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("HashBytesSHA256Hex mismatch: got %q want %q", got, want)
	}
}

func TestMarshalJSONAndHashSHA256Hex(t *testing.T) {
	input := struct {
		A string `json:"a"`
		B int    `json:"b"`
	}{A: "x", B: 2}
	hash, payload, err := MarshalJSONAndHashSHA256Hex(input)
	if err != nil {
		t.Fatalf("MarshalJSONAndHashSHA256Hex error: %v", err)
	}
	if len(payload) == 0 {
		t.Fatalf("MarshalJSONAndHashSHA256Hex returned empty payload")
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if hash != HashBytesSHA256Hex(payload) {
		t.Fatalf("hash mismatch with payload")
	}
}

func TestHashJSONSHA256HexOrEmpty(t *testing.T) {
	got := HashJSONSHA256HexOrEmpty(map[string]string{"x": "y"})
	if got == "" {
		t.Fatalf("HashJSONSHA256HexOrEmpty returned empty hash for valid JSON")
	}
}
