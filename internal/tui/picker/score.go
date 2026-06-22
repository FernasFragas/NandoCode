package picker

import (
	"strings"
	"unicode"
)

func fuzzyScore(query, candidate string) (float64, []int, bool) {
	q := []rune(strings.ToLower(query))
	c := []rune(strings.ToLower(candidate))
	if len(q) == 0 {
		return 0, nil, true
	}

	matches := make([]int, 0, len(q))
	ci := 0
	for qi := 0; qi < len(q); qi++ {
		found := -1
		for ci < len(c) {
			if c[ci] == q[qi] {
				found = ci
				ci++
				break
			}
			ci++
		}
		if found < 0 {
			return 0, nil, false
		}
		matches = append(matches, found)
	}

	score := float64(len(matches) * 10)
	for i, idx := range matches {
		if isBoundary(candidate, idx) {
			score += 5
		}
		if i > 0 {
			gap := idx - matches[i-1] - 1
			if gap > 0 {
				score -= float64(gap)
			}
		}
	}
	base := basename(candidate)
	if strings.HasPrefix(strings.ToLower(base), strings.ToLower(query)) {
		score += 20
	}
	return score, matches, true
}

func basename(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 && idx+1 < len(path) {
		return path[idx+1:]
	}
	return path
}

func isBoundary(candidate string, idx int) bool {
	runes := []rune(candidate)
	if idx <= 0 {
		return true
	}
	prev := runes[idx-1]
	cur := runes[idx]
	if prev == '/' || prev == '_' || prev == '-' || prev == '.' {
		return true
	}
	if unicode.IsLower(prev) && unicode.IsUpper(cur) {
		return true
	}
	return false
}
