package eval

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/turnprep"
)

func TestBuildAgentInputKeepsEvaluatorPromptAndAddsGrounding(t *testing.T) {
	fixture := Fixture{Task: "Explain the retry loop."}
	fixture.Config.Execution.PermissionMode = "default"
	fixture.Config.Execution.ApprovalStrategy = ApprovalStrategyAllow
	in, _, err := buildAgentInput(context.Background(), fixture, t.TempDir(), fixture.Config, "m")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(in.SystemPrompt, evaluatorSystemPrompt) || !strings.Contains(in.SystemPrompt, turnprep.GroundingMarker) {
		t.Fatalf("system prompt=%q", in.SystemPrompt)
	}
	if len(in.Messages) != 1 || in.Messages[0].Content != fixture.Task {
		t.Fatalf("eval must hand the task through unchanged: %#v", in.Messages)
	}
	if in.ToolMode != "" || in.RouteAction != "" {
		t.Fatalf("eval keeps routing out of the run: tool=%q action=%q", in.ToolMode, in.RouteAction)
	}
	if in.MaxOutputTokens != 8192 {
		t.Fatalf("max output tokens=%d", in.MaxOutputTokens)
	}
}

// The grounded-code-question fixture reproduces the hallucination
// investigation prompt against a tiny repo. Its recording shows the grounded
// behaviour (search, read, then answer from evidence).
func TestGroundedCodeQuestionFixtureRecordedPasses(t *testing.T) {
	root := t.TempDir()
	fixtures, err := LoadAndValidate(filepath.Join("..", "..", "evals"))
	if err != nil {
		t.Fatal(err)
	}
	var picked []Fixture
	for _, f := range fixtures {
		if f.ID == "grounded-code-question" {
			picked = append(picked, f)
		}
	}
	if len(picked) != 1 {
		t.Fatalf("fixture grounded-code-question not found")
	}
	report, err := Run(context.Background(), picked, RunOptions{Provider: ProviderRecorded, OutputDir: filepath.Join(root, "out")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].Status != StatusPassed {
		t.Fatalf("results=%+v", report.Results)
	}
}
