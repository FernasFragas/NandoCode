// Package fileread implements the FileRead tool.
package fileread

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

// Input is the FileRead tool input.
type Input struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	LineLimit int    `json:"line_limit,omitempty"`
}

// PermissionTarget returns the file path for permission target matching.
func (i Input) PermissionTarget() string {
	return i.Path
}

// Output is the FileRead tool output.
type Output struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Truncated  bool   `json:"truncated"`
	SizeBytes  int64  `json:"size_bytes"`
	StartLine  int    `json:"start_line,omitempty"`
	LineCount  int    `json:"line_count,omitempty"`
	TotalLines int    `json:"total_lines,omitempty"`
	ReadBytes  int    `json:"read_bytes,omitempty"`
}

// NewFileReadTool creates a FileRead tool.
func NewFileReadTool() tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:             "FileRead",
		Description:      "Read UTF-8 text from an allowed file path using line ranges. Prefer start_line and line_limit for large files.",
		Aliases:          []string{"Read"},
		Schema:           schema(),
		Unmarshal:        unmarshalInput,
		IsReadOnlyFunc:   isInput,
		IsConcurrentFunc: isInput,
		IsDestructiveFunc: func(input any) bool {
			return false
		},
		CheckPermFunc: checkPermissions,
		CallFunc:      call,
		RenderFunc: func(input any, result tools.Result) tools.RenderHints {
			in, _ := input.(Input)
			return tools.RenderHints{Title: "FileRead", Summary: in.Path}
		},
	})
}

func schema() map[string]any {
	return tools.ObjectSchema(map[string]any{
		"path":       tools.StringProperty("Path to the text file to read."),
		"start_line": tools.IntegerProperty("Optional 1-based line to start reading from. Default: 1.", 1),
		"line_limit": tools.IntegerProperty("Optional maximum number of lines to read.", 1),
	}, []string{"path"})
}

func unmarshalInput(raw json.RawMessage) (any, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	if _, ok := fields["offset"]; ok {
		return nil, errors.New("offset is not supported; use start_line")
	}
	if _, ok := fields["limit"]; ok {
		return nil, errors.New("limit is not supported; use line_limit")
	}

	var input Input
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Path) == "" {
		return nil, errors.New("path is required")
	}
	if input.StartLine < 0 {
		return nil, errors.New("start_line must be non-negative")
	}
	if input.LineLimit < 0 {
		return nil, errors.New("line_limit must be non-negative")
	}
	return input, nil
}

func isInput(input any) bool {
	_, ok := input.(Input)
	return ok
}

func checkPermissions(ctx tools.Context, input any) tools.PermissionResult {
	if _, ok := input.(Input); !ok {
		return tools.PermissionResult{Decision: tools.PermDeny, Reason: "invalid FileRead input"}
	}
	return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
}

func call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	in, ok := input.(Input)
	if !ok {
		return tools.Result{}, errors.New("invalid FileRead input")
	}
	path, err := tools.ResolvePath(ctx, in.Path, tools.PathRead)
	if err != nil {
		return tools.Result{}, err
	}
	startLine := in.StartLine
	if startLine <= 0 {
		startLine = 1
	}
	lineLimit := in.LineLimit
	if lineLimit <= 0 {
		// Convert the existing max char budget into a conservative line cap.
		lineLimit = max(1, ctx.EffectiveMaxReadChars()/80)
	}
	res, err := ReadRange(ReadRangeRequest{
		Path:      path,
		StartLine: startLine,
		LineLimit: lineLimit,
		MaxBytes:  ctx.EffectiveMaxReadChars(),
	})
	if err != nil {
		return tools.Result{}, err
	}
	if ctx.ReadFileRangeSnapshot != nil {
		if _, ok := ctx.ReadFileRangeSnapshot(path, res.StartLine, res.LineCount, res.MTime.UnixNano()); ok {
			out := Output{
				Path:       path,
				Content:    "",
				Truncated:  res.Truncated,
				SizeBytes:  res.TotalBytes,
				StartLine:  res.StartLine,
				LineCount:  res.LineCount,
				TotalLines: res.TotalLines,
				ReadBytes:  0,
			}
			return tools.Result{
				Data:           out,
				Display:        formatAlreadyInContextDisplay(out),
				MaxResultChars: ctx.EffectiveMaxReadChars(),
			}, nil
		}
	}
	if ctx.RecordFileSnapshot != nil {
		ctx.RecordFileSnapshot(path, []byte(res.Content))
	}
	if ctx.RecordFileRangeSnapshot != nil {
		ctx.RecordFileRangeSnapshot(path, res.StartLine, res.LineCount, res.MTime.UnixNano(), []byte(res.Content))
	}
	out := Output{
		Path:       path,
		Content:    res.Content,
		Truncated:  res.Truncated,
		SizeBytes:  res.TotalBytes,
		StartLine:  res.StartLine,
		LineCount:  res.LineCount,
		TotalLines: res.TotalLines,
		ReadBytes:  res.ReadBytes,
	}
	return tools.Result{
		Data:           out,
		Display:        formatDisplay(out),
		MaxResultChars: ctx.EffectiveMaxReadChars(),
	}, nil
}

