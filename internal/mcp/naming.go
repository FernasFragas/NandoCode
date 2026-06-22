package mcp

import (
	"strings"
	"unicode"
)

func sanitizeName(s string) string {
	out := sanitizeNameNoFallback(s)
	if out == "" {
		return "unknown"
	}
	return out
}

func sanitizeNameNoFallback(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		isWord := unicode.IsLetter(r) || unicode.IsDigit(r)
		if isWord {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	return out
}

func toolName(server, tool string) string {
	return "mcp__" + sanitizeName(server) + "__" + sanitizeName(tool)
}
