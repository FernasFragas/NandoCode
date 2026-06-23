package fileread

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

func TestFileReadToolReadsText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewFileReadTool()
	result, err := tool.Call(tools.DefaultContext(context.Background(), dir), Input{Path: "readme.txt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.Content != "hello\nworld" {
		t.Fatalf("Content = %q", out.Content)
	}
	if out.StartLine != 1 || out.LineCount != 2 || out.TotalLines != 2 {
		t.Fatalf("unexpected range metadata: %#v", out)
	}
	if !strings.Contains(result.Display, "hello") {
		t.Fatalf("Display = %q", result.Display)
	}
}

func TestFileReadToolTruncates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), dir)
	ctx.MaxReadChars = 3
	result, err := NewFileReadTool().Call(ctx, Input{Path: "big.txt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.Content != "abc" || !out.Truncated {
		t.Fatalf("out = %#v", out)
	}
}

func TestFileReadToolLineRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("a\nb\nc\nd\ne\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewFileReadTool().Call(tools.DefaultContext(context.Background(), dir), Input{Path: "data.txt", StartLine: 3, LineLimit: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.Content != "c\nd" || !out.Truncated {
		t.Fatalf("out = %#v", out)
	}
	if out.StartLine != 3 || out.LineCount != 2 || out.TotalLines != 5 {
		t.Fatalf("unexpected range metadata: %#v", out)
	}
	if !strings.Contains(result.Display, "    3  c") || !strings.Contains(result.Display, "    4  d") {
		t.Fatalf("display should keep original line numbers: %q", result.Display)
	}
}

func TestFileReadToolRejectsOutsidePath(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewFileReadTool().Call(tools.DefaultContext(context.Background(), dir), Input{Path: "../x"}, nil); err == nil {
		t.Fatal("expected outside path error")
	}
}

func TestFileReadToolRejectsDirectoryAndBinary(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewFileReadTool().Call(tools.DefaultContext(context.Background(), dir), Input{Path: "."}, nil); err == nil {
		t.Fatal("expected directory error")
	}
	if err := os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileReadTool().Call(tools.DefaultContext(context.Background(), dir), Input{Path: "bin.dat"}, nil); err == nil {
		t.Fatal("expected binary error")
	}
}

func TestFileReadToolUnmarshalInput(t *testing.T) {
	input, err := NewFileReadTool().UnmarshalInput(json.RawMessage(`{"path":"a.txt","start_line":2,"line_limit":3}`))
	if err != nil {
		t.Fatal(err)
	}
	got := input.(Input)
	if got.Path != "a.txt" || got.StartLine != 2 || got.LineLimit != 3 {
		t.Fatalf("got %#v", got)
	}
	if _, err := NewFileReadTool().UnmarshalInput(json.RawMessage(`{"path":""}`)); err == nil {
		t.Fatal("expected empty path error")
	}
	if _, err := NewFileReadTool().UnmarshalInput(json.RawMessage(`{"path":"a.txt","offset":1}`)); err == nil {
		t.Fatal("expected offset rejection")
	}
	if _, err := NewFileReadTool().UnmarshalInput(json.RawMessage(`{"path":"a.txt","limit":10}`)); err == nil {
		t.Fatal("expected limit rejection")
	}
}

func TestFileReadToolPermissions(t *testing.T) {
	tool := NewFileReadTool()
	input := Input{Path: "a.txt"}
	if !tool.IsReadOnly(input) || !tool.IsConcurrencySafe(input) || tool.IsDestructive(input) {
		t.Fatal("unexpected classification")
	}
	if perm := tool.CheckPermissions(tools.Context{}, input); perm.Decision != tools.PermAllow {
		t.Fatalf("permission = %s", perm.Decision)
	}
	if tool.IsReadOnly("bad") || tool.IsConcurrencySafe("bad") {
		t.Fatal("wrong input type should fail closed")
	}
}

func TestFileReadToolStartBeyondEOF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("a\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewFileReadTool().Call(tools.DefaultContext(context.Background(), dir), Input{Path: "data.txt", StartLine: 99, LineLimit: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.Content != "" || out.LineCount != 0 || out.TotalLines != 2 || out.StartLine != 99 {
		t.Fatalf("unexpected out: %#v", out)
	}
}

func TestFileReadToolCRLFNormalization(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("a\r\nb\r\nc\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewFileReadTool().Call(tools.DefaultContext(context.Background(), dir), Input{Path: "data.txt", StartLine: 2, LineLimit: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.Content != "b\nc" {
		t.Fatalf("unexpected CRLF content: %q", out.Content)
	}
}

func TestFileReadToolLargeRangeUsesStreamingPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.txt")
	line := strings.Repeat("x", 128) + "\n"
	// >10MB to force streaming path.
	const targetBytes = readerFastPathThreshold + (2 * 1024 * 1024)
	var b strings.Builder
	for b.Len() < int(targetBytes) {
		b.WriteString(line)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewFileReadTool().Call(tools.DefaultContext(context.Background(), dir), Input{Path: "huge.txt", StartLine: 5, LineLimit: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Data.(Output)
	if out.LineCount == 0 || out.StartLine != 5 || out.TotalLines == 0 {
		t.Fatalf("unexpected streaming output: %#v", out)
	}
}

func TestFileReadToolSchemaDoesNotExposeByteOffsetLimit(t *testing.T) {
	tool := NewFileReadTool()
	s := tool.JSONSchema()
	props, _ := s["properties"].(map[string]any)
	if _, ok := props["offset"]; ok {
		t.Fatal("offset should not be exposed in schema")
	}
	if _, ok := props["limit"]; ok {
		t.Fatal("limit should not be exposed in schema")
	}
	if _, ok := props["start_line"]; !ok {
		t.Fatal("start_line should be exposed in schema")
	}
	if _, ok := props["line_limit"]; !ok {
		t.Fatal("line_limit should be exposed in schema")
	}
}

func TestFileReadToolRangeDedupeReturnsCompactNotice(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("a\nb\nc\nd\ne\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := tools.DefaultContext(context.Background(), dir)
	snaps := map[string]bool{}
	ctx.RecordFileRangeSnapshot = func(path string, startLine, lineCount int, mtimeUnixNano int64, content []byte) {
		key := fmt.Sprintf("%s|%d|%d|%d", path, startLine, lineCount, mtimeUnixNano)
		snaps[key] = true
	}
	ctx.ReadFileRangeSnapshot = func(path string, startLine, lineCount int, mtimeUnixNano int64) ([]byte, bool) {
		key := fmt.Sprintf("%s|%d|%d|%d", path, startLine, lineCount, mtimeUnixNano)
		_, ok := snaps[key]
		return nil, ok
	}
	tool := NewFileReadTool()
	if _, err := tool.Call(ctx, Input{Path: "data.txt", StartLine: 2, LineLimit: 2}, nil); err != nil {
		t.Fatal(err)
	}
	result, err := tool.Call(ctx, Input{Path: "data.txt", StartLine: 2, LineLimit: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Display, "already present in context") {
		t.Fatalf("expected compact dedupe notice, got: %q", result.Display)
	}
}
