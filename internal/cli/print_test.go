package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/mentions"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

func TestCollectPrintOutputAndTextWrite(t *testing.T) {
	t.Parallel()
	events := make(chan agent.Event, 4)
	events <- agent.AssistantTextDelta{Content: "hello "}
	events <- agent.AssistantTextDelta{Content: "world"}
	events <- agent.Terminal{Reason: agent.TerminalCompleted}
	close(events)

	content, toolUses, warnings, term, err := collectPrintOutput(events)
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello world" {
		t.Fatalf("content=%q", content)
	}
	if len(toolUses) != 0 {
		t.Fatalf("expected no tool uses, got %d", len(toolUses))
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %d", len(warnings))
	}
	if term.Reason != agent.TerminalCompleted {
		t.Fatalf("unexpected terminal reason %q", term.Reason)
	}

	var out bytes.Buffer
	if err := writePrintOutput(&out, content, nil, term, false); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello world\n" {
		t.Fatalf("text output=%q", got)
	}
}

func TestWritePrintOutputJSON(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	toolUses := []map[string]any{{"name": "Bash", "ok": true}}
	term := agent.Terminal{Reason: agent.TerminalCompleted}
	if err := writePrintOutput(&out, "hi", toolUses, term, true); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["content"] != "hi" {
		t.Fatalf("content=%v", decoded["content"])
	}
	if _, ok := decoded["tool_uses"]; !ok {
		t.Fatal("missing tool_uses field")
	}
	if _, ok := decoded["usage"]; !ok {
		t.Fatal("missing usage field")
	}
}

func TestCodeForTerminalReason(t *testing.T) {
	t.Parallel()
	if got := codeForTerminalReason(agent.TerminalAborted); got != 1 {
		t.Fatalf("aborted code=%d", got)
	}
	if got := codeForTerminalReason(agent.TerminalUnrecoverable); got != 2 {
		t.Fatalf("unrecoverable code=%d", got)
	}
	if got := codeForTerminalReason(agent.TerminalMaxTurns); got != 1 {
		t.Fatalf("max_turns code=%d", got)
	}
}

func TestCollectPrintOutputRequiresTerminal(t *testing.T) {
	t.Parallel()
	events := make(chan agent.Event, 1)
	events <- agent.AssistantTextDelta{Content: "x"}
	close(events)
	if _, _, _, _, err := collectPrintOutput(events); err == nil {
		t.Fatal("expected missing terminal error")
	}
}

func TestBuildPrintInputIncludesLargeFileTailRangeEvidence(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 4200; i++ {
		line := "noise"
		if i == 4199 {
			line = "LATEST_STATUS_MARKER implemented context packing"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(root, "phase-log.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	toolCtx := tools.DefaultContext(context.Background(), root)
	in, err := buildPrintInput(
		"review @phase-log.md",
		toolCtx,
		agent.Config{ContextMode: "auto", MaxOutputTokens: 4096, NumCtx: 32768, ContextReserve: 4096},
		"test-model",
		"auto",
		4096,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Messages) != 1 {
		t.Fatalf("messages=%d", len(in.Messages))
	}
	prompt := in.Messages[0].Content
	if !strings.Contains(prompt, "<file_range path=\"phase-log.md\"") {
		t.Fatalf("expected file_range evidence block:\n%s", prompt)
	}
	if !strings.Contains(prompt, "LATEST_STATUS_MARKER implemented context packing") {
		t.Fatalf("expected tail marker in prompt:\n%s", prompt)
	}
}

func TestBuildPrintInputUsesToolModeNoneForCheapPrompt(t *testing.T) {
	root := t.TempDir()
	toolCtx := tools.DefaultContext(context.Background(), root)
	in, err := buildPrintInput(
		"Respond with exactly: ok",
		toolCtx,
		agent.Config{ContextMode: "auto", MaxOutputTokens: 4096, NumCtx: 32768, ContextReserve: 4096},
		"test-model",
		"auto",
		4096,
	)
	if err != nil {
		t.Fatal(err)
	}
	if in.ToolMode != agent.ToolModeNone {
		t.Fatalf("tool mode=%q want %q", in.ToolMode, agent.ToolModeNone)
	}
	if in.RouteReason != "skip_general_prompt" {
		t.Fatalf("route reason=%q want skip_general_prompt", in.RouteReason)
	}
	if in.RouteProfile != "general_prompt" {
		t.Fatalf("route profile=%q want general_prompt", in.RouteProfile)
	}
}

func TestBuildPrintInputKeepsDefaultToolModeForFileStatusPrompt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("status\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	toolCtx := tools.DefaultContext(context.Background(), root)
	in, err := buildPrintInput(
		"what is the current status of @plan.md",
		toolCtx,
		agent.Config{ContextMode: "auto", MaxOutputTokens: 4096, NumCtx: 32768, ContextReserve: 4096},
		"test-model",
		"auto",
		4096,
	)
	if err != nil {
		t.Fatal(err)
	}
	if in.ToolMode != agent.ToolModeDefault {
		t.Fatalf("tool mode=%q want %q", in.ToolMode, agent.ToolModeDefault)
	}
	if in.AttachmentPolicy != string(mentions.AttachContent) {
		t.Fatalf("attachment policy=%q want %q", in.AttachmentPolicy, mentions.AttachContent)
	}
	if in.RouteAction != "use_explicit_context_only" {
		t.Fatalf("route action=%q want use_explicit_context_only", in.RouteAction)
	}
}
