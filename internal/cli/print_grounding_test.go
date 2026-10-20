package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/turnprep"
)

func TestBuildPrintInputGroundsCodeQuestionsButNotToollessPrompts(t *testing.T) {
	toolCtx := tools.DefaultContext(context.Background(), t.TempDir())
	cfg := agent.Config{ContextMode: "auto", MaxOutputTokens: 4096, NumCtx: 32768, ContextReserve: 4096}
	in, err := buildPrintInput("Find the server path that updates session models, verify the behavior", toolCtx, cfg, "test-model", "auto", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(in.SystemPrompt, turnprep.GroundingMarker) {
		t.Fatalf("system prompt lacks grounding: %q", in.SystemPrompt)
	}
	in, err = buildPrintInput("Respond with exactly: ok", toolCtx, cfg, "test-model", "auto", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if in.SystemPrompt != "" {
		t.Fatalf("tool-less prompt must stay ungrounded: %q", in.SystemPrompt)
	}
}
