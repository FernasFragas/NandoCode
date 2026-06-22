package mentions

import "strings"

// ParseLineRangeToken parses @file#Lstart-Lend mention suffixes.
// Supported syntax is intentionally strict: #L10-L20 only.
func ParseLineRangeToken(token string) (string, int, int, bool) {
	idx := strings.LastIndex(token, "#L")
	if idx <= 0 {
		return "", 0, 0, false
	}
	path := token[:idx]
	rangePart := token[idx+2:]
	parts := strings.Split(rangePart, "-L")
	if len(parts) != 2 {
		return "", 0, 0, false
	}
	start := strings.TrimSpace(parts[0])
	end := strings.TrimSpace(parts[1])
	if start == "" || end == "" {
		return "", 0, 0, false
	}
	startVal := 0
	endVal := 0
	for _, c := range start {
		if c < '0' || c > '9' {
			return "", 0, 0, false
		}
		startVal = startVal*10 + int(c-'0')
	}
	for _, c := range end {
		if c < '0' || c > '9' {
			return "", 0, 0, false
		}
		endVal = endVal*10 + int(c-'0')
	}
	if startVal <= 0 || endVal < startVal {
		return "", 0, 0, false
	}
	return path, startVal, endVal, true
}
