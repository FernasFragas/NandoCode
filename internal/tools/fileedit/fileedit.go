// Package fileedit implements the FileEdit tool.
package fileedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/filewrite"
)

// Input is the FileEdit tool input.
type Input struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// PermissionTarget returns the file path for permission target matching.
func (i Input) PermissionTarget() string {
	return i.Path
}

// Output is the FileEdit tool output.
type Output struct {
	Path                string `json:"path"`
	OccurrencesReplaced int    `json:"occurrences_replaced"`
	BytesBefore         int    `json:"bytes_before"`
	BytesAfter          int    `json:"bytes_after"`
}

// NewFileEditTool creates a FileEdit tool.
func NewFileEditTool() tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:        "FileEdit",
		Description: "Edit an existing file by replacing old_string with new_string.",
		Aliases:     []string{"Edit"},
		Schema:      schema(),
		Unmarshal:   unmarshalInput,
		IsReadOnlyFunc: func(input any) bool {
			return false
		},
		IsConcurrentFunc: func(input any) bool {
			return false
		},
		IsDestructiveFunc: func(input any) bool {
			_, ok := input.(Input)
			return ok
		},
		CheckPermFunc: checkPermissions,
		CallFunc:      call,
		RenderFunc: func(input any, result tools.Result) tools.RenderHints {
			in, _ := input.(Input)
			return tools.RenderHints{Title: "FileEdit", Summary: in.Path}
		},
	})
}

func schema() map[string]any {
	return tools.ObjectSchema(map[string]any{
		"path":        tools.StringProperty("Path to the file to edit."),
		"old_string":  tools.StringProperty("The exact text to replace."),
		"new_string":  tools.StringProperty("The replacement text."),
		"replace_all": map[string]any{"type": "boolean", "description": "Replace all occurrences instead of requiring uniqueness."},
	}, []string{"path", "old_string", "new_string"})
}

func unmarshalInput(raw json.RawMessage) (any, error) {
	var input Input
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Path) == "" {
		return nil, errors.New("path is required")
	}
	if input.OldString == "" {
		return nil, errors.New("old_string is required")
	}
	return input, nil
}

func checkPermissions(ctx tools.Context, input any) tools.PermissionResult {
	if _, ok := input.(Input); !ok {
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "invalid FileEdit input"}
	}
	switch ctx.PermissionMode {
	case tools.PermissionBypassPermissions:
		return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
	case tools.PermissionPlan, tools.PermissionDontAsk:
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "FileEdit modifies files"}
	default:
		return tools.PermissionResult{Decision: tools.PermAsk, Reason: "FileEdit modifies files", UpdatedInput: input}
	}
}

func call(ctx tools.Context, input any, _ chan<- tools.ProgressEvent) (tools.Result, error) {
	in, ok := input.(Input)
	if !ok {
		return tools.Result{}, errors.New("invalid FileEdit input")
	}
	if in.OldString == in.NewString {
		return tools.Result{}, errors.New("FileEdit: old_string and new_string are identical; no edit needed")
	}

	path, err := tools.ResolvePath(ctx, in.Path, tools.PathRead)
	if err != nil {
		return tools.Result{}, err
	}

	rawContent, err := os.ReadFile(path)
	if err != nil {
		return tools.Result{}, err
	}

	// Staleness check: compare current bytes to the last snapshot.
	if ctx.ReadFileSnapshot != nil {
		if snap, ok := ctx.ReadFileSnapshot(path); ok {
			if !bytes.Equal(snap, rawContent) {
				return tools.Result{}, errors.New("FileEdit: staleness detected — file has changed since last read; re-read the file before editing")
			}
		}
	}

	// Normalize both file content and old_string for matching.
	fileStr := normalizeLineEndings(string(rawContent))
	normalOld := normalizeForMatch(in.OldString)
	normalFile := normalizeForMatch(fileStr)

	count := countOccurrences(normalFile, normalOld)
	if count == 0 {
		return tools.Result{}, errors.New("FileEdit: old_string not found in file; check the exact content with FileRead first")
	}
	if count > 1 && !in.ReplaceAll {
		return tools.Result{}, fmt.Errorf("FileEdit: old_string is not unique (%d occurrences); use replace_all=true or provide more context", count)
	}

	// Apply replacement to normalized file content.
	var newStr string
	var replaced int
	if in.ReplaceAll {
		newStr = strings.ReplaceAll(normalFile, normalOld, in.NewString)
		replaced = count
	} else {
		newStr = strings.Replace(normalFile, normalOld, in.NewString, 1)
		replaced = 1
	}

	newBytes := []byte(newStr)
	if err := filewrite.AtomicWrite(path, newBytes, 0o644); err != nil {
		return tools.Result{}, err
	}
	if ctx.RecordFileSnapshot != nil {
		ctx.RecordFileSnapshot(path, newBytes)
	}

	diff := buildDiffDisplay(in.Path, normalFile, newStr)
	out := Output{
		Path:                path,
		OccurrencesReplaced: replaced,
		BytesBefore:         len(rawContent),
		BytesAfter:          len(newBytes),
	}
	return tools.Result{Data: out, Display: diff}, nil
}

// normalizeLineEndings converts CRLF to LF.
func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// normalizeForMatch trims trailing whitespace from each line.
func normalizeForMatch(s string) string {
	s = normalizeLineEndings(s)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// countOccurrences counts non-overlapping occurrences of needle in haystack.
func countOccurrences(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	return strings.Count(haystack, needle)
}

// buildDiffDisplay produces a minimal unified-diff-style output.
func buildDiffDisplay(path, before, after string) string {
	if before == after {
		return fmt.Sprintf("--- %s\n+++ %s\n(no changes)\n", path, path)
	}
	oldLines := strings.Split(before, "\n")
	newLines := strings.Split(after, "\n")

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", path, path)

	// Simple line-by-line diff: find first and last differing line.
	minLen := len(oldLines)
	if len(newLines) < minLen {
		minLen = len(newLines)
	}
	firstDiff := -1
	for i := 0; i < minLen; i++ {
		if oldLines[i] != newLines[i] {
			firstDiff = i
			break
		}
	}
	if firstDiff == -1 {
		// Lines added at end
		firstDiff = minLen
	}

	lastOld := len(oldLines) - 1
	lastNew := len(newLines) - 1
	for lastOld > firstDiff && lastNew > firstDiff && oldLines[lastOld] == newLines[lastNew] {
		lastOld--
		lastNew--
	}

	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", firstDiff+1, lastOld-firstDiff+1, firstDiff+1, lastNew-firstDiff+1)
	for i := firstDiff; i <= lastOld; i++ {
		fmt.Fprintf(&b, "-%s\n", oldLines[i])
	}
	for i := firstDiff; i <= lastNew; i++ {
		fmt.Fprintf(&b, "+%s\n", newLines[i])
	}
	return b.String()
}
