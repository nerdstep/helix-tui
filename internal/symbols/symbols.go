package symbols

import "strings"

func Normalize(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, symbol := range raw {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" {
			continue
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		out = append(out, symbol)
	}
	return out
}

func Merge(lists ...[]string) []string {
	total := 0
	for _, list := range lists {
		total += len(list)
	}
	out := make([]string, 0, total)
	seen := map[string]struct{}{}
	for _, list := range lists {
		for _, symbol := range list {
			symbol = strings.ToUpper(strings.TrimSpace(symbol))
			if symbol == "" {
				continue
			}
			if _, ok := seen[symbol]; ok {
				continue
			}
			seen[symbol] = struct{}{}
			out = append(out, symbol)
		}
	}
	return out
}
