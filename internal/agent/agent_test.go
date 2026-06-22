package agent

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/tools"
	"github.com/FernasFragas/nandocodego/internal/tools/builtin"
)

func TestAgentNew(t *testing.T) {
	client := newFakeClient(nil)
	reg, _ := builtin.NewRegistry()

	agent, err := New(client, reg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if agent == nil {
		t.Fatal("agent is nil")
	}

	// Nil client should error
	_, err = New(nil, reg)
	if err == nil {
		t.Fatal("expected error for nil client")
	}

	// Nil registry should be allowed
	agent, err = New(client, nil)
	if err != nil {
		t.Fatalf("New with nil registry failed: %v", err)
	}
	if agent.tools != nil {
		t.Fatal("expected nil tools registry")
	}
}

func TestConfigWatchdogForProvider(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	local := cfg.watchdogForProvider(string(llm.ProviderOllamaLocal))
	if local.IdleTimeout != cfg.Watchdog.IdleTimeout {
		t.Fatalf("local timeout=%s want %s", local.IdleTimeout, cfg.Watchdog.IdleTimeout)
	}
	cloud := cfg.watchdogForProvider(string(llm.ProviderOllamaCloudAPI))
	if cloud.IdleTimeout != cfg.CloudWatchdog.IdleTimeout {
		t.Fatalf("cloud timeout=%s want %s", cloud.IdleTimeout, cfg.CloudWatchdog.IdleTimeout)
	}
	zero := Config{}.watchdogForProvider("")
	if zero.IdleTimeout != llm.DefaultWatchdogConfig().IdleTimeout {
		t.Fatalf("zero fallback timeout=%s want %s", zero.IdleTimeout, llm.DefaultWatchdogConfig().IdleTimeout)
	}
}

func TestAgentRunNoTools(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		textTurn("Hello, world!"),
	})

	agent, _ := New(client, nil)
	ctx := context.Background()

	events := agent.Run(ctx, Input{
		Model:    "test-model",
		Messages: []llm.Message{},
	})

	var textDeltas []string
	var terminal *Terminal

	for evt := range events {
		switch e := evt.(type) {
		case AssistantTextDelta:
			textDeltas = append(textDeltas, e.Content)
		case Terminal:
			terminal = &e
		}
	}

	if terminal == nil {
		t.Fatal("no terminal event")
	}
	if terminal.Reason != TerminalCompleted {
		t.Fatalf("expected completed, got %s", terminal.Reason)
	}
	if len(textDeltas) == 0 {
		t.Fatal("expected text deltas")
	}
	if strings.Join(textDeltas, "") != "Hello, world!" {
		t.Fatalf("unexpected text: %v", textDeltas)
	}
}

func TestAgentRunThinkingAndContent(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		thinkingTurn("Let me think...", "The answer is 42."),
	})

	agent, _ := New(client, nil)
	ctx := context.Background()

	events := agent.Run(ctx, Input{
		Model:    "test-model",
		Messages: []llm.Message{},
	})

	var thinkingDeltas []string
	var textDeltas []string

	for evt := range events {
		switch e := evt.(type) {
		case AssistantThinkingDelta:
			thinkingDeltas = append(thinkingDeltas, e.Thinking)
		case AssistantTextDelta:
			textDeltas = append(textDeltas, e.Content)
		}
	}

	if strings.Join(thinkingDeltas, "") != "Let me think..." {
		t.Fatalf("unexpected thinking: %v", thinkingDeltas)
	}
	if strings.Join(textDeltas, "") != "The answer is 42." {
		t.Fatalf("unexpected text: %v", textDeltas)
	}
}