func formatDisplay(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "File: %s\n", out.Path)
	if out.LineCount > 0 {
		endLine := out.StartLine + out.LineCount - 1
		fmt.Fprintf(&b, "Range: lines %d-%d of %d\n", out.StartLine, endLine, out.TotalLines)
	} else {
		fmt.Fprintf(&b, "Range: lines %d-%d of %d\n", out.StartLine, out.StartLine, out.TotalLines)
	}
	if out.Truncated {
		fmt.Fprintf(&b, "Size: %d bytes (truncated)\n\n", out.SizeBytes)
	} else {
		fmt.Fprintf(&b, "Size: %d bytes\n\n", out.SizeBytes)
	}
	lines := strings.SplitAfter(out.Content, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		fmt.Fprintf(&b, "%5d  %s", out.StartLine+i, line)
	}
	return b.String()
}

func formatAlreadyInContextDisplay(out Output) string {
	var b strings.Builder
	endLine := out.StartLine
	if out.LineCount > 0 {
		endLine = out.StartLine + out.LineCount - 1
	}
	fmt.Fprintf(&b, "File: %s\n", out.Path)
	fmt.Fprintf(&b, "Range: lines %d-%d of %d\n", out.StartLine, endLine, out.TotalLines)
	b.WriteString("Notice: range unchanged and already present in context; returning compact result.\n")
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// readerFastPathThreshold is intentionally conservative. Files above this size
// are line-scanned using the streaming implementation.
const readerFastPathThreshold int64 = 10 * 1024 * 1024

func readLineRange(path string, startLine, lineLimit, maxBytes int) (lineRangeResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return lineRangeResult{}, err
	}
	if info.Size() > readerFastPathThreshold {
		return readLineRangeStreaming(path, startLine, lineLimit, maxBytes)
	}
	return readLineRangeFast(path, startLine, lineLimit, maxBytes)
}

func readLineRangeFast(path string, startLine, lineLimit, maxBytes int) (lineRangeResult, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return lineRangeResult{}, err
	}
	return selectLineRangeFromBytes(path, content, startLine, lineLimit, maxBytes)
}

func readLineRangeStreaming(path string, startLine, lineLimit, maxBytes int) (lineRangeResult, error) {
	// Stream and select ranges without loading the entire file into memory.
	f, err := os.Open(path)
	if err != nil {
		return lineRangeResult{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return lineRangeResult{}, err
	}
	return selectLineRangeFromReader(path, f, info.Size(), startLine, lineLimit, maxBytes)
}

type lineRangeResult struct {
	Content    string
	StartLine  int
	LineCount  int
	TotalLines int
	ReadBytes  int
	Truncated  bool
}

// basenameForErrors returns a user-facing path segment similar to existing messages.
func basenameForErrors(path string) string {
	return filepath.Base(path)
}
