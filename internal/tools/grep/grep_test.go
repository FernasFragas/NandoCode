package grep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/tools"
)

func makeCtx(t *testing.T, dir string) tools.Context {
	t.Helper()
	ctx := tools.DefaultContext(context.Background(), dir)
	ctx.PermissionMode = tools.PermissionBypassPermissions
	return ctx
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGrep_LiteralMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello world\ngoodbye\nhello again\n")

	tool := NewGrepTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if len(out.Matches) == 0 {
		t.Fatal("expected matches")
	}
	for _, m := range out.Matches {
		if !m.IsContext && !strings.Contains(m.Content, "hello") {
			t.Errorf("match line %q doesn't contain 'hello'", m.Content)
		}
	}
}

func TestGrep_RegexMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "var FOO = 1\nvar BAR = 2\nvar baz = 3\n")

	tool := NewGrepTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "[A-Z]{3}"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if len(out.Matches) < 2 {
		t.Fatalf("expected at least 2 matches, got %d", len(out.Matches))
	}
}

func TestGrep_BinaryFileSkipped(t *testing.T) {
	dir := t.TempDir()
	// Write binary file with null bytes
	binPath := filepath.Join(dir, "binary.bin")
	os.WriteFile(binPath, []byte{0x7F, 0x00, 0x01, 0x02, 'h', 'e', 'l', 'l', 'o'}, 0o644)
	writeFile(t, dir, "text.txt", "hello world\n")

	tool := NewGrepTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	for _, m := range out.Matches {
		if m.File == "binary.bin" {
			t.Error("binary file should be skipped")
		}
	}
}

func TestGrep_IncludeFilter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "func main() {}\n")
	writeFile(t, dir, "main.txt", "func main() {}\n")

	tool := NewGrepTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "func", Include: "*.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	for _, m := range out.Matches {
		if !strings.HasSuffix(m.File, ".go") {
			t.Errorf("include filter should only return .go files, got %q", m.File)
		}
	}
}

func TestGrep_ExcludeFilter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", "func main() {}\n")
	writeFile(t, dir, "main_test.go", "func TestMain(t *testing.T) {}\n")

	tool := NewGrepTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "func", Exclude: "*_test.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	for _, m := range out.Matches {
		if strings.HasSuffix(m.File, "_test.go") {
			t.Errorf("exclude filter should skip _test.go files, got %q", m.File)
		}
	}
}

func TestGrep_ExcludesDotGit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	writeFile(t, filepath.Join(dir, ".git"), "config", "hello\n")
	writeFile(t, dir, "main.go", "hello world\n")

	tool := NewGrepTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	for _, m := range out.Matches {
		if strings.HasPrefix(m.File, ".git/") {
			t.Error("should not search .git directory")
		}
	}
}

func TestGrep_ContextLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "line1\nline2\nMATCH\nline4\nline5\n")

	tool := NewGrepTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "MATCH", ContextLines: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	// Should have context lines before and after
	var hasContext bool
	for _, m := range out.Matches {
		if m.IsContext {
			hasContext = true
		}
	}
	if !hasContext {
		t.Error("expected context lines in output")
	}
	if len(out.Matches) < 3 {
		t.Fatalf("expected at least 3 lines (match + context), got %d", len(out.Matches))
	}
}

func TestGrep_HeadLimit(t *testing.T) {
	dir := t.TempDir()
	var content strings.Builder
	for i := 0; i < 100; i++ {
		content.WriteString("match line\n")
	}
	writeFile(t, dir, "a.txt", content.String())

	tool := NewGrepTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "match", HeadLimit: 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if !out.AppliedLimit {
		t.Error("expected AppliedLimit=true")
	}
	if len(out.Matches) > 5 {
		t.Fatalf("expected at most 5 matches, got %d", len(out.Matches))
	}
}

func TestGrep_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	tool := NewGrepTool()
	_, err := tool.Call(makeCtx(t, dir), Input{Pattern: "["}, nil)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestGrep_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")

	tool := NewGrepTool()
	res, err := tool.Call(makeCtx(t, dir), Input{Pattern: "anything"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Data.(Output)
	if len(out.Matches) != 0 {
		t.Errorf("expected no matches in empty file, got %d", len(out.Matches))
	}
}
