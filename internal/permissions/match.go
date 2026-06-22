package permissions

import (
	"encoding/json"
	"path"
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
	target = normalizePathSeparators(target)
	argGlob = normalizePathSeparators(argGlob)

	// Match using the recursive glob matcher.
	return globMatch(argGlob, target)
}

// normalizePathSeparators converts Windows-style backslashes to forward slashes.
// This allows rules written on Unix to work on Windows and vice versa.
func normalizePathSeparators(s string) string {
	return filepath.ToSlash(s)
}

// globMatch matches a glob pattern against a target string.
// It handles * and ? as standard glob wildcards, and ** as a recursive directory matcher.
// Uses path.Match for non-** segments.
func globMatch(pattern, target string) bool {
	// If no ** segments, use path.Match directly.
	if !strings.Contains(pattern, "**") {
		ok, _ := path.Match(pattern, target)
		return ok
	}

	// Handle ** by splitting into segments and matching recursively.
	return globMatchRecursive(pattern, target)
}

// globMatchRecursive handles ** segments in glob patterns.
// It splits the pattern and target by "/" and handles ** as matching zero or more segments.
func globMatchRecursive(pattern, target string) bool {
	// Split by "/" but track whether the pattern/target end with "/" for edge cases.
	patternParts := strings.Split(pattern, "/")
	targetParts := strings.Split(target, "/")

	return matchParts(patternParts, targetParts)
}

// matchParts recursively matches pattern parts against target parts.
// Handles ** as matching zero or more directory segments.
func matchParts(patternParts, targetParts []string) bool {
	for len(patternParts) > 0 && len(targetParts) > 0 {
		p := patternParts[0]
		t := targetParts[0]

		if p == "**" {
			// ** can match zero or more segments.
			// Try matching the rest of the pattern starting from this position,
			// as well as consuming one target segment and trying again.

			// Try zero segments: skip ** in pattern and continue.
			if matchParts(patternParts[1:], targetParts) {
				return true
			}

			// Try one or more segments: consume a target segment and stay on **.
			if matchParts(patternParts, targetParts[1:]) {
				return true
			}

			return false
		}

		// Regular glob match for non-** segment.
		ok, _ := path.Match(p, t)
		if !ok {
			return false
		}

		patternParts = patternParts[1:]
		targetParts = targetParts[1:]
	}

	// If there are remaining pattern parts, check if they're all **.
	for _, p := range patternParts {
		if p != "**" && p != "" {
			return false
		}
	}

	// Both must be exhausted.
	return len(patternParts) == 0 && len(targetParts) == 0
}
