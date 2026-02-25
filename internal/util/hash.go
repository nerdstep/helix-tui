package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func HashBytesSHA256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func MarshalJSONAndHashSHA256Hex(v any) (string, []byte, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return "", nil, err
	}
	return HashBytesSHA256Hex(payload), payload, nil
}

func HashJSONSHA256Hex(v any) (string, error) {
	hash, _, err := MarshalJSONAndHashSHA256Hex(v)
	if err != nil {
		return "", err
	}
	return hash, nil
}

func HashJSONSHA256HexOrEmpty(v any) string {
	hash, err := HashJSONSHA256Hex(v)
	if err != nil {
		return ""
	}
	return hash
}