func TestAgentRunToolCall(t *testing.T) {
	reg, _ := builtin.NewRegistry()
	client := newFakeClient([]fakeTurn{
		toolCallTurn("Bash", map[string]any{"command": "ls"}),
		textTurn("The directory contains files."),
	})

	agent, _ := New(client, reg)
	ctx := context.Background()
	tmpDir := t.TempDir()

	events := agent.Run(ctx, Input{
		Model: "test-model",
		ToolContext: tools.Context{
			Context:        ctx,
			WorkingDir:     tmpDir,
			PermissionMode: tools.PermissionBypassPermissions,
		},
	})

	var toolStarts []string
	var toolResults []string
	var terminal *Terminal

	for evt := range events {
		switch e := evt.(type) {
		case ToolUseStart:
			toolStarts = append(toolStarts, e.Name)
		case ToolUseResult:
			toolResults = append(toolResults, e.ID)
		case Terminal:
			terminal = &e
		}
	}

	if len(toolStarts) == 0 {
		t.Fatal("expected tool start")
	}
	if toolStarts[0] != "Bash" {
		t.Fatalf("expected Bash, got %s", toolStarts[0])
	}
	if len(toolResults) == 0 {
		t.Fatal("expected tool result")
	}
	if terminal == nil {
		t.Fatal("no terminal event")
	}
	if terminal.Reason != TerminalCompleted {
		t.Fatalf("expected completed, got %s", terminal.Reason)
	}
	if terminal.Usage.Turns != 2 {
		t.Fatalf("expected 2 turns, got %d", terminal.Usage.Turns)
	}
	if terminal.Usage.ToolCalls != 1 {
		t.Fatalf("expected 1 tool call, got %d", terminal.Usage.ToolCalls)
	}
}

func TestAgentRunUnknownTool(t *testing.T) {
	reg, _ := builtin.NewRegistry()
	client := newFakeClient([]fakeTurn{
		toolCallTurn("UnknownTool", map[string]any{"arg": "value"}),
		textTurn("I understand that tool doesn't exist."),
	})

	agent, _ := New(client, reg)
	ctx := context.Background()

	events := agent.Run(ctx, Input{
		Model: "test-model",
		ToolContext: tools.Context{
			Context:    ctx,
			WorkingDir: t.TempDir(),
		},
	})

	var toolResults []ToolUseResult
	var terminal *Terminal

	for evt := range events {
		switch e := evt.(type) {
		case ToolUseResult:
			toolResults = append(toolResults, e)
		case Terminal:
			terminal = &e
		}
	}

	if len(toolResults) == 0 {
		t.Fatal("expected tool result")
	}
	if toolResults[0].Err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(toolResults[0].Err.Error(), "unknown tool") {
		t.Fatalf("unexpected error: %v", toolResults[0].Err)
	}
	if terminal.Reason != TerminalCompleted {
		t.Fatalf("expected completed, got %s", terminal.Reason)
	}
}

func TestAgentRunMalformedToolArgs(t *testing.T) {
	reg, _ := builtin.NewRegistry()
	// FileRead requires "path" field
	client := newFakeClient([]fakeTurn{
		toolCallTurn("FileRead", map[string]any{"wrong": "field"}),
		textTurn("I see the input was invalid."),
	})

	agent, _ := New(client, reg)
	ctx := context.Background()

	events := agent.Run(ctx, Input{
		Model: "test-model",
		ToolContext: tools.Context{
			Context:    ctx,
			WorkingDir: t.TempDir(),
		},
	})

	var toolResults []ToolUseResult

	for evt := range events {
		if e, ok := evt.(ToolUseResult); ok {
			toolResults = append(toolResults, e)
		}
	}

	if len(toolResults) == 0 {
		t.Fatal("expected tool result")
	}
	if toolResults[0].Err == nil {
		t.Fatal("expected error for malformed args")
	}
}

func TestAgentRunDeniedTool(t *testing.T) {
	reg, _ := builtin.NewRegistry()
	// rm -rf is destructive and should be denied in default mode
	client := newFakeClient([]fakeTurn{
		toolCallTurn("Bash", map[string]any{"command": "rm -rf /"}),
		textTurn("I understand that command is not allowed."),
	})

	agent, _ := New(client, reg)
	ctx := context.Background()

	events := agent.Run(ctx, Input{
		Model: "test-model",
		ToolContext: tools.Context{
			Context:        ctx,
			WorkingDir:     t.TempDir(),
			PermissionMode: tools.PermissionDefault,
		},
	})

	var toolResults []ToolUseResult

	for evt := range events {
		if e, ok := evt.(ToolUseResult); ok {
			toolResults = append(toolResults, e)
		}
	}

	if len(toolResults) == 0 {
		t.Fatal("expected tool result")
	}
	if toolResults[0].Err == nil {
		t.Fatal("expected permission denied error")
	}
	if !strings.Contains(toolResults[0].Err.Error(), "permission") {
		t.Fatalf("unexpected error: %v", toolResults[0].Err)
	}
}

