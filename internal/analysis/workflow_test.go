package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildProjectAnalysisPrompt_ExplicitMentionsFirst(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	t.Setenv("NANDOCODEGO_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "tui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "tui", "app.go"), []byte("package tui\nfunc Render() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "agent.go"), []byte("package internal\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, report, err := BuildProjectAnalysisPrompt(ProjectWorkflowOptions{
		RootDir:   root,
		ScopePath: "internal",
		Question:  "review @internal/agent.go and ui rendering",
		Retrieved: []string{"internal/tui/app.go", "internal/agent.go"},
		MaxFiles:  5,
	})
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if report.SelectedFiles == 0 {
		t.Fatalf("expected selected files, report=%+v", report)
	}
	idxExplicit := strings.Index(prompt, "internal/agent.go")
	idxRetrieved := strings.Index(prompt, "internal/tui/app.go")
	if idxExplicit < 0 || idxRetrieved < 0 {
		t.Fatalf("expected both files in prompt:\n%s", prompt)
	}
	if idxExplicit > idxRetrieved {
		t.Fatalf("expected explicit mention first, prompt:\n%s", prompt)
	}
}

func TestBuildProjectAnalysisPrompt_UsesCacheOnSecondRun(t *testing.T) {
	t.Setenv("NANDOCODEGO_STATE_HOME", t.TempDir())
	t.Setenv("NANDOCODEGO_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := ProjectWorkflowOptions{
		RootDir:   root,
		ScopePath: ".",
		Question:  "analyze",
		Retrieved: []string{"main.go"},
		MaxFiles:  3,
	}
	if _, report1, err := BuildProjectAnalysisPrompt(opts); err != nil {
		t.Fatalf("first run: %v", err)
	} else if report1.CacheMisses == 0 {
		t.Fatalf("expected cache miss on first run: %+v", report1)
	}
	if _, report2, err := BuildProjectAnalysisPrompt(opts); err != nil {
		t.Fatalf("second run: %v", err)
	} else if report2.CacheHits == 0 {
		t.Fatalf("expected cache hit on second run: %+v", report2)
	}
}
