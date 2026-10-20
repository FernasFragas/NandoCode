package permissions

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// PermissionTargeter is an optional interface that tool inputs can implement
// to provide the permission target string for rule matching.
type PermissionTargeter interface {
	PermissionTarget() string
}

// ExtractTarget extracts the permission target string from a parsed tool input.
// It tries three strategies in order:
// 1. Call PermissionTarget() if the input implements the interface.
// 2. Extract from struct fields Path, Command, or Query (in that order).
// 3. Fall back to compact JSON representation.
func ExtractTarget(input any) string {
	if pt, ok := input.(PermissionTargeter); ok {
		return pt.PermissionTarget()
	}

	// Try struct field introspection.
	if v, ok := input.(interface{ Path() string }); ok {
		if path := v.Path(); path != "" {
			return path
		}
	}
	if v, ok := input.(interface{ Command() string }); ok {
		if cmd := v.Command(); cmd != "" {
			return cmd
		}
	}
	if v, ok := input.(interface{ Query() string }); ok {
		if query := v.Query(); query != "" {
			return query
		}
	}

	// For generic struct types, try to find Path, Command, or Query fields.
	// This uses reflection to handle inputs that don't have methods but have struct fields.
	type fieldStruct struct {
		Path    string `json:"path"`
		Command string `json:"command"`
		Query   string `json:"query"`
	}
	if data, err := json.Marshal(input); err == nil {
		var fs fieldStruct
		if err := json.Unmarshal(data, &fs); err == nil {
			if fs.Path != "" {
				return fs.Path
			}
			if fs.Command != "" {
				return fs.Command
			}
			if fs.Query != "" {
				return fs.Query
			}
		}
		// Fall back to compact JSON if no fields matched.
		return string(data)
	}

	return ""
}

// ParsePattern parses a rule pattern in the form "ToolName(arg-glob)".
// Returns (toolName, argGlob, ok) where ok is false if the pattern is malformed.
func ParsePattern(pattern string) (string, string, bool) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", "", false
	}

	// Find the opening parenthesis.
	openIdx := strings.IndexByte(pattern, '(')
	if openIdx == -1 {
		return "", "", false
	}

	toolName := strings.TrimSpace(pattern[:openIdx])
	if toolName == "" {
		return "", "", false
	}

	// Find the closing parenthesis at the end.
	if !strings.HasSuffix(pattern, ")") {
		return "", "", false
	}

	argGlob := pattern[openIdx+1 : len(pattern)-1]
	if argGlob == "" {
		// Empty glob is invalid.
		return "", "", false
	}

	// Verify there are no trailing characters after the closing paren.
	// (We already checked HasSuffix, so we're good.)

	return toolName, argGlob, true
}

// matches checks if a pattern matches the given tool name and target string.
// Returns false if the pattern is malformed or tool name doesn't match exactly.
func matches(pattern, toolName, target string) bool {
	pToolName, argGlob, ok := ParsePattern(pattern)
	if !ok {
		return false
	}

	// Tool name is case-sensitive and must match exactly.
	if pToolName != toolName {
		return false
	}

	// Normalize path separators for consistent matching.
	// Only the target is normalized. The rule glob is left alone because "\\"
	// is its escape character; rewriting it to "/" would break escapes on
	// Windows. Windows users write "/" in rules.
	target = normalizePathSeparators(target)

	// Match using the recursive glob matcher.
	return globMatch(argGlob, target)
}

// normalizePathSeparators converts Windows-style backslashes to forward slashes.
// This allows rules written on Unix to work on Windows and vice versa.
func normalizePathSeparators(s string) string {
	return filepath.ToSlash(s)
}

// globMatch matches a glob pattern against a target string.
//
// Semantics (decided 2026-10-06):
//   - "*" matches any run of characters, including "/" and the empty run.
//   - "?" matches exactly one character (including "/").
//   - "[abc]", "[a-z]" and "[!abc]" / "[^abc]" match one character from the class.
//   - "\x" matches the literal character x.
//   - "**" is the same as "*", with one addition: a "**" that forms a whole
//     path segment also matches zero directories, so "src/**/x.go" matches
//     "src/x.go", "**/x.go" matches "x.go", and "docs/**" matches "docs".
func globMatch(pattern, target string) bool {
	for _, variant := range doubleStarVariants(pattern) {
		if wildcardMatch([]rune(variant), []rune(target)) {
			return true
		}
	}
	return false
}

