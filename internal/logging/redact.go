package logging

import (
	"regexp"
	"strings"
)

var (
	skTokenRE     = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{6,}\b`)
	bearerTokenRE = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._-]{8,}\b`)
	apiKeyRE      = regexp.MustCompile(`(?i)\b(ollama_api_key|api_key)\b\s*[:=]\s*([^\s]+)`)
)

// Redact removes known secret patterns from text.
func Redact(in string) string {
	s := skTokenRE.ReplaceAllStringFunc(in, func(tok string) string {
		if strings.HasPrefix(tok, "sk-") {
			return "sk-***"
		}
		return "***"
	})
	s = bearerTokenRE.ReplaceAllString(s, "Bearer ***")
	s = apiKeyRE.ReplaceAllString(s, "$1=***")
	for _, key := range []string{"TOKEN", "OLLAMA_API_KEY", "API_KEY", "OLLAMA_API_KEY", "OLLAMA_APIKEY"} {
		s = redactAssignment(s, key)
	}
	return s
}

func redactAssignment(in, key string) string {
	u := strings.ToUpper(in)
	marker := key + "="
	idx := strings.Index(u, marker)
	if idx < 0 {
		return in
	}
	start := idx + len(marker)
	end := start
	for end < len(in) {
		switch in[end] {
		case ' ', '\n', '\t', '\r':
			return in[:start] + "***" + in[end:]
		default:
			end++
		}
	}
	return in[:start] + "***"
}
