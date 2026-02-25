package util

import (
	"sort"
	"strings"
)

func DedupeSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func RemoveOverlappingStrings(keep, filtered []string) []string {
	if len(keep) == 0 || len(filtered) == 0 {
		return filtered
	}
	keepSet := make(map[string]struct{}, len(keep))
	for _, value := range keep {
		keepSet[value] = struct{}{}
	}
	out := make([]string, 0, len(filtered))
	for _, value := range filtered {
		if _, exists := keepSet[value]; exists {
			continue
		}
		out = append(out, value)
	}
	return out
}
