package observability

import (
	"encoding/json"
	"time"

	"github.com/FernasFragas/nandocodego/internal/permissions"
	"github.com/FernasFragas/nandocodego/internal/tools"
)

type observedTool struct {
	next   tools.Tool
	meter  *Meter
	bridge Bridge
}

// WrapTool decorates a single tool with call timing and error counters.
func WrapTool(next tools.Tool, meter *Meter, bridge Bridge) tools.Tool {
	if next == nil {
		return nil
	}
	if bridge == nil {
		bridge = noopBridge{}
	}
	return &observedTool{next: next, meter: meter, bridge: bridge}
}

// WrapRegistry returns a new registry containing decorated copies of every tool in src.
func WrapRegistry(src *tools.Registry, meter *Meter, bridge Bridge) (*tools.Registry, error) {
	if src == nil {
		return nil, nil
	}
	out := tools.NewRegistry()
	for _, t := range src.All() {
		if err := out.Register(WrapTool(t, meter, bridge)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (t *observedTool) Name() string               { return t.next.Name() }
func (t *observedTool) Description() string        { return t.next.Description() }
func (t *observedTool) Aliases() []string          { return t.next.Aliases() }
func (t *observedTool) JSONSchema() map[string]any { return t.next.JSONSchema() }
func (t *observedTool) UnmarshalInput(raw json.RawMessage) (any, error) {
	return t.next.UnmarshalInput(raw)
}
func (t *observedTool) IsEnabled(ctx tools.Context) bool { return t.next.IsEnabled(ctx) }
func (t *observedTool) IsReadOnly(input any) bool        { return t.next.IsReadOnly(input) }
func (t *observedTool) IsConcurrencySafe(input any) bool { return t.next.IsConcurrencySafe(input) }
func (t *observedTool) IsDestructive(input any) bool     { return t.next.IsDestructive(input) }
func (t *observedTool) CheckPermissions(ctx tools.Context, input any) tools.PermissionResult {
	return t.next.CheckPermissions(ctx, input)
}

func (t *observedTool) Call(ctx tools.Context, input any, progress chan<- tools.ProgressEvent) (tools.Result, error) {
	start := time.Now()
	res, err := t.next.Call(ctx, input, progress)
	dur := time.Since(start)
	t.meter.RecordToolCall(t.next.Name(), dur, err)
	t.bridge.RecordToolCall(t.next.Name(), dur, err)
	return res, err
}

func (t *observedTool) Render(input any, result tools.Result) tools.RenderHints {
	return t.next.Render(input, result)
}

// Preserve prompt-aware tools through decoration.
func (t *observedTool) SetPermissionPrompt(prompt permissions.PromptFunc) {
	if aware, ok := t.next.(interface{ SetPermissionPrompt(permissions.PromptFunc) }); ok {
		aware.SetPermissionPrompt(prompt)
	}
}