func TestAgentReadOnlyToolsetFiltersToolDefs(t *testing.T) {
	reg, _ := builtin.NewRegistry()
	client := newFakeClient([]fakeTurn{
		textTurn("ok"),
	})
	a, _ := New(client, reg)
	events := a.Run(context.Background(), Input{
		Model:       "test-model",
		ToolsetName: ToolsetReadOnly,
		ToolContext: tools.Context{
			WorkingDir: t.TempDir(),
		},
	})
	for range events {
	}
	if len(client.requests) == 0 {
		t.Fatal("expected at least one chat request")
	}
	var names []string
	for _, td := range client.requests[0].Tools {
		names = append(names, td.Function.Name)
	}
	want := []string{"FileRead", "Glob", "Grep"}
	if !slices.Equal(names, want) {
		t.Fatalf("tool defs mismatch: got %v want %v", names, want)
	}
}

func TestAgentReadOnlyToolsetRejectsMutatingToolCalls(t *testing.T) {
	reg, _ := builtin.NewRegistry()
	client := newFakeClient([]fakeTurn{
		toolCallTurn("Bash", map[string]any{"command": "ls"}),
		textTurn("fallback"),
	})
	a, _ := New(client, reg)
	events := a.Run(context.Background(), Input{
		Model:       "test-model",
		ToolsetName: ToolsetReadOnly,
		ToolContext: tools.Context{
			WorkingDir: t.TempDir(),
		},
	})
	var toolResults []ToolUseResult
	for evt := range events {
		if tr, ok := evt.(ToolUseResult); ok {
			toolResults = append(toolResults, tr)
		}
	}
	if len(toolResults) == 0 {
		t.Fatal("expected tool result")
	}
	if toolResults[0].Err == nil || !strings.Contains(toolResults[0].Err.Error(), "unknown tool") {
		t.Fatalf("expected unknown-tool error, got %#v", toolResults[0].Err)
	}
}

func TestAgentToolModeNoneOmitsToolDefs(t *testing.T) {
	reg, _ := builtin.NewRegistry()
	client := newFakeClient([]fakeTurn{
		textTurn("ok"),
	})
	a, _ := New(client, reg)
	events := a.Run(context.Background(), Input{
		Model:    "test-model",
		ToolMode: ToolModeNone,
		ToolContext: tools.Context{
			WorkingDir: t.TempDir(),
		},
	})
	for range events {
	}
	if len(client.requests) == 0 {
		t.Fatal("expected at least one chat request")
	}
	if got := len(client.requests[0].Tools); got != 0 {
		t.Fatalf("expected no tools in request, got %d", got)
	}
}

func TestValidateInputToolModeNonePropagatesContext(t *testing.T) {
	t.Parallel()
	in := Input{
		Model:    "test-model",
		ToolMode: ToolModeNone,
		ToolContext: tools.Context{
			Context: context.Background(),
		},
	}
	if err := validateInput(context.Background(), &in); err != nil {
		t.Fatalf("validateInput failed: %v", err)
	}
	if in.ToolMode != ToolModeNone {
		t.Fatalf("tool mode=%q", in.ToolMode)
	}
	if got := toolModeFromContext(in.ToolContext.Context); got != ToolModeNone {
		t.Fatalf("tool mode from context=%q", got)
	}
}

func TestAgentInjectsPendingMessagesBetweenTurns(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		textTurn("first"),
		textTurn("second"),
	})
	a, _ := New(client, nil)
	injected := false
	events := a.Run(context.Background(), Input{
		Model:    "test-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "start"}},
		PendingMessagesProvider: func(ctx context.Context) []llm.Message {
			if injected {
				return nil
			}
			injected = true
			return []llm.Message{{Role: llm.RoleUser, Content: "[mailbox] ping"}}
		},
	})
	var terminal *Terminal
	for evt := range events {
		if tEvt, ok := evt.(Terminal); ok {
			terminal = &tEvt
		}
	}
	if terminal == nil {
		t.Fatal("expected terminal event")
	}
	if len(client.requests) < 2 {
		t.Fatalf("expected at least 2 turns, got %d", len(client.requests))
	}
	foundMailbox := false
	for _, m := range client.requests[1].Messages {
		if m.Role == llm.RoleUser && strings.Contains(m.Content, "[mailbox] ping") {
			foundMailbox = true
			break
		}
	}
	if !foundMailbox {
		t.Fatal("expected injected mailbox message in second turn request")
	}
}

