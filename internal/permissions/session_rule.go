package permissions

import "strings"

// SessionAllowRule builds the session-scoped "always allow" rule for one
// approved tool call: it matches exactly that tool and target, nothing wider.
// Glob metacharacters in the target are escaped so a target such as "rm -rf *"
// is matched literally. The target uses "/" separators, as matching does, so
// a Windows path matches itself. It reports false when the tool name or target is empty
// (ParsePattern rejects empty globs), so no rule is created.
//
// The TUI and the browser server share this so both surfaces mean the same
// thing by "Always Allow".
func SessionAllowRule(toolName, target string) (Rule, bool) {
	toolName = strings.TrimSpace(toolName)
	target = strings.TrimSpace(target)
	if toolName == "" || target == "" || strings.ContainsAny(toolName, "()") {
		return Rule{}, false
	}
	return Rule{Pattern: toolName + "(" + escapeGlob(normalizePathSeparators(target)) + ")", Source: SourceSession}, true
}

// escapeGlob backslash-escapes the metacharacters understood by path.Match.
func escapeGlob(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '*', '?', '[', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
