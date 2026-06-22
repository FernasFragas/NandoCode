package hooks

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/permissions"
)

func matchesHook(h Hook, env Envelope) bool {
	if h.Event != env.Event {
		return false
	}
	if strings.TrimSpace(h.Matcher) == "" {
		return true
	}
	if env.Tool == nil {
		return false
	}
	tool, glob, ok := permissions.ParsePattern(h.Matcher)
	if !ok {
		return h.Matcher == env.Tool.Name
	}
	if tool != env.Tool.Name {
		return false
	}
	target := filepath.ToSlash(env.Tool.Target)
	glob = filepath.ToSlash(glob)
	return wildcardMatch(glob, target)
}

func matchingHooks(snapshot Snapshot, env Envelope) []Hook {
	var matched []Hook
	for _, h := range snapshot.Hooks {
		if matchesHook(h, env) {
			matched = append(matched, h)
		}
	}
	return matched
}

func wildcardMatch(pattern, target string) bool {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	ok, _ := regexp.MatchString(b.String(), target)
	return ok
}
