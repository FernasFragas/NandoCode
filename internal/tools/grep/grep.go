// Package grep implements the Grep tool.
package grep

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

const (
	defaultHeadLimit = 250
	maxContextLines  = 5
	maxFileSizeBytes = 10 * 1024 * 1024 // 10 MB
	binaryCheckBytes = 512
	defaultMaxResult = 50_000
)

var defaultExcludes = []string{".git", "node_modules", ".svn", "vendor", "dist"}

// Input is the Grep tool input.
type Input struct {
	Pattern      string `json:"pattern"`
	Path         string `json:"path,omitempty"`
	Include      string `json:"include,omitempty"`
	Exclude      string `json:"exclude,omitempty"`
	ContextLines int    `json:"context_lines,omitempty"`
	HeadLimit    int    `json:"head_limit,omitempty"`
}

// Match is a single grep result line.
type Match struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Content   string `json:"content"`
	IsContext bool   `json:"is_context,omitempty"`
}

// Output is the Grep tool output.
type Output struct {
	Matches      []Match `json:"matches"`
	AppliedLimit bool    `json:"applied_limit"`
	HeadLimit    int     `json:"head_limit"`
}

// NewGrepTool creates a Grep tool.
func NewGrepTool() tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:        "Grep",
		Description: "Search for a regex pattern in files, returning matching lines with optional context.",
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
				return tools.RenderHints{Title: "Grep", Summary: fmt.Sprintf("%s (%d matches)", in.Pattern, len(out.Matches))}
			}
			return tools.RenderHints{Title: "Grep", Summary: in.Pattern}
		},
	})
}

func schema() map[string]any {
	return tools.ObjectSchema(map[string]any{
		"pattern":       tools.StringProperty("Regex pattern to search for."),
		"path":          tools.StringProperty("Directory or file to search. Defaults to working directory."),
		"include":       tools.StringProperty("Glob pattern to restrict which files are searched, e.g. *.go"),
		"exclude":       tools.StringProperty("Glob pattern to skip files, e.g. *_test.go"),
		"context_lines": tools.IntegerProperty("Number of context lines before and after each match (0-5).", 0),
		"head_limit":    tools.IntegerProperty("Maximum number of match lines to return. Default 250.", 1),
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
		return tools.Result{}, errors.New("invalid Grep input")
	}

	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return tools.Result{}, fmt.Errorf("invalid regex pattern: %w", err)
	}

	headLimit := in.HeadLimit
	if headLimit <= 0 {
		headLimit = defaultHeadLimit
	}
	ctxLines := in.ContextLines
	if ctxLines < 0 {
		ctxLines = 0
	}
	if ctxLines > maxContextLines {
		ctxLines = maxContextLines
	}

	searchPath := strings.TrimSpace(in.Path)
	if searchPath == "" {
		searchPath = ctx.WorkingDir
	}
	resolvedPath, err := tools.ResolvePath(ctx, searchPath, tools.PathRead)
	if err != nil {
		return tools.Result{}, err
	}

	var matches []Match
	appliedLimit := false

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return tools.Result{}, err
	}

	if info.IsDir() {
		err = filepath.WalkDir(resolvedPath, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return nil
			}
			rel, _ := filepath.Rel(resolvedPath, p)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if rel != "." && shouldExcludeDir(rel) {
					return filepath.SkipDir
				}
				return nil
			}
			if !matchesInclude(d.Name(), in.Include) {
				return nil
			}
			if matchesExclude(d.Name(), in.Exclude) {
				return nil
			}
			if isBinaryFile(p) {
				return nil
			}
			relFile, _ := filepath.Rel(resolvedPath, p)
			count := len(matches)
			searchFileInto(p, filepath.ToSlash(relFile), re, ctxLines, headLimit, &matches)
			if len(matches) > count && len(matches) >= headLimit {
				appliedLimit = true
				return errors.New("stop")
			}
			return nil
		})
		if err != nil && err.Error() != "stop" {
			return tools.Result{}, err
		}
	} else {
		if !isBinaryFile(resolvedPath) {
			searchFileInto(resolvedPath, filepath.Base(resolvedPath), re, ctxLines, headLimit, &matches)
			if len(matches) >= headLimit {
				appliedLimit = true
			}
		}
	}

	// Apply head_limit to matches count
	if len(matches) > headLimit {
		matches = matches[:headLimit]
		appliedLimit = true
	}

	out := Output{
		Matches:      matches,
		AppliedLimit: appliedLimit,
		HeadLimit:    headLimit,
	}

	maxChars := ctx.EffectiveMaxResultChars()
	if maxChars < defaultMaxResult {
		maxChars = defaultMaxResult
	}

	display := buildDisplay(out)
	if len(display) > maxChars {
		display = display[:maxChars] + "\n[truncated]\n"
	}
	return tools.Result{Data: out, Display: display}, nil
}

// searchFileInto appends matches from path into matches slice.
func searchFileInto(path, relFile string, re *regexp.Regexp, ctxLines, headLimit int, matches *[]Match) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() > maxFileSizeBytes {
		return
	}

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	type matchPos struct{ lineIdx int }
	var matchPositions []matchPos
	for i, line := range lines {
		if re.MatchString(line) {
			matchPositions = append(matchPositions, matchPos{i})
		}
	}

	// Build output with context, deduplicating lines
	emitted := make(map[int]bool)
	matchCount := 0
	for _, mp := range matchPositions {
		if matchCount >= headLimit {
			break
		}
		start := mp.lineIdx - ctxLines
		if start < 0 {
			start = 0
		}
		end := mp.lineIdx + ctxLines
		if end >= len(lines) {
			end = len(lines) - 1
		}
		for i := start; i <= end; i++ {
			if !emitted[i] {
				emitted[i] = true
				*matches = append(*matches, Match{
					File:      relFile,
					Line:      i + 1,
					Content:   lines[i],
					IsContext: i != mp.lineIdx,
				})
			}
		}
		matchCount++
	}
}

// isBinaryFile checks the first 512 bytes for null bytes.
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, binaryCheckBytes)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false
	}
	return bytes.ContainsRune(buf[:n], 0)
}

// matchesInclude returns true if filename matches the include pattern (or no pattern set).
func matchesInclude(filename, include string) bool {
	if include == "" {
		return true
	}
	matched, _ := filepath.Match(include, filename)
	return matched
}

// matchesExclude returns true if filename matches the exclude pattern.
func matchesExclude(filename, exclude string) bool {
	if exclude == "" {
		return false
	}
	matched, _ := filepath.Match(exclude, filename)
	return matched
}

// shouldExcludeDir returns true if the relative dir path starts with an excluded name.
func shouldExcludeDir(relPath string) bool {
	first := firstComponent(relPath)
	for _, exc := range defaultExcludes {
		if first == exc {
			return true
		}
	}
	return false
}

func firstComponent(p string) string {
	parts := strings.SplitN(p, "/", 2)
	return parts[0]
}

func buildDisplay(out Output) string {
	if len(out.Matches) == 0 {
		return "No matches found.\n"
	}
	var b strings.Builder
	for _, m := range out.Matches {
		if m.IsContext {
			fmt.Fprintf(&b, "%s:%d:%s\n", m.File, m.Line, m.Content)
		} else {
			fmt.Fprintf(&b, "%s:%d:%s\n", m.File, m.Line, m.Content)
		}
	}
	if out.AppliedLimit {
		fmt.Fprintf(&b, "[%d matches; head_limit=%d applied]\n", len(out.Matches), out.HeadLimit)
	} else {
		fmt.Fprintf(&b, "[%d matches]\n", len(out.Matches))
	}
	return b.String()
}
