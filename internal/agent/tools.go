package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/builtin"
)

type permissionPromptAware interface {
	SetPermissionPrompt(permissions.PromptFunc)
}

// executeToolCalls executes tool calls serially and returns tool-result messages.
func (a *Agent) executeToolCalls(
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

	for _, call := range toolCalls {
		toolID := a.nextToolID()
		toolCallCount++

		// Look up tool
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
			continue
		}

		// Marshal arguments to JSON
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
			continue
		}

		// Parse input
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
			continue
		}

		// Emit tool start
		sendEvent(ctx, events, ToolUseStart{
			ID:    toolID,
			Name:  call.Function.Name,
			Input: parsedInput,
		})
		if ppTool, ok := tool.(permissionPromptAware); ok {
			ppTool.SetPermissionPrompt(permissionPrompt)
		}

		// Resolve permissions using the new Phase 5 resolver.
		permResult := permissions.Resolve(ctx, permissions.Request{
			Mode:         permissionMode,
			Rules:        permissionRules,
			Tool:         tool,
			ToolName:     call.Function.Name,
			Input:        parsedInput,
			ToolContext:  toolCtx,
			HookDecision: hookDecision,
			Prompt:       permissionPrompt,
			Classifier:   nil, // No auto-classifier in Phase 5
			Observer:     a.config.PermissionObserver,
		})

		// Use UpdatedInput if the resolver transformed it.
		if permResult.UpdatedInput != nil {
			parsedInput = permResult.UpdatedInput
		}

		if permResult.Decision != permissions.DecisionAllow {
			// Not allowed.
			reason := permResult.Reason
			if reason == "" {
				if permResult.Decision == permissions.DecisionDeny {
					reason = "tool execution denied by policy"
				} else {
					reason = "tool requires interactive permission"
				}
			}
			sendEvent(ctx, events, ToolUseResult{
				ID:  toolID,
				Err: fmt.Errorf("permission denied: %s", reason),
			})
			if permissionDenied != nil {
				permissionDenied(ctx, ToolHookEvent{
					ToolName:       call.Function.Name,
					Input:          parsedInput,
					Target:         permissions.ExtractTarget(parsedInput),
					Err:            fmt.Errorf("permission denied: %s", reason),
					ToolContext:    toolCtx,
					PermissionMode: permissionMode,
					Model:          model,
				})
			}
			messages = append(messages, llm.Message{
				Role:     llm.RoleTool,
				ToolName: call.Function.Name,
				Content:  fmt.Sprintf("Permission denied: %s", reason),
			})
			continue
		}

		// Execute tool
		result, execErr := a.executeTool(ctx, tool, toolID, parsedInput, toolCtx, events)
		if postToolUse != nil {
			postToolUse(ctx, ToolHookEvent{
				ToolName:       call.Function.Name,
				Input:          parsedInput,
				Target:         permissions.ExtractTarget(parsedInput),
				Result:         result,
				Err:            execErr,
				ToolContext:    toolCtx,
				PermissionMode: permissionMode,
				Model:          model,
			})
		}

		// Emit result
		sendEvent(ctx, events, ToolUseResult{
			ID:     toolID,
			Result: result,
			Err:    execErr,
		})

		// Build tool-result message
		content := a.formatToolResult(result, execErr, toolCtx)
		messages = append(messages, llm.Message{
			Role:     llm.RoleTool,
			ToolName: call.Function.Name,
			Content:  content,
		})
	}

	return messages, toolCallCount
}

// lookupTool finds a tool by canonical name or alias.
func (a *Agent) lookupTool(name string) (tools.Tool, bool) {
	return a.lookupToolFromRegistry(a.tools, name)
}

func (a *Agent) lookupToolFromRegistry(registry *tools.Registry, name string) (tools.Tool, bool) {
	if registry == nil {
		return nil, false
	}
	return registry.Lookup(name)
}

func (a *Agent) effectiveRegistry(toolsetName string) *tools.Registry {
	normalized := strings.TrimSpace(strings.ToLower(toolsetName))
	if normalized == "" || normalized == ToolsetDefault {
		return a.tools
	}
	if normalized == ToolsetReadOnly {
		reg, err := builtin.NewReadOnlyRegistry()
		if err != nil {
			a.logger.Warn("failed to initialize read-only tool registry", "error", err)
			return nil
		}
		return reg
	}
	a.logger.Warn("unknown toolset requested, falling back to default", "toolset", toolsetName)
	return a.tools
}

// executeTool runs the tool and forwards progress events.
func (a *Agent) executeTool(
	ctx context.Context,
	tool tools.Tool,
	toolID string,
	input any,
	toolCtx tools.Context,
	events chan<- Event,
) (tools.Result, error) {
	// Create buffered progress channel
	progress := make(chan tools.ProgressEvent, 8)
	done := make(chan struct{})

	// Forward progress events
	go func() {
		defer close(done)
		for prog := range progress {
			sendEvent(ctx, events, ToolUseProgress{
				ID:   toolID,
				Data: prog,
			})
		}
	}()

	// Execute tool
	result, err := tool.Call(toolCtx, input, progress)
	close(progress)
	<-done

	return result, err
}

// formatToolResult converts a tool result to model-readable content.
func (a *Agent) formatToolResult(result tools.Result, err error, toolCtx tools.Context) string {
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	// Prefer display text
	if result.Display != "" {
		display, _ := tools.TruncateDisplay(result.Display, toolCtx.EffectiveMaxResultChars())
		return display
	}

	// Fallback to JSON marshal of data
	if result.Data != nil {
		dataJSON, marshalErr := json.MarshalIndent(result.Data, "", "  ")
		if marshalErr == nil {
			display, _ := tools.TruncateDisplay(string(dataJSON), toolCtx.EffectiveMaxResultChars())
			return display
		}
	}

	return "Tool completed successfully"
}
