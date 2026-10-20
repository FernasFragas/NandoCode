package runctl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/state"
)

func msg(role llm.Role, content string) llm.Message { return llm.Message{Role: role, Content: content} }

func baseApp() state.App {
	return state.App{
		ActiveRun: true,
		Messages:  []llm.Message{msg(llm.RoleUser, "hi")},
	}
}

func TestApplyTerminalAppendsConversationAndRecordsOutcome(t *testing.T) {
	term := agent.Terminal{
		Reason:       agent.TerminalCompleted,
		Detail:       "ok",
		Usage:        agent.Usage{Turns: 2, ToolCalls: 1},
		Conversation: []llm.Message{msg(llm.RoleAssistant, "hello")},
	}
	got := ApplyTerminal(baseApp(), term, TerminalOptions{})
	if got.ActiveRun {
		t.Error("ActiveRun should be false")
	}
	if got.TerminalReason != agent.TerminalCompleted || got.TerminalDetail != "ok" {
		t.Errorf("terminal fields = %q/%q", got.TerminalReason, got.TerminalDetail)
	}
	if got.Usage.Turns != 2 || got.Usage.ToolCalls != 1 {
		t.Errorf("usage = %+v", got.Usage)
	}
	if len(got.Messages) != 2 || got.Messages[1].Content != "hello" {
		t.Errorf("messages = %+v", got.Messages)
	}
}

func TestApplyTerminalPrependsPromptForSurfacesThatDoNotPreAppend(t *testing.T) {
	app := state.App{ActiveRun: true}
	term := agent.Terminal{Reason: agent.TerminalCompleted, Conversation: []llm.Message{msg(llm.RoleAssistant, "a1")}}
	got := ApplyTerminal(app, term, TerminalOptions{UserPrompt: "q1"})
	if len(got.Messages) != 2 || got.Messages[0].Role != llm.RoleUser || got.Messages[0].Content != "q1" || got.Messages[1].Content != "a1" {
		t.Fatalf("messages = %+v", got.Messages)
	}
	// a second turn keeps the first (per-turn history, as the server does)
	got = ApplyTerminal(got, agent.Terminal{Reason: agent.TerminalCompleted, Conversation: []llm.Message{msg(llm.RoleAssistant, "a2")}}, TerminalOptions{UserPrompt: "q2"})
	if len(got.Messages) != 4 || got.Messages[2].Content != "q2" || got.Messages[3].Content != "a2" {
		t.Fatalf("messages after turn 2 = %+v", got.Messages)
	}
}

func TestApplyTerminalSideRunLeavesHistoryAlone(t *testing.T) {
	term := agent.Terminal{Reason: agent.TerminalCompleted, Usage: agent.Usage{Turns: 1}, Conversation: []llm.Message{msg(llm.RoleAssistant, "side")}}
	got := ApplyTerminal(baseApp(), term, TerminalOptions{Side: true, UserPrompt: "btw"})
	if len(got.Messages) != 1 {
		t.Fatalf("side run changed history: %+v", got.Messages)
	}
	if got.ActiveRun || got.Usage.Turns != 1 {
		t.Errorf("side run must still record outcome: %+v", got)
	}
}

func TestApplyTerminalFailedRunStillKeepsPartialConversation(t *testing.T) {
	term := agent.Terminal{Reason: agent.TerminalAborted, Detail: "canceled", Conversation: []llm.Message{msg(llm.RoleAssistant, "partial")}}
	got := ApplyTerminal(baseApp(), term, TerminalOptions{})
	if got.TerminalReason != agent.TerminalAborted || len(got.Messages) != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestApplyTerminalDoesNotAliasInput(t *testing.T) {
	in := baseApp()
	in.Messages = make([]llm.Message, 1, 8) // spare capacity would tempt an in-place append
	in.Messages[0] = msg(llm.RoleUser, "hi")
	_ = ApplyTerminal(in, agent.Terminal{Conversation: []llm.Message{msg(llm.RoleAssistant, "x")}}, TerminalOptions{})
	if len(in.Messages) != 1 || in.Messages[:2][1].Content != "" {
		t.Fatal("ApplyTerminal wrote into the caller's backing array")
	}
}

func TestBuildCheckpoint(t *testing.T) {
	app := state.App{
		ActiveModel:  "m",
		ToolSettings: state.ToolSettings{WorkingDir: "/w"},
		Messages: []llm.Message{
			msg(llm.RoleUser, "explain @main.go"),
			msg(llm.RoleAssistant, "It starts the server."),
		},
	}
	cp := BuildCheckpoint(app, agent.Terminal{Reason: agent.TerminalCompleted}, "")
	if cp.Model != "m" || cp.WorkingDir != "/w" || cp.LastUserPrompt != "explain @main.go" || cp.LastAssistantOutput != "It starts the server." {
		t.Fatalf("checkpoint = %+v", cp)
	}
	if cp.PendingFinalAnswer || cp.SynthesisStage != "completed" {
		t.Errorf("completed run: pending=%v stage=%q", cp.PendingFinalAnswer, cp.SynthesisStage)
	}

	cp = BuildCheckpoint(app, agent.Terminal{Reason: agent.TerminalAborted, Detail: "x"}, "")
	if !cp.PendingFinalAnswer || cp.SynthesisStage != "synthesis_pending" || cp.TerminalDetail != "x" {
		t.Errorf("aborted run: %+v", cp)
	}
}

func TestBuildCheckpointUsesFallbackAssistantText(t *testing.T) {
	app := state.App{Messages: []llm.Message{msg(llm.RoleUser, "q")}}
	cp := BuildCheckpoint(app, agent.Terminal{Reason: agent.TerminalCompleted}, "from transcript")
	if cp.LastAssistantOutput != "from transcript" {
		t.Fatalf("fallback not used: %+v", cp)
	}
}

func TestSaveRunCheckpointWritesUnderDataHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NANDOCODEGO_STATE_HOME", home)
	app := state.App{Messages: []llm.Message{msg(llm.RoleUser, "q"), msg(llm.RoleAssistant, "done.")}}
	if err := SaveRunCheckpoint(app, agent.Terminal{Reason: agent.TerminalCompleted}, ""); err != nil {
		t.Fatalf("SaveRunCheckpoint: %v", err)
	}
	found := false
	_ = filepath.WalkDir(home, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Base(p) == "latest-checkpoint.json" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("checkpoint file not written under NANDOCODEGO_STATE_HOME")
	}
}
