package util

import (
	"encoding/json"
	"strings"
)

func EncodeStringListJSON(values []string) (string, error) {
	values = DedupeSortedStrings(values)
	if len(values) == 0 {
		return "[]", nil
	}
	body, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func DecodeStringListJSON(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return DedupeSortedStrings(values), nil
}