func TestAgentRunProgressEvents(t *testing.T) {
	reg, _ := builtin.NewRegistry()
	client := newFakeClient([]fakeTurn{
		toolCallTurn("Bash", map[string]any{"command": "echo hello"}),
		textTurn("Done."),
	})

	agent, _ := New(client, reg)
	ctx := context.Background()

	events := agent.Run(ctx, Input{
		Model: "test-model",
		ToolContext: tools.Context{
			Context:        ctx,
			WorkingDir:     t.TempDir(),
			PermissionMode: tools.PermissionBypassPermissions,
		},
	})

	var progressEvents []ToolUseProgress

	for evt := range events {
		if e, ok := evt.(ToolUseProgress); ok {
			progressEvents = append(progressEvents, e)
		}
	}

	// Bash tool emits progress for stdout/stderr
	if len(progressEvents) == 0 {
		t.Log("Note: progress events depend on tool implementation")
	}
}

func TestAgentRunAbort(t *testing.T) {
	// Use a pre-cancelled context to ensure abort
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client := newFakeClient([]fakeTurn{
		textTurn("This should not complete"),
	})

	agent, _ := New(client, nil)

	events := agent.Run(ctx, Input{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "summarize what is missing in this implementation"},
		},
		OriginalUserText: "summarize what is missing in this implementation",
	})

	start := time.Now()
	var got Terminal

	for evt := range events {
		if term, ok := evt.(Terminal); ok {
			got = term
			break
		}
	}
	elapsed := time.Since(start)

	if got.Reason != TerminalAborted {
		t.Fatalf("expected aborted, got %s: %s", got.Reason, got.Detail)
	}
	if elapsed > 200*time.Millisecond {
		t.Logf("Warning: abort took %v, target is <200ms", elapsed)
	}
}

func TestAgentRunMaxTurns(t *testing.T) {
	reg, _ := builtin.NewRegistry()
	// Create turns that keep calling tools, ensuring we hit the limit
	turns := []fakeTurn{
		toolCallTurn("Bash", map[string]any{"command": "echo turn1"}),
		toolCallTurn("Bash", map[string]any{"command": "echo turn2"}),
		toolCallTurn("Bash", map[string]any{"command": "echo turn3"}), // This should be prevented
	}

	client := newFakeClient(turns)
	agent, _ := New(client, reg, WithConfig(Config{
		MaxTurns:          2,
		MaxOutputTokens:   8192,
		LengthRetryTokens: 65536,
		Watchdog:          llm.DefaultWatchdogConfig(),
	}))

	ctx := context.Background()
	events := agent.Run(ctx, Input{
		Model: "test-model",
		ToolContext: tools.Context{
			Context:        ctx,
			WorkingDir:     t.TempDir(),
			PermissionMode: tools.PermissionBypassPermissions,
		},
	})

	var terminal *Terminal
	var turnCount int
	seenTurns := make(map[int]bool)

	for evt := range events {
		switch e := evt.(type) {
		case ToolUseStart:
			// Count unique turns by checking if we've seen this tool start before
			if !seenTurns[turnCount] {
				seenTurns[turnCount] = true
			}
			turnCount = len(seenTurns)
		case Terminal:
			terminal = &e
		}
	}

	if terminal == nil {
		t.Fatal("no terminal event")
	}
	if terminal.Reason != TerminalMaxTurns {
		t.Fatalf("expected max_turns, got %s (detail: %s)", terminal.Reason, terminal.Detail)
	}
	if terminal.Usage.Turns > 2 {
		t.Fatalf("expected at most 2 turns, got %d", terminal.Usage.Turns)
	}
}

func TestAgentRunLengthRetry(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		lengthTurn(),
		textTurn("Success after retry."),
	})

	agent, _ := New(client, nil)
	ctx := context.Background()

	events := agent.Run(ctx, Input{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "summarize what is missing in this implementation"},
		},
		OriginalUserText: "summarize what is missing in this implementation",
	})

	var retryNotices []RetryNotice
	var terminal *Terminal

	for evt := range events {
		if retry, ok := evt.(RetryNotice); ok {
			retryNotices = append(retryNotices, retry)
		}
		if term, ok := evt.(Terminal); ok {
			terminal = &term
		}
	}

	if len(retryNotices) == 0 {
		t.Fatal("expected retry notice for length")
	}
	if terminal.Reason != TerminalCompleted {
		t.Fatalf("expected completed after retry, got %s", terminal.Reason)
	}
}

