package agent

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/paths"
	"github.com/FernasFragas/nandocodego/internal/permissions"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

type BubbleEscalation interface {
	Ask(context.Context, permissions.Prompt) (permissions.Decision, string, error)
}

type NilBubbleEscalation struct{}

func (NilBubbleEscalation) Ask(_ context.Context, _ permissions.Prompt) (permissions.Decision, string, error) {
	return permissions.DecisionDeny, "escalation unavailable", nil
}

// SpawnMode selects the sub-agent variant.
type SpawnMode string

const (
	SpawnBuiltin SpawnMode = "builtin"
	SpawnCustom  SpawnMode = "custom"
	SpawnFork    SpawnMode = "fork"
)

// SubagentParams defines how a sub-agent is spawned.
type SubagentParams struct {
	Mode           SpawnMode
	SystemPrompt   string
	Task           string
	Model          string
	PermissionMode permissions.Mode
	Messages       []llm.Message
	Background     bool
	MaxTurns       int
	SessionID      string
	NotifyParent   func(string)
}

func runSubagent(
	ctx context.Context,
	parentInput Input,
	params SubagentParams,
	client llm.Client,
	registry *tools.Registry,
	baseConfig Config,
) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sub-agent panic: %v", r)
		}
	}()
	if strings.TrimSpace(params.Task) == "" {
		return "", errors.New("sub-agent task is required")
	}
	if parentInput.IsSubagent {
		return "", errors.New("sub-agent recursion not allowed")
	}
	if client == nil {
		return "", errors.New("llm client is required")
	}

	taskID, err := generateTaskID()
	if err != nil {
		return "", fmt.Errorf("generate task id: %w", err)
	}

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopParentAbort := wireParentAbort(parentInput.ParentAbort, cancel)
	defer stopParentAbort()

	var outputSink io.Writer
	var closeOutput func() error
	if params.Background {
		if strings.TrimSpace(params.SessionID) == "" {
			params.SessionID = "default"
		}
		outPath := paths.TaskOutputPath(params.SessionID, taskID)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
			return "", err
		}
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return "", err
		}
		outputSink = f
		closeOutput = f.Close
	}
	if closeOutput != nil {
		defer func() { _ = closeOutput() }()
	}
	if params.NotifyParent != nil {
		params.NotifyParent("sub-agent started: " + taskID)
		defer params.NotifyParent("sub-agent stopped: " + taskID)
	}

	permissionMode := params.PermissionMode.Normalize()
	if permissionMode == "" {
		permissionMode = permissions.ModeBubble
	}
	if parentInput.PermissionMode == permissions.ModeBypass || parentInput.PermissionMode == permissions.ModeDontAsk {
		permissionMode = parentInput.PermissionMode
	}

	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = parentInput.Model
	}
	systemPrompt := strings.TrimSpace(params.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = "You are a delegated sub-agent. Complete the assigned task and return concise results."
	}

	messages := []llm.Message{{Role: llm.RoleUser, Content: params.Task}}
	if params.Mode == SpawnFork {
		messages = append(append([]llm.Message(nil), parentInput.Messages...), messages...)
	}
	if len(params.Messages) > 0 {
		messages = append(append([]llm.Message(nil), params.Messages...), messages...)
	}

	childInput := Input{
		Model:            model,
		LLMProvider:      parentInput.LLMProvider,
		SystemPrompt:     systemPrompt,
		Messages:         messages,
		ToolContext:      parentInput.ToolContext,
		PermissionMode:   permissionMode,
		PermissionRules:  parentInput.PermissionRules,
		PermissionPrompt: parentInput.PermissionPrompt,
		HookDecision:     parentInput.HookDecision,
		PostToolUse:      parentInput.PostToolUse,
		PermissionDenied: parentInput.PermissionDenied,
		StopHook:         nil,
		IsSubagent:       true,
		ParentAbort:      parentInput.ParentAbort,
		OutputSink:       outputSink,
	}
	childInput.ToolContext.Context = childCtx
	childInput.ToolContext.IsSubagent = true

	cfg := baseConfig
	if cfg.MaxTurns > 0 && cfg.MaxTurns < DefaultConfig().MaxTurns {
		cfg.MaxTurns = DefaultConfig().MaxTurns
	}
	// MaxTurns == 0 means unlimited; only override with params if explicitly set.
	if params.MaxTurns > 0 {
		cfg.MaxTurns = params.MaxTurns
	}
	childAgent, err := New(client, registry, WithConfig(cfg))
	if err != nil {
		return "", err
	}

	events := childAgent.Run(childCtx, childInput)
	lastAssistant := ""
	sawTerminal := false
	for evt := range events {
		switch e := evt.(type) {
		case AssistantTextDelta:
			lastAssistant += e.Content
			writeJSONL(outputSink, map[string]any{
				"ts":      time.Now().UTC().Format(time.RFC3339Nano),
				"kind":    "text",
				"content": e.Content,
			})
		case AssistantThinkingDelta:
			writeJSONL(outputSink, map[string]any{
				"ts":       time.Now().UTC().Format(time.RFC3339Nano),
				"kind":     "thinking",
				"thinking": e.Thinking,
			})
		case ToolUseStart:
			writeJSONL(outputSink, map[string]any{
				"ts":    time.Now().UTC().Format(time.RFC3339Nano),
				"kind":  "tool_start",
				"id":    e.ID,
				"name":  e.Name,
				"input": e.Input,
			})
		case ToolUseProgress:
			writeJSONL(outputSink, map[string]any{
				"ts":   time.Now().UTC().Format(time.RFC3339Nano),
				"kind": "tool_progress",
				"id":   e.ID,
				"data": e.Data,
			})
		case ToolUseResult:
			errText := ""
			if e.Err != nil {
				errText = e.Err.Error()
			}
			writeJSONL(outputSink, map[string]any{
				"ts":     time.Now().UTC().Format(time.RFC3339Nano),
				"kind":   "tool_result",
				"id":     e.ID,
				"result": e.Result,
				"error":  errText,
			})
		case RetryNotice:
			writeJSONL(outputSink, map[string]any{
				"ts":              time.Now().UTC().Format(time.RFC3339Nano),
				"kind":            "retry",
				"attempt":         e.Attempt,
				"cause":           e.Cause,
				"retry_kind":      e.Kind,
				"done_reason":     e.DoneReason,
				"assistant_chars": e.AssistantChars,
				"thinking_chars":  e.ThinkingChars,
			})
		case CompactionStarted:
			writeJSONL(outputSink, map[string]any{
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"kind":           "compaction_started",
				"turn_count":     e.TurnCount,
				"context_tokens": e.ContextTokens,
			})
		case CompactionCompleted:
			writeJSONL(outputSink, map[string]any{
				"ts":     time.Now().UTC().Format(time.RFC3339Nano),
				"kind":   "compaction_completed",
				"result": e.Result,
			})
		case HookNotice:
			writeJSONL(outputSink, map[string]any{
				"ts":      time.Now().UTC().Format(time.RFC3339Nano),
				"kind":    "hook_notice",
				"message": e.Message,
			})
		case Terminal:
			sawTerminal = true
			writeJSONL(outputSink, map[string]any{
				"ts":     time.Now().UTC().Format(time.RFC3339Nano),
				"kind":   "terminal",
				"reason": string(e.Reason),
			})
			if e.Reason != TerminalCompleted {
				return "", fmt.Errorf("sub-agent ended with %s: %s", e.Reason, e.Detail)
			}
			if lastAssistant == "" {
				lastAssistant = lastAssistantFromConversation(e.Conversation)
			}
		}
	}
	if !sawTerminal {
		if err := childCtx.Err(); err != nil {
			return "", err
		}
		return "", errors.New("sub-agent terminated without terminal event")
	}

	if params.Background {
		return taskID, nil
	}
	if strings.TrimSpace(lastAssistant) == "" {
		return "Sub-agent completed with no output.", nil
	}
	return strings.TrimSpace(lastAssistant), nil
}

// RunSubagent is the exported entrypoint used by tool adapters.
func RunSubagent(
	ctx context.Context,
	parentInput Input,
	params SubagentParams,
	client llm.Client,
	registry *tools.Registry,
	baseConfig Config,
) (string, error) {
	return runSubagent(ctx, parentInput, params, client, registry, baseConfig)
}

func generateTaskID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func wireParentAbort(parentAbort <-chan struct{}, cancel context.CancelFunc) func() {
	if parentAbort == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-parentAbort:
			cancel()
		case <-done:
		}
	}()
	return func() { close(done) }
}

func writeJSONL(w io.Writer, payload map[string]any) {
	if w == nil {
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	bw := bufio.NewWriter(w)
	_, _ = bw.Write(append(b, '\n'))
	_ = bw.Flush()
}

func lastAssistantFromConversation(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleAssistant && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	return ""
}
