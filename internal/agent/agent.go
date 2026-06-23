package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// Agent coordinates LLM chat with tool execution.
type Agent struct {
	client llm.Client
	tools  *tools.Registry
	config Config
	logger *slog.Logger
	toolID uint64
}

// Option is a functional option for Agent construction.
type Option func(*Agent)

// WithConfig sets the agent configuration.
func WithConfig(cfg Config) Option {
	return func(a *Agent) {
		a.config = cfg
	}
}

// WithLogger sets the agent logger.
func WithLogger(logger *slog.Logger) Option {
	return func(a *Agent) {
		a.logger = logger
	}
}

// WithWatchdog sets the watchdog configuration.
func WithWatchdog(wd llm.WatchdogConfig) Option {
	return func(a *Agent) {
		a.config.Watchdog = wd
	}
}

// WithCompactionConfig sets the compaction configuration.
func WithCompactionConfig(cfg CompactionConfig) Option {
	return func(a *Agent) {
		a.config.Compaction = cfg
	}
}

// New constructs an Agent with the given LLM client and tool registry.
// The client is required; a nil registry means no tools are available.
func New(client llm.Client, registry *tools.Registry, opts ...Option) (*Agent, error) {
	if client == nil {
		return nil, errors.New("llm client is required")
	}
	a := &Agent{
		client: client,
		tools:  registry,
		config: DefaultConfig(),
		logger: defaultLogger(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// Run executes the agent loop and returns a channel of events.
// The channel is closed after exactly one Terminal event is emitted.
// The caller should drain the channel to completion or cancel the context.
func (a *Agent) Run(ctx context.Context, in Input) <-chan Event {
	events := make(chan Event, 16)
	go func() {
		defer close(events)
		a.run(ctx, in, events)
	}()
	return events
}

// run is the internal implementation of the agent loop.
func (a *Agent) run(ctx context.Context, in Input, events chan<- Event) {
	if err := validateInput(ctx, &in); err != nil {
		sendEvent(ctx, events, Terminal{
			Reason:       TerminalUnrecoverable,
			Detail:       err.Error(),
			Conversation: nil,
		})
		return
	}

	// Copy message history to avoid mutating caller's slice
	history := append([]llm.Message(nil), in.Messages...)
	if in.HistoryPolicy == HistoryPolicyLatestOnly {
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == llm.RoleUser {
				history = []llm.Message{history[i]}
				break
			}
		}
	}

	// Prepend system prompt if provided
	if in.SystemPrompt != "" {
		history = append([]llm.Message{{
			Role:    llm.RoleSystem,
			Content: in.SystemPrompt,
		}}, history...)
	}

	usage := Usage{}
	outputTokenBudget := a.config.MaxOutputTokens
	if in.MaxOutputTokens > 0 {
		outputTokenBudget = in.MaxOutputTokens
	}
	lengthRetryCount := 0
	incompleteAssistantRetryCount := 0
	compactionAttempted := false
	lastContextTokens := int64(0)
	addedConversation := make([]llm.Message, 0, 16)
	in.ToolContext.IsSubagent = in.IsSubagent
	effectiveRegistry := a.effectiveRegistry(in.ToolsetName)
	cfg := a.config.Compaction

	for turn := 1; a.config.MaxTurns == 0 || turn <= a.config.MaxTurns; turn++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			sendEventForce(ctx, events, Terminal{
				Reason:       TerminalAborted,
				Detail:       ctx.Err().Error(),
				Usage:        usage,
				Conversation: append([]llm.Message(nil), addedConversation...),
			})
			return
		default:
		}

		// Handle manual compact request (non-blocking)
		if in.CompactRequest != nil {
			select {
			case <-in.CompactRequest:
				history, _ = a.doCompact(ctx, cfg, history, lastContextTokens, events, in.Model)
			default:
			}
		}

		usage.Turns = turn
		sendEvent(ctx, events, AssistantTurnStarted{Turn: turn})
		runNumCtx := effectiveNumCtx(a.config, in, in.Model, history, outputTokenBudget)
		inputBudget := runNumCtx - outputTokenBudget - a.config.ContextReserve
		packedHistory := history
		var lastPackReport *PromptPackReport
		if inputBudget > 0 {
			packStart := time.Now()
			packed := packPromptHistory(history, inputBudget)
			packDuration := time.Since(packStart)
			packedHistory = packed.Messages
			rep := packed.Report
			rep.HistoryPolicy = in.HistoryPolicy
			rep.Intent = in.PromptIntent
			rep.AttachmentPolicy = in.AttachmentPolicy
			if strings.EqualFold(strings.TrimSpace(in.PromptIntent), "directory_listing") || strings.EqualFold(strings.TrimSpace(in.AttachmentPolicy), "listing_tree_only") {
				rep.MemoryPolicy = "skipped_listing_intent"
			} else {
				rep.MemoryPolicy = "default"
			}
			rep.RetryPolicy = "anchored_original_request"
			rep.DirectoryTreeAttached = strings.Contains(latestUserContent(packedHistory), "Directory tree data:")
			rep.IncludedFileBodies = historyFileBodyCount(packedHistory)
			lastPackReport = &rep
			if packed.Report.SkippedMessages > 0 {
				sendEvent(ctx, events, PromptPackReport(rep))
				sendEvent(ctx, events, StageTiming{Stage: "prompt_pack", Duration: packDuration})
			}
		}

		// Execute one model turn
		result, err := a.executeOneTurn(ctx, in.Model, in.LLMProvider, packedHistory, effectiveRegistry, in.ToolContext, in.PermissionMode, in.PermissionRules, in.PermissionPrompt, in.HookDecision, in.PostToolUse, in.PermissionDenied, outputTokenBudget, runNumCtx, lastPackReport, in.PromptIntent, in.AttachmentPolicy, in.HistoryPolicy, in.EvidencePack, events)
		if err != nil {
			// Map internal errors to terminal reasons
			reason, detail := classifyError(err)
			usage.PromptEvalCount += result.PromptEvalCount
			usage.EvalCount += result.EvalCount
			usage.TotalDuration += result.TotalDuration
			usage.ToolCalls += result.ToolCallsInTurn
			sendEvent(ctx, events, Terminal{
				Reason:       reason,
				Detail:       detail,
				Usage:        usage,
				Conversation: append([]llm.Message(nil), addedConversation...),
			})
			return
		}

		// Update usage
		usage.PromptEvalCount += result.PromptEvalCount
		usage.EvalCount += result.EvalCount
		usage.TotalDuration += result.TotalDuration
		usage.ToolCalls += result.ToolCallsInTurn
		usage.DoneReason = result.DoneReason
		lastContextTokens = result.PromptEvalCount

		// Handle done reason
		if result.DoneReason == "length" {
			lengthRetryCount++
			if lengthRetryCount == 1 {
				// Retry with expanded output token budget
				outputTokenBudget = a.config.LengthRetryTokens
				sendEvent(ctx, events, RetryNotice{
					Attempt:        lengthRetryCount,
					Cause:          "context length exceeded, retrying with expanded token budget",
					Kind:           "length",
					DoneReason:     result.DoneReason,
					AssistantChars: len(result.AssistantMessage.Content),
					ThinkingChars:  len(result.AssistantMessage.Thinking),
				})
				turn-- // Don't count this as a turn
				continue
			}
			if lengthRetryCount == 2 && !cfg.Disabled && !compactionAttempted {
				// Try reactive compaction
				compactionAttempted = true
				var compacted bool
				history, compacted = a.doCompact(ctx, cfg, history, lastContextTokens, events, in.Model)
				if compacted {
					if in.MaxOutputTokens > 0 { // reset to per-run budget
						outputTokenBudget = in.MaxOutputTokens
					} else {
						outputTokenBudget = a.config.MaxOutputTokens
					}
					turn-- // don't count as a turn
					continue
				}
				// compaction skipped (not enough turns) — fall through to TerminalContextOverflow
			}
			// Third length or compaction also failed
			sendEvent(ctx, events, Terminal{
				Reason:       TerminalContextOverflow,
				Detail:       "context length exceeded after retry and compaction",
				Usage:        usage,
				Conversation: append([]llm.Message(nil), addedConversation...),
			})
			return
		}
		if isTerminalStreamFailure(result.DoneReason) {
			sendEvent(ctx, events, Terminal{
				Reason:       TerminalUnrecoverable,
				Detail:       streamFailureDetail(result.DoneReason),
				Usage:        usage,
				Conversation: append([]llm.Message(nil), addedConversation...),
			})
			return
		}

		// Reset length retry count on success
		lengthRetryCount = 0

		// Proactive compaction check
		if !cfg.Disabled && cfg.MaxContextTokens > 0 && cfg.Threshold > 0 {
			if float64(lastContextTokens) > float64(cfg.MaxContextTokens)*cfg.Threshold {
				if countTurns(history) >= cfg.MinTurns {
					history, _ = a.doCompact(ctx, cfg, history, lastContextTokens, events, in.Model)
				}
			}
		}

		// Append assistant message to history
		history = append(history, result.AssistantMessage)
		addedConversation = append(addedConversation, result.AssistantMessage)

		// If no tool calls, we're done
		if len(result.ToolCallMessages) == 0 {
			if in.PendingMessagesProvider != nil {
				pending := in.PendingMessagesProvider(ctx)
				if len(pending) > 0 {
					history = append(history, pending...)
					addedConversation = append(addedConversation, pending...)
					turn--
					continue
				}
			}
			if shouldRetryIncompleteAssistantResponse(result.AssistantMessage, result.DoneReason) && incompleteAssistantRetryCount < incompleteAssistantRetryLimit {
				incompleteAssistantRetryCount++
				retryPrompt, ok := buildIncompleteAssistantRetryPrompt(incompleteRetryInput{
					PromptIntent:         in.PromptIntent,
					AttachmentPolicy:     in.AttachmentPolicy,
					OriginalUserText:     in.OriginalUserText,
					LastUserContent:      latestUserContent(history),
					LastAssistantContent: result.AssistantMessage.Content,
				})
				if !ok {
					sendEvent(ctx, events, Terminal{
						Reason:       TerminalCompleted,
						Usage:        usage,
						Conversation: append([]llm.Message(nil), addedConversation...),
					})
					return
				}
				history = append(history, llm.Message{Role: llm.RoleUser, Content: retryPrompt})
				sendEvent(ctx, events, RetryNotice{
					Attempt:        incompleteAssistantRetryCount,
					Cause:          "assistant response looked incomplete, requesting an anchored continuation",
					Kind:           "incomplete_assistant_response",
					DoneReason:     result.DoneReason,
					AssistantChars: len(result.AssistantMessage.Content),
					ThinkingChars:  len(result.AssistantMessage.Thinking),
				})
				turn-- // Treat the continuation request as part of the same user-visible turn.
				continue
			}
			if in.StopHook != nil {
				if reason, blocked := in.StopHook(ctx, append([]llm.Message(nil), addedConversation...)); blocked {
					sendEvent(ctx, events, HookNotice{Message: reason})
					sendEvent(ctx, events, Terminal{
						Reason:       TerminalStopHook,
						Detail:       reason,
						Usage:        usage,
						Conversation: append([]llm.Message(nil), addedConversation...),
					})
					return
				}
			}
			sendEvent(ctx, events, Terminal{
				Reason:       TerminalCompleted,
				Usage:        usage,
				Conversation: append([]llm.Message(nil), addedConversation...),
			})
			return
		}

		// Append tool result messages and continue
		history = append(history, result.ToolCallMessages...)
		addedConversation = append(addedConversation, result.ToolCallMessages...)
		if in.PendingMessagesProvider != nil {
			pending := in.PendingMessagesProvider(ctx)
			if len(pending) > 0 {
				history = append(history, pending...)
				addedConversation = append(addedConversation, pending...)
			}
		}
	}

	// Max turns reached
	sendEvent(ctx, events, Terminal{
		Reason:       TerminalMaxTurns,
		Detail:       "exceeded maximum turn count",
		Usage:        usage,
		Conversation: append([]llm.Message(nil), addedConversation...),
	})
}