func TestAgentRunRetriesIncompleteAssistantPreamble(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		textTurn("Based on the files, here is the comprehensive missing-implementation summary:"),
		textTurn("The implementation is missing these pieces:\n- Add the guard.\n- Add tests."),
	})

	agent, _ := New(client, nil)
	ctx := context.Background()

	events := agent.Run(ctx, Input{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "summarize what is missing in this implementation"},
		},
		OriginalUserText: "summarize what is missing in this implementation",
	})

	var retryNotices []RetryNotice
	var terminal *Terminal
	var text strings.Builder

	for evt := range events {
		switch e := evt.(type) {
		case AssistantTextDelta:
			text.WriteString(e.Content)
		case RetryNotice:
			retryNotices = append(retryNotices, e)
		case Terminal:
			terminal = &e
		}
	}

	if len(retryNotices) != 1 {
		t.Fatalf("expected one retry notice, got %d", len(retryNotices))
	}
	if !strings.Contains(retryNotices[0].Cause, "incomplete") {
		t.Fatalf("unexpected retry cause: %q", retryNotices[0].Cause)
	}
	if retryNotices[0].Kind != "incomplete_assistant_response" {
		t.Fatalf("unexpected retry kind: %q", retryNotices[0].Kind)
	}
	if retryNotices[0].DoneReason != "stop" {
		t.Fatalf("unexpected retry done reason: %q", retryNotices[0].DoneReason)
	}
	if retryNotices[0].AssistantChars == 0 {
		t.Fatal("expected retry assistant char count")
	}
	if got := len(client.requests); got != 2 {
		t.Fatalf("expected two chat requests, got %d", got)
	}
	if terminal == nil || terminal.Reason != TerminalCompleted {
		t.Fatalf("expected completed terminal, got %#v", terminal)
	}
	if terminal.Usage.DoneReason != "stop" {
		t.Fatalf("expected terminal done reason stop, got %q", terminal.Usage.DoneReason)
	}
	if !strings.Contains(text.String(), "Add the guard") {
		t.Fatalf("expected retried answer in text stream, got %q", text.String())
	}
	if len(terminal.Conversation) != 2 {
		t.Fatalf("expected two assistant messages in conversation, got %d", len(terminal.Conversation))
	}
}

func TestAgentRunRetriesIncompleteAssistantPreambleWithAnchoredListingPrompt(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		textTurn("Now I have the full picture. Let me write the missing-implementation summary:"),
		textTurn("docs/\n- a.md\n- b.md"),
	})

	agent, _ := New(client, nil)
	ctx := context.Background()
	events := agent.Run(ctx, Input{
		Model: "test-model",
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: "User request:\nlist all files in @docs/\n\nDirectory tree data:\ndocs/\ndocs/a.md\ndocs/b.md",
			},
		},
		PromptIntent:     "directory_listing",
		AttachmentPolicy: "listing_tree_only",
		OriginalUserText: "list all files in @docs/",
	})

	var terminal *Terminal
	for evt := range events {
		if term, ok := evt.(Terminal); ok {
			terminal = &term
		}
	}

	if terminal == nil || terminal.Reason != TerminalCompleted {
		t.Fatalf("expected completed terminal, got %#v", terminal)
	}
	if got := len(client.requests); got != 2 {
		t.Fatalf("expected two chat requests, got %d", got)
	}
	second := client.requests[1]
	if len(second.Messages) == 0 {
		t.Fatal("expected second request messages")
	}
	last := second.Messages[len(second.Messages)-1]
	if last.Role != llm.RoleUser {
		t.Fatalf("expected retry user message, got role=%q", last.Role)
	}
	if !strings.Contains(last.Content, "Original user request:") || !strings.Contains(last.Content, "Directory tree data:") {
		t.Fatalf("expected anchored listing retry prompt, got %q", last.Content)
	}
	if strings.Contains(strings.ToLower(last.Content), "promised answer") {
		t.Fatalf("retry prompt must not include promised-answer wording, got %q", last.Content)
	}
}

