package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/permissions"
	"github.com/FernasFragas/nandocodego/internal/tools"
	"golang.org/x/sync/errgroup"
)

// SpeculativeExecutor manages concurrent batch execution of tool calls.
// It preserves submission order of results while allowing concurrent execution.
//
// NOTE: With the Ollama client, tool calls arrive only in the Done event
// (full JSON, not streaming fragments). This limits speculative execution
// to starting immediately after the stream closes. With providers that
// stream incremental tool call JSON, this executor could start earlier.
type SpeculativeExecutor struct {
	agent            *Agent
	toolCtx          tools.Context
	permissionPrompt func(context.Context, string) (tools.PermissionResult, error)
	hook             func(context.Context, ToolHookEvent)
	denyHook         func(context.Context, ToolHookEvent)
	batchObserver    ToolBatchObserverFunc
	concurrencyLimit int
	logger           *slog.Logger
}

type batchResult struct {
	Index    int
	ToolName string
	Result   tools.Result
	Err      error
	Input    any
	Target   string
}

// NewSpeculativeExecutor creates a new speculative executor.
func NewSpeculativeExecutor(
	agent *Agent,
	toolCtx tools.Context,
	concurrencyLimit int,
	logger *slog.Logger,
) *SpeculativeExecutor {
	if logger == nil {
		logger = defaultLogger()
	}
	if concurrencyLimit <= 0 {
		concurrencyLimit = 10
	}
	return &SpeculativeExecutor{
		agent:            agent,
		toolCtx:          toolCtx,
		concurrencyLimit: concurrencyLimit,
		logger:           logger,
	}
}

// SetPermissionPrompt sets the permission prompt callback.
func (se *SpeculativeExecutor) SetPermissionPrompt(fn func(context.Context, string) (tools.PermissionResult, error)) {
	se.permissionPrompt = fn
}

// SetHooks sets the tool hook callbacks.
func (se *SpeculativeExecutor) SetHooks(hook, denyHook func(context.Context, ToolHookEvent)) {
	se.hook = hook
	se.denyHook = denyHook
}

// SetBatchObserver sets a callback for per-batch execution metrics.
func (se *SpeculativeExecutor) SetBatchObserver(observer ToolBatchObserverFunc) {
	se.batchObserver = observer
}

// ExecuteBatches executes the tool calls in batches, running safe concurrent calls
// in parallel and unsafe calls serially. Results are returned in submission order.
func (se *SpeculativeExecutor) ExecuteBatches(
	ctx context.Context,
	batches []Batch,
	events chan<- Event,
) ([]llm.Message, int) {
	var messages []llm.Message
	toolCallCount := 0

	for _, batch := range batches {
		batchStart := time.Now()
		if batch.Safe && len(batch.Calls) > 1 {
			batchMessages, count := se.executeConcurrentBatch(ctx, batch, events)
			messages = append(messages, batchMessages...)
			toolCallCount += count
		} else {
			for _, ic := range batch.Calls {
				msg, _ := se.executeSingleCall(ctx, ic, events)
				messages = append(messages, msg...)
				toolCallCount++
			}
		}
		if se.batchObserver != nil {
			se.batchObserver(len(batch.Calls), batch.Safe, time.Since(batchStart))
		}
	}

	return messages, toolCallCount
}

// executeConcurrentBatch executes a batch of safe tool calls concurrently.
func (se *SpeculativeExecutor) executeConcurrentBatch(
	ctx context.Context,
	batch Batch,
	events chan<- Event,
) ([]llm.Message, int) {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(se.concurrencyLimit)

	results := make([]batchResult, len(batch.Calls))

	// Emit start events in submission order before launching the batch.
	for _, ic := range batch.Calls {
		sendEvent(ctx, events, ToolUseStart{
			ID:    ic.ToolID,
			Name:  ic.Call.Function.Name,
			Input: ic.Input,
		})
	}

	for i, ic := range batch.Calls {
		i, ic := i, ic
		eg.Go(func() error {
			toolCtx := se.toolCtx
			toolCtx.Context = egCtx
			result, err := ic.Tool.Call(toolCtx, ic.Input, nil)
			results[i] = batchResult{
				Index:    ic.Index,
				ToolName: ic.Call.Function.Name,
				Result:   result,
				Err:      err,
				Input:    ic.Input,
				Target:   permissions.ExtractTarget(ic.Input),
			}
			return nil
		})
	}

	_ = eg.Wait()

	var messages []llm.Message
	for i, br := range results {
		sendEvent(ctx, events, ToolUseResult{
			ID:     batch.Calls[i].ToolID,
			Result: br.Result,
			Err:    br.Err,
		})

		if se.hook != nil {
			se.hook(ctx, ToolHookEvent{
				ToolName:       br.ToolName,
				Input:          br.Input,
				Target:         br.Target,
				Result:         br.Result,
				Err:            br.Err,
				ToolContext:    se.toolCtx,
				PermissionMode: permissions.FromToolsMode(se.toolCtx.PermissionMode),
			})
		}

		content := se.formatToolResult(br.Result, br.Err)
		messages = append(messages, llm.Message{
			Role:     llm.RoleTool,
			ToolName: br.ToolName,
			Content:  content,
		})
	}

	return messages, len(batch.Calls)
}

// executeSingleCall executes a single tool call and returns the result messages.
func (se *SpeculativeExecutor) executeSingleCall(
	ctx context.Context,
	ic indexedCall,
	events chan<- Event,
) ([]llm.Message, error) {
	tool := ic.Tool
	call := ic.Call

	sendEvent(ctx, events, ToolUseStart{
		ID:    ic.ToolID,
		Name:  call.Function.Name,
		Input: ic.Input,
	})

	toolCtx := se.toolCtx
	toolCtx.Context = ctx
	result, err := tool.Call(toolCtx, ic.Input, nil)
	sendEvent(ctx, events, ToolUseResult{
		ID:     ic.ToolID,
		Result: result,
		Err:    err,
	})

	if se.hook != nil {
		se.hook(ctx, ToolHookEvent{
			ToolName:       call.Function.Name,
			Input:          ic.Input,
			Target:         permissions.ExtractTarget(ic.Input),
			Result:         result,
			Err:            err,
			ToolContext:    se.toolCtx,
			PermissionMode: permissions.FromToolsMode(se.toolCtx.PermissionMode),
		})
	}

	content := se.formatToolResult(result, err)

	resultMsg := llm.Message{
		Role:     llm.RoleTool,
		ToolName: call.Function.Name,
		Content:  content,
	}

	return []llm.Message{resultMsg}, err
}

func (se *SpeculativeExecutor) formatToolResult(result tools.Result, err error) string {
	if se.agent != nil {
		return se.agent.formatToolResult(result, err, se.toolCtx)
	}
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if result.Display != "" {
		return result.Display
	}
	return "Tool completed successfully"
}
