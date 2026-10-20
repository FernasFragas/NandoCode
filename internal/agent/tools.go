package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/permissions"
	"github.com/FernasFragas/Nandocode/internal/tools"
	"github.com/FernasFragas/Nandocode/internal/tools/builtin"
)

type permissionPromptAware interface {
	SetPermissionPrompt(permissions.PromptFunc)
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

// callToolWithProgress runs the tool and forwards its progress as
// ToolUseProgress events. It returns only after every progress event has been
// sent, so no progress event can arrive after the caller emits the result.
func callToolWithProgress(
	ctx context.Context,
	tool tools.Tool,
	toolID string,
	input any,
	toolCtx tools.Context,
	events chan<- Event,
) (tools.Result, error) {
	progress := make(chan tools.ProgressEvent, 8)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for prog := range progress {
			sendEvent(ctx, events, ToolUseProgress{
				ID:   toolID,
				Data: prog,
			})
		}
	}()

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