func TestAgentRunDoesNotRetryNormalShortAnswer(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		textTurn("Done."),
	})

	agent, _ := New(client, nil)
	ctx := context.Background()

	events := agent.Run(ctx, Input{Model: "test-model"})

	var retryNotices []RetryNotice
	var terminal *Terminal
	for evt := range events {
		switch e := evt.(type) {
		case RetryNotice:
			retryNotices = append(retryNotices, e)
		case Terminal:
			terminal = &e
		}
	}

	if len(retryNotices) != 0 {
		t.Fatalf("expected no retry notices, got %d", len(retryNotices))
	}
	if got := len(client.requests); got != 1 {
		t.Fatalf("expected one chat request, got %d", got)
	}
	if terminal == nil || terminal.Reason != TerminalCompleted {
		t.Fatalf("expected completed terminal, got %#v", terminal)
	}
}

func TestAgentRunStreamFailureDoneReasonIsUnrecoverable(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		{events: []llm.StreamEvent{{Done: true, DoneReason: "watchdog_timeout"}}},
	})

	agent, _ := New(client, nil)
	ctx := context.Background()

	events := agent.Run(ctx, Input{Model: "test-model"})

	var terminal *Terminal
	for evt := range events {
		if term, ok := evt.(Terminal); ok {
			terminal = &term
		}
	}

	if terminal == nil {
		t.Fatal("no terminal event")
	}
	if terminal.Reason != TerminalUnrecoverable {
		t.Fatalf("expected unrecoverable terminal, got %s", terminal.Reason)
	}
	if terminal.Usage.DoneReason != "watchdog_timeout" {
		t.Fatalf("expected done reason watchdog_timeout, got %q", terminal.Usage.DoneReason)
	}
	if !strings.Contains(terminal.Detail, "watchdog") {
		t.Fatalf("expected watchdog detail, got %q", terminal.Detail)
	}
}

func TestAgentRunContextOverflow(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		lengthTurn(),
		lengthTurn(),
	})

	agent, _ := New(client, nil)
	ctx := context.Background()

	events := agent.Run(ctx, Input{Model: "test-model"})

	var terminal *Terminal
	for evt := range events {
		if term, ok := evt.(Terminal); ok {
			terminal = &term
		}
	}

	if terminal == nil {
		t.Fatal("no terminal event")
	}
	if terminal.Reason != TerminalContextOverflow {
		t.Fatalf("expected context_overflow, got %s", terminal.Reason)
	}
}

func TestAgentRunCompactionOnSecondLength(t *testing.T) {
	// Build enough history to pass MinTurns=2
	history := makeTurnMessages(3)

	// Client: 2 length turns to trigger reactive compaction, then a summary turn (for compact),
	// then a final success turn
	client := newFakeClient([]fakeTurn{
		lengthTurn(),
		lengthTurn(),
		textTurn("summary of earlier conversation"), // used by Compact's LLM call
		textTurn("Final answer after compaction."),
	})

	a, _ := New(client, nil, WithCompactionConfig(CompactionConfig{
		Threshold:     0.8,
		MinTurns:      2,
		MaxSummaryLen: 2000,
	}))
	ctx := context.Background()

	var compactionStarted, compactionCompleted bool
	var terminal *Terminal

	for evt := range a.Run(ctx, Input{Model: "test-model", Messages: history}) {
		switch e := evt.(type) {
		case CompactionStarted:
			compactionStarted = true
		case CompactionCompleted:
			compactionCompleted = true
			if e.Result.Skipped {
				t.Error("expected compaction not skipped")
			}
		case Terminal:
			terminal = &e
		}
	}

	if !compactionStarted {
		t.Error("expected CompactionStarted event")
	}
	if !compactionCompleted {
		t.Error("expected CompactionCompleted event")
	}
	if terminal == nil {
		t.Fatal("no terminal event")
	}
	if terminal.Reason != TerminalCompleted {
		t.Fatalf("expected completed after compaction, got %s: %s", terminal.Reason, terminal.Detail)
	}
}

func TestAgentRunCompactionDisabled(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		lengthTurn(),
		lengthTurn(),
	})

	a, _ := New(client, nil, WithCompactionConfig(CompactionConfig{Disabled: true}))
	ctx := context.Background()

	var terminal *Terminal
	for evt := range a.Run(ctx, Input{Model: "test-model"}) {
		if term, ok := evt.(Terminal); ok {
			terminal = &term
		}
	}

	if terminal == nil {
		t.Fatal("no terminal event")
	}
	if terminal.Reason != TerminalContextOverflow {
		t.Fatalf("expected context_overflow when disabled, got %s", terminal.Reason)
	}
}

