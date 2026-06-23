package agent

import (
	"context"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// turnResult captures the outcome of one model turn.
type turnResult struct {
	AssistantMessage llm.Message
	ToolCallMessages []llm.Message
	DoneReason       string
	PromptEvalCount  int64
	EvalCount        int64
	TotalDuration    int64
	ToolCallsInTurn  int
}

// executeOneTurn runs one model turn: streams the response, accumulates the message,
// executes any tool calls, and returns the result.
func (a *Agent) executeOneTurn(
	ctx context.Context,
	model string,
	llmProvider string,
	history []llm.Message,
	registry *tools.Registry,
	toolCtx tools.Context,
	permissionMode permissions.Mode,
	permissionRules permissions.Rules,
	permissionPrompt permissions.PromptFunc,
	hookDecision permissions.HookDecisionFunc,
	postToolUse ToolHookFunc,
	permissionDenied ToolHookFunc,
	outputTokenBudget int,
	effectiveNumCtx int,
	promptPackReport *PromptPackReport,
	promptIntent string,
	attachmentPolicy string,
	historyPolicy string,
	evidencePack *EvidencePackReport,
	events chan<- Event,
) (turnResult, error) {
	// Build enabled tool definitions
	enabledTools := a.buildToolDefs(registry, toolCtx)

	// Build chat request
	req := &llm.ChatRequest{
		Model:    model,
		Messages: history,
		Tools:    enabledTools,
		Stream:   true,
		Options: map[string]any{
			"num_predict": outputTokenBudget,
			"num_ctx":     effectiveNumCtx,
		},
	}
	if cap := llm.ModelCapabilities(model); cap.SupportsThinking {
		req.Think = true
	}
	if strings.TrimSpace(a.config.ChatKeepAlive) != "" {
		req.KeepAlive = strings.TrimSpace(a.config.ChatKeepAlive)
	}
	recordPromptDump(req, promptPackReport, promptIntent, attachmentPolicy, historyPolicy, evidencePack, toolCtx.PromptDumpMode, toolCtx.PromptDumpKeep, toolCtx.PromptPreviewChars)

	// Call LLM
	requestStartedAt := time.Now()
	sendEvent(ctx, events, LLMRequestStarted{})
	stream, err := a.client.Chat(ctx, req)
	if err != nil {
		return turnResult{}, err
	}
	sendEvent(ctx, events, LLMStreamOpened{Latency: time.Since(requestStartedAt)})

	// Wrap with watchdog
	watchdogCfg := a.config.watchdogForProvider(llmProvider)
	if watchdogCfg.IdleWarningTimeout > 0 {
		prevWarn := watchdogCfg.OnIdleWarning
		watchdogCfg = watchdogCfg.WithIdleWarning(watchdogCfg.IdleWarningTimeout, func() {
			if prevWarn != nil {
				prevWarn()
			}
			sendEvent(ctx, events, LLMIdleWarning{
				Provider: llmProvider,
				Timeout:  watchdogCfg.IdleWarningTimeout,
			})
		})
	}
	watchedStream, cancelWatchdog := llm.WatchStream(ctx, stream, watchdogCfg)
	defer cancelWatchdog()

	// Accumulate the turn
	return a.accumulateTurn(ctx, watchedStream, registry, toolCtx, permissionMode, permissionRules, permissionPrompt, hookDecision, postToolUse, permissionDenied, model, events, requestStartedAt)
}

// accumulateTurn reads from the stream, emits deltas, accumulates the final message,
// and executes tool calls if present.
func (a *Agent) accumulateTurn(
	ctx context.Context,
	stream <-chan llm.StreamEvent,
	registry *tools.Registry,
	toolCtx tools.Context,
	permissionMode permissions.Mode,
	permissionRules permissions.Rules,
	permissionPrompt permissions.PromptFunc,
	hookDecision permissions.HookDecisionFunc,
	postToolUse ToolHookFunc,
	permissionDenied ToolHookFunc,
	model string,
	events chan<- Event,
	requestStartedAt time.Time,
) (turnResult, error) {
	var contentBuf strings.Builder
	var thinkingBuf strings.Builder
	var toolCalls []llm.ToolCall
	var usage turnResult
	firstTokenSent := false

	for evt := range stream {
		if !firstTokenSent && (evt.Message.Content != "" || evt.Message.Thinking != "" || len(evt.Message.ToolCalls) > 0) {
			firstTokenSent = true
			sendEvent(ctx, events, FirstTokenReceived{Latency: time.Since(requestStartedAt)})
		}
		// Emit text delta
		if evt.Message.Content != "" {
			sendEvent(ctx, events, AssistantTextDelta{Content: evt.Message.Content})
			contentBuf.WriteString(evt.Message.Content)
		}

		// Emit thinking delta
		if evt.Message.Thinking != "" {
			sendEvent(ctx, events, AssistantThinkingDelta{Thinking: evt.Message.Thinking})
			thinkingBuf.WriteString(evt.Message.Thinking)
		}

		// Capture latest tool calls
		if len(evt.Message.ToolCalls) > 0 {
			toolCalls = evt.Message.ToolCalls
		}

		// Capture done event
		if evt.Done {
			usage.DoneReason = evt.DoneReason
			usage.PromptEvalCount = evt.PromptEvalCount
			usage.EvalCount = evt.EvalCount
			usage.TotalDuration = evt.TotalDuration
		}
	}

	// Build assistant message
	assistantMsg := llm.Message{
		Role:      llm.RoleAssistant,
		Content:   contentBuf.String(),
		Thinking:  thinkingBuf.String(),
		ToolCalls: toolCalls,
	}

	usage.AssistantMessage = assistantMsg

	// Execute tool calls if present (using concurrent execution)
	if len(toolCalls) > 0 {
		toolMessages, toolCallCount := a.executeToolCallsConcurrent(ctx, registry, toolCalls, toolCtx, permissionMode, permissionRules, permissionPrompt, hookDecision, postToolUse, permissionDenied, model, events)
		usage.ToolCallMessages = toolMessages
		usage.ToolCallsInTurn = toolCallCount
	}

	return usage, nil
}

// buildToolDefs converts enabled tools to LLM tool definitions.
func (a *Agent) buildToolDefs(registry *tools.Registry, toolCtx tools.Context) []llm.ToolDef {
	if strings.EqualFold(strings.TrimSpace(toolModeFromContext(toolCtx.Context)), ToolModeNone) {
		return nil
	}
	if registry == nil {
		return nil
	}

	var defs []llm.ToolDef
	for _, tool := range registry.All() {
		if !tool.IsEnabled(toolCtx) {
			continue
		}
		def, err := tools.ToLLMToolDef(tool)
		if err != nil {
			a.logger.Warn("failed to convert tool to LLM definition",
				"tool", tool.Name(),
				"error", err)
			continue
		}
		defs = append(defs, def)
	}
	return defs
}