func latestUserContent(history []llm.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == llm.RoleUser {
			return history[i].Content
		}
	}
	return ""
}

func historyFileBodyCount(history []llm.Message) int {
	count := 0
	for i := range history {
		count += strings.Count(history[i].Content, "<file ")
	}
	return count
}

// doCompact runs compaction and emits the appropriate events.
// Returns the new message slice and whether compaction was effectively applied.
func (a *Agent) doCompact(ctx context.Context, cfg CompactionConfig, messages []llm.Message, contextTokens int64, events chan<- Event, model string) ([]llm.Message, bool) {
	turnCount := countTurns(messages)
	sendEvent(ctx, events, CompactionStarted{TurnCount: turnCount, ContextTokens: contextTokens})

	result := Compact(ctx, a.client, cfg, model, messages)

	sendEvent(ctx, events, CompactionCompleted{Result: result})

	a.logger.Debug("compaction complete",
		"before", result.Before,
		"after", result.After,
		"layer", result.Layer,
		"skipped", result.Skipped,
	)

	if result.Skipped || result.Messages == nil {
		return messages, false
	}
	return result.Messages, true
}

// executeToolCallsConcurrent executes tool calls with speculative concurrency.
// It handles all cases from executeToolCalls (unknown tools, parse errors, permissions)
// while executing safe concurrent calls in parallel.
func (a *Agent) executeToolCallsConcurrent(
	ctx context.Context,
	registry *tools.Registry,
	toolCalls []llm.ToolCall,
	toolCtx tools.Context,
	permissionMode permissions.Mode,
	permissionRules permissions.Rules,
	permissionPrompt permissions.PromptFunc,
	hookDecision permissions.HookDecisionFunc,
	postToolUse ToolHookFunc,
	permissionDenied ToolHookFunc,
	model string,
	events chan<- Event,
) ([]llm.Message, int) {
	var messages []llm.Message
	toolCallCount := 0

	// Build indexed calls with tool lookups and error handling
	indexed := make([]indexedCall, 0, len(toolCalls))
	for i, call := range toolCalls {
		toolID := a.nextToolID()

		tool, found := a.lookupToolFromRegistry(registry, call.Function.Name)
		if !found {
			// Unknown tool
			sendEvent(ctx, events, ToolUseStart{
				ID:    toolID,
				Name:  call.Function.Name,
				Input: call.Function.Arguments,
			})
			sendEvent(ctx, events, ToolUseResult{
				ID:  toolID,
				Err: fmt.Errorf("unknown tool: %s", call.Function.Name),
			})
			messages = append(messages, llm.Message{
				Role:     llm.RoleTool,
				ToolName: call.Function.Name,
				Content:  fmt.Sprintf("Error: unknown tool '%s'", call.Function.Name),
			})
			toolCallCount++
			continue
		}

		// Parse input
		argsJSON, err := json.Marshal(call.Function.Arguments)
		if err != nil {
			sendEvent(ctx, events, ToolUseStart{
				ID:    toolID,
				Name:  call.Function.Name,
				Input: call.Function.Arguments,
			})
			sendEvent(ctx, events, ToolUseResult{
				ID:  toolID,
				Err: fmt.Errorf("failed to marshal tool arguments: %w", err),
			})
			messages = append(messages, llm.Message{
				Role:     llm.RoleTool,
				ToolName: call.Function.Name,
				Content:  fmt.Sprintf("Error: failed to marshal arguments: %v", err),
			})
			toolCallCount++
			continue
		}

		parsedInput, err := tool.UnmarshalInput(argsJSON)
		if err != nil {
			sendEvent(ctx, events, ToolUseStart{
				ID:    toolID,
				Name:  call.Function.Name,
				Input: call.Function.Arguments,
			})
			sendEvent(ctx, events, ToolUseResult{
				ID:  toolID,
				Err: fmt.Errorf("failed to parse tool input: %w", err),
			})
			messages = append(messages, llm.Message{
				Role:     llm.RoleTool,
				ToolName: call.Function.Name,
				Content:  fmt.Sprintf("Error: invalid input format: %v", err),
			})
			toolCallCount++
			continue
		}

		indexed = append(indexed, indexedCall{
			Index:  i,
			ToolID: toolID,
			Call:   call,
			Tool:   tool,
			Input:  parsedInput,
		})
		toolCallCount++
	}

	if len(indexed) == 0 {
		return messages, toolCallCount
	}

	// Check permissions for all indexed calls before concurrent execution
	var permittedCalls []indexedCall
	for _, ic := range indexed {
		if ppTool, ok := ic.Tool.(permissionPromptAware); ok {
			ppTool.SetPermissionPrompt(permissionPrompt)
		}

		permResult := permissions.Resolve(ctx, permissions.Request{
			Mode:         permissionMode,
			Rules:        permissionRules,
			Tool:         ic.Tool,
			ToolName:     ic.Call.Function.Name,
			Input:        ic.Input,
			ToolContext:  toolCtx,
			HookDecision: hookDecision,
			Prompt:       permissionPrompt,
			Classifier:   nil,
			Observer:     a.config.PermissionObserver,
		})

		if permResult.UpdatedInput != nil {
			ic.Input = permResult.UpdatedInput
		}

		if permResult.Decision != permissions.DecisionAllow {
			// Permission denied
			reason := permResult.Reason
			if reason == "" {
				if permResult.Decision == permissions.DecisionDeny {
					reason = "tool execution denied by policy"
				} else {
					reason = "tool requires interactive permission"
				}
			}

			sendEvent(ctx, events, ToolUseResult{
				ID:  ic.ToolID,
				Err: fmt.Errorf("permission denied: %s", reason),
			})

			if permissionDenied != nil {
				permissionDenied(ctx, ToolHookEvent{
					ToolName:       ic.Call.Function.Name,
					Input:          ic.Input,
					Target:         permissions.ExtractTarget(ic.Input),
					Err:            fmt.Errorf("permission denied: %s", reason),
					ToolContext:    toolCtx,
					PermissionMode: permissionMode,
					Model:          model,
				})
			}

			messages = append(messages, llm.Message{
				Role:     llm.RoleTool,
				ToolName: ic.Call.Function.Name,
				Content:  fmt.Sprintf("Permission denied: %s", reason),
			})
			continue
		}

		permittedCalls = append(permittedCalls, ic)
	}

	if len(permittedCalls) == 0 {
		return messages, toolCallCount
	}

	// Partition by safety and execute concurrently
	batches := Partition(permittedCalls)
	limit := a.config.MaxConcurrentTools
	if limit <= 0 {
		limit = 10
	}
	executor := NewSpeculativeExecutor(a, toolCtx, limit, a.logger)

	// Store executor hooks for concurrent execution
	executor.SetHooks(postToolUse, permissionDenied)
	executor.SetBatchObserver(a.config.ToolBatchObserver)

	executedMessages, _ := executor.ExecuteBatches(ctx, batches, events)
	messages = append(messages, executedMessages...)

	return messages, toolCallCount
}

func defaultLogger() *slog.Logger {
	return slog.Default()
}

func (a *Agent) nextToolID() string {
	id := atomic.AddUint64(&a.toolID, 1) - 1
	return fmt.Sprintf("tool-%d", id)
}

func sendEvent(ctx context.Context, events chan<- Event, evt Event) {
	select {
	case events <- evt:
	case <-ctx.Done():
	}
}

func sendEventForce(ctx context.Context, events chan<- Event, evt Event) {
	select {
	case events <- evt:
		return
	default:
	}
	select {
	case events <- evt:
	case <-ctx.Done():
	}
}

func isTerminalStreamFailure(doneReason string) bool {
	return doneReason == "watchdog_timeout" || strings.HasPrefix(doneReason, "stream_error:")
}

func streamFailureDetail(doneReason string) string {
	if doneReason == "watchdog_timeout" {
		return "llm stream watchdog timeout"
	}
	return doneReason
}