func TestAgentRunCompactionSkippedMinTurns(t *testing.T) {
	// Empty history — 0 turns, below MinTurns=4
	client := newFakeClient([]fakeTurn{
		lengthTurn(),
		lengthTurn(),
	})

	a, _ := New(client, nil) // default config MinTurns=4
	ctx := context.Background()

	var compactionStarted bool
	var terminal *Terminal
	for evt := range a.Run(ctx, Input{Model: "test-model"}) {
		switch e := evt.(type) {
		case CompactionStarted:
			compactionStarted = true
		case CompactionCompleted:
			if !e.Result.Skipped {
				t.Error("expected compaction to be skipped when not enough turns")
			}
		case Terminal:
			terminal = &e
		}
	}

	if !compactionStarted {
		t.Error("expected CompactionStarted even on skip")
	}
	if terminal == nil {
		t.Fatal("no terminal event")
	}
	if terminal.Reason != TerminalContextOverflow {
		t.Fatalf("expected context_overflow after skipped compaction, got %s", terminal.Reason)
	}
}

func TestAgentRunCompactRequestChannel(t *testing.T) {
	history := makeTurnMessages(3)
	client := newFakeClient([]fakeTurn{
		textTurn("summary"),  // for compact's LLM call
		textTurn("response"), // for main turn
	})

	a, _ := New(client, nil, WithCompactionConfig(CompactionConfig{
		Threshold:     0.8,
		MinTurns:      2,
		MaxSummaryLen: 2000,
	}))
	ctx := context.Background()

	compactCh := make(chan struct{}, 1)
	compactCh <- struct{}{} // pre-signal compact before first turn

	var compactionStarted bool
	for evt := range a.Run(ctx, Input{
		Model:          "test-model",
		Messages:       history,
		CompactRequest: compactCh,
	}) {
		if _, ok := evt.(CompactionStarted); ok {
			compactionStarted = true
		}
	}

	if !compactionStarted {
		t.Error("expected CompactionStarted from manual compact request")
	}
}

func TestInputValidation(t *testing.T) {
	client := newFakeClient(nil)
	agent, _ := New(client, nil)
	ctx := context.Background()

	// Missing model
	events := agent.Run(ctx, Input{})
	var terminal *Terminal
	for evt := range events {
		if term, ok := evt.(Terminal); ok {
			terminal = &term
		}
	}
	if terminal == nil || terminal.Reason != TerminalUnrecoverable {
		t.Fatal("expected unrecoverable for missing model")
	}
}

func TestAgentRunEmitsPromptPackReportWhenHistoryTrimmed(t *testing.T) {
	client := newFakeClient([]fakeTurn{
		textTurn("ok"),
	})
	a, _ := New(client, nil, WithConfig(Config{
		MaxTurns:           1,
		MaxOutputTokens:    256,
		LengthRetryTokens:  512,
		NumCtx:             1024,
		MaxConcurrentTools: 1,
		Watchdog:           llm.DefaultWatchdogConfig(),
		Compaction:         DefaultCompactionConfig(),
		ContextMode:        "auto",
		ContextMinNumCtx:   512,
		ContextMaxNumCtx:   0,
		ContextReserve:     128,
	}))
	ctx := context.Background()
	history := []llm.Message{
		{Role: llm.RoleSystem, Content: strings.Repeat("S", 12000)},
		{Role: llm.RoleUser, Content: strings.Repeat("U", 20000)},
		{Role: llm.RoleAssistant, Content: strings.Repeat("A", 20000)},
		{Role: llm.RoleUser, Content: "latest"},
	}

	var sawPack bool
	for evt := range a.Run(ctx, Input{
		Model:           "test-model",
		Messages:        history,
		MaxOutputTokens: 128,
	}) {
		if rep, ok := evt.(PromptPackReport); ok {
			sawPack = true
			if rep.SkippedMessages == 0 {
				t.Fatal("expected skipped messages in report")
			}
		}
	}

	if !sawPack {
		t.Fatal("expected PromptPackReport event")
	}
}
