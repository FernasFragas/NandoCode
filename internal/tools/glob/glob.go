// Package glob implements the Glob tool.
package glob

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

const maxFiles = 1000

// defaultExcludes lists top-level directory names that are always skipped.
var defaultExcludes = []string{".git", "node_modules", ".svn", "vendor", "dist"}

// Input is the Glob tool input.
type Input struct {
	Pattern  string `json:"pattern"`
	BasePath string `json:"base_path,omitempty"`
}

// Output is the Glob tool output.
type Output struct {
	Paths        []string `json:"paths"`
	TotalMatched int      `json:"total_matched"`
	Truncated    bool     `json:"truncated"`
}

// NewGlobTool creates a Glob tool.
func NewGlobTool() tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:        "Glob",
		Description: "Find files matching a glob pattern, including ** recursive patterns.",
		Schema:      schema(),
		Unmarshal:   unmarshalInput,
		IsReadOnlyFunc: func(input any) bool {
			return true
		},
		IsConcurrentFunc: func(input any) bool {
			return true
		},
		IsDestructiveFunc: func(input any) bool {
			return false
		},
		CheckPermFunc: func(ctx tools.Context, input any) tools.PermissionResult {
			return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
		},
		CallFunc: call,
		RenderFunc: func(input any, result tools.Result) tools.RenderHints {
			in, _ := input.(Input)
			if out, ok := result.Data.(Output); ok {
				return tools.RenderHints{Title: "Glob", Summary: fmt.Sprintf("%s (%d files)", in.Pattern, out.TotalMatched)}
			}
			return tools.RenderHints{Title: "Glob", Summary: in.Pattern}
		},
	})
}

func schema() map[string]any {
	return tools.ObjectSchema(map[string]any{
		"pattern":   tools.StringProperty("Glob pattern to match, e.g. **/*.go"),
		"base_path": tools.StringProperty("Optional base directory to search in. Defaults to working directory."),
	}, []string{"pattern"})
}

func unmarshalInput(raw json.RawMessage) (any, error) {
	var input Input
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Pattern) == "" {
		return nil, errors.New("pattern is required")
	}
	return input, nil
}

func call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in, ok := input.(Input)
	if !ok {
		return tools.Result{}, errors.New("invalid Glob input")
	}
	if strings.TrimSpace(in.Pattern) == "" {
		return tools.Result{}, errors.New("pattern is required")
	}

	basePath := strings.TrimSpace(in.BasePath)
	if basePath == "" {
		basePath = ctx.WorkingDir
	}
	resolvedBase, err := tools.ResolvePath(ctx, basePath, tools.PathRead)
	if err != nil {
		return tools.Result{}, err
	}

	var paths []string
	if strings.Contains(in.Pattern, "**") {
		paths, err = walkGlob(resolvedBase, in.Pattern)
	} else {
		paths, err = simpleGlob(resolvedBase, in.Pattern)
	}
	if err != nil {
		return tools.Result{}, err
	}

	// Make paths relative to base
	for i, p := range paths {
		if rel, relErr := filepath.Rel(resolvedBase, p); relErr == nil {
			paths[i] = filepath.ToSlash(rel)
		}
	}

	sort.Strings(paths)

	total := len(paths)
	truncated := false
	if total > maxFiles {
		paths = paths[:maxFiles]
		truncated = true
	}

	out := Output{
		Paths:        paths,
		TotalMatched: total,
		Truncated:    truncated,
	}
	return tools.Result{Data: out, Display: buildDisplay(out)}, nil
}

func walkGlob(base, pattern string) ([]string, error) {
	re, err := patternToRegexp(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern: %w", err)
	}
	var matches []string
	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(base, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && shouldExclude(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if re.MatchString(rel) {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

func simpleGlob(base, pattern string) ([]string, error) {
	globPat := filepath.Join(base, pattern)
	raw, err := filepath.Glob(globPat)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, m := range raw {
		rel, relErr := filepath.Rel(base, m)
		if relErr != nil {
			continue
		}
		if shouldExclude(filepath.ToSlash(rel)) {
			continue
		}
		info, err := os.Stat(m)
		if err != nil || info.IsDir() {
			continue
		}
		result = append(result, m)
	}
	return result, nil
}

// patternToRegexp converts a glob pattern (with **) to a regexp.
// ** matches any sequence of characters including path separators.
// * matches any sequence of non-separator characters.
// ? matches any single non-separator character.
func patternToRegexp(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(pattern)
	var b strings.Builder
	b.WriteString("^")
	i := 0
	for i < len(pattern) {
		if i+1 < len(pattern) && pattern[i] == '*' && pattern[i+1] == '*' {
			// ** — consume optional trailing slash
			b.WriteString(".*")
			i += 2
			if i < len(pattern) && pattern[i] == '/' {
				i++
				b.WriteString("/?")
			}
			continue
		}
		switch pattern[i] {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '{', '}', '[', ']', '^', '$', '|', '\\':
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
		default:
			b.WriteByte(pattern[i])
		}
		i++
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// shouldExclude returns true if the relative path starts with an excluded directory name.
func shouldExclude(relPath string) bool {
	first := firstComponent(relPath)
	for _, exc := range defaultExcludes {
		if first == exc {
			return true
		}
	}
	return false
}

// firstComponent returns the first path segment of a slash-separated relative path.
func firstComponent(p string) string {
	parts := strings.SplitN(p, "/", 2)
	return parts[0]
}

func buildDisplay(out Output) string {
	if len(out.Paths) == 0 {
		return "No files matched.\n"
	}
	var b strings.Builder
	for _, p := range out.Paths {
		fmt.Fprintln(&b, p)
	}
	if out.Truncated {
		fmt.Fprintf(&b, "[truncated: showing %d of %d matches]\n", maxFiles, out.TotalMatched)
	} else {
		fmt.Fprintf(&b, "%d files matched.\n", out.TotalMatched)
	}
	return b.String()
}