// maxDoubleStarSegments caps the 2^n expansion of "**" path segments.
const maxDoubleStarSegments = 12

// doubleStarVariants returns the pattern itself plus the forms where each
// whole-segment "**" has been collapsed away (matching zero directories).
func doubleStarVariants(pattern string) []string {
	if !strings.Contains(pattern, "**") {
		return []string{pattern}
	}
	segs := strings.Split(pattern, "/")
	// Adjacent "**" segments are equivalent to one.
	dedup := segs[:0:0]
	for _, seg := range segs {
		if seg == "**" && len(dedup) > 0 && dedup[len(dedup)-1] == "**" {
			continue
		}
		dedup = append(dedup, seg)
	}
	segs = dedup
	var starIdx []int
	for i, seg := range segs {
		if seg == "**" {
			starIdx = append(starIdx, i)
		}
	}
	if len(starIdx) == 0 {
		return []string{pattern}
	}
	if len(starIdx) > maxDoubleStarSegments {
		starIdx = starIdx[:maxDoubleStarSegments]
	}
	variants := make([]string, 0, 1<<len(starIdx))
	for mask := 0; mask < 1<<len(starIdx); mask++ {
		drop := map[int]bool{}
		for bit, idx := range starIdx {
			if mask&(1<<bit) != 0 {
				drop[idx] = true
			}
		}
		kept := make([]string, 0, len(segs))
		for i, seg := range segs {
			if !drop[i] {
				kept = append(kept, seg)
			}
		}
		variants = append(variants, strings.Join(kept, "/"))
	}
	return variants
}

// wildcardMatch matches p against s with "*", "?", classes and escapes.
// It uses the iterative backtracking algorithm, so it is O(len(p)*len(s)) at worst.
func wildcardMatch(p, s []rune) bool {
	pi, si := 0, 0
	star, mark := -1, 0
	for si < len(s) {
		if pi < len(p) && p[pi] == '*' {
			star, mark = pi, si
			pi++
			continue
		}
		if pi < len(p) {
			if next, ok := matchOne(p, pi, s[si]); ok {
				pi = next
				si++
				continue
			}
		}
		if star >= 0 {
			mark++
			si = mark
			pi = star + 1
			continue
		}
		return false
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

// matchOne matches the single pattern element at p[pi] against c and returns
// the index just past that element. ok is false when it does not match.
func matchOne(p []rune, pi int, c rune) (int, bool) {
	switch p[pi] {
	case '?':
		return pi + 1, true
	case '\\':
		if pi+1 < len(p) {
			return pi + 2, p[pi+1] == c
		}
		return pi + 1, c == '\\'
	case '[':
		if next, matched, valid := matchClass(p, pi, c); valid {
			return next, matched
		}
		return pi + 1, c == '['
	default:
		return pi + 1, p[pi] == c
	}
}

// matchClass parses the class starting at p[pi] == '['. valid is false when the
// class is not terminated, in which case '[' is treated as a literal.
func matchClass(p []rune, pi int, c rune) (next int, matched, valid bool) {
	i := pi + 1
	negate := false
	if i < len(p) && (p[i] == '!' || p[i] == '^') {
		negate = true
		i++
	}
	first := true
	for i < len(p) {
		if p[i] == ']' && !first {
			return i + 1, matched != negate, true
		}
		first = false
		lo := p[i]
		if lo == '\\' && i+1 < len(p) {
			i++
			lo = p[i]
		}
		hi := lo
		if i+2 < len(p) && p[i+1] == '-' && p[i+2] != ']' {
			hi = p[i+2]
			i += 2
		}
		if lo <= c && c <= hi {
			matched = true
		}
		i++
	}
	return 0, false, false
}
