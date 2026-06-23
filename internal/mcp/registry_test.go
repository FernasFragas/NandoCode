package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

func TestOverlayRegistryLookupBuiltIn(t *testing.T) {
	t.Parallel()
	base := tools.NewRegistry()
	builtin := fakeTool("bash")
	if err := base.Register(builtin); err != nil {
		t.Fatal(err)
	}
	overlay := NewOverlayRegistry(base, nil)
	got, ok := overlay.Lookup("bash")
	if !ok || got == nil || got.Name() != "bash" {
		t.Fatalf("expected built-in tool lookup, got ok=%t tool=%v", ok, got)
	}
}

func TestOverlayRegistryLookupMCP(t *testing.T) {
	t.Parallel()
	base := tools.NewRegistry()
	mcpTool := fakeTool("mcp__fs__list")
	overlay := NewOverlayRegistry(base, []tools.Tool{mcpTool})
	got, ok := overlay.Lookup("mcp__fs__list")
	if !ok || got == nil || got.Name() != "mcp__fs__list" {
		t.Fatalf("expected mcp tool lookup, got ok=%t tool=%v", ok, got)
	}
}

func TestOverlayRegistryLookupUnknown(t *testing.T) {
	t.Parallel()
	overlay := NewOverlayRegistry(tools.NewRegistry(), nil)
	if got, ok := overlay.Lookup("missing"); ok || got != nil {
		t.Fatalf("expected missing lookup")
	}
}

func TestOverlayRegistryListIncludesBuiltInAndMCP(t *testing.T) {
	t.Parallel()
	base := tools.NewRegistry()
	if err := base.Register(fakeTool("bash")); err != nil {
		t.Fatal(err)
	}
	overlay := NewOverlayRegistry(base, []tools.Tool{fakeTool("mcp__fs__read_file")})
	list := overlay.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(list))
	}
	if list[0].Name() != "bash" || list[1].Name() != "mcp__fs__read_file" {
		t.Fatalf("unexpected tool order/list: %q, %q", list[0].Name(), list[1].Name())
	}
}

func TestOverlayRegistryCollisionWarningAndBuiltInWins(t *testing.T) {
	t.Parallel()
	base := tools.NewRegistry()
	if err := base.Register(fakeTool("mcp__fs__read_file")); err != nil {
		t.Fatal(err)
	}
	overlay := NewOverlayRegistry(base, []tools.Tool{fakeTool("mcp__fs__read_file")})
	warnings := overlay.Warnings()
	if len(warnings) == 0 {
		t.Fatalf("expected collision warning")
	}
	if !strings.Contains(warnings[0], "collides") {
		t.Fatalf("unexpected warning: %q", warnings[0])
	}
	got, ok := overlay.Lookup("mcp__fs__read_file")
	if !ok || got == nil || got.Name() != "mcp__fs__read_file" {
		t.Fatalf("expected built-in to remain available")
	}
}

func fakeTool(name string) tools.Tool {
	return tools.BuildTool(tools.Spec{
		Name:        name,
		Description: "Utility tool.",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		Unmarshal: func(json.RawMessage) (any, error) {
			return map[string]any{}, nil
		},
		CallFunc: func(tools.Context, any, chan<- tools.ProgressEvent) (tools.Result, error) {
			return tools.Result{}, nil
		},
		IsEnabledFunc:     func(tools.Context) bool { return true },
		IsReadOnlyFunc:    func(any) bool { return true },
		IsConcurrentFunc:  func(any) bool { return true },
		IsDestructiveFunc: func(any) bool { return false },
		CheckPermFunc: func(_ tools.Context, input any) tools.PermissionResult {
			return tools.PermissionResult{Decision: tools.PermAllow, UpdatedInput: input}
		},
		RenderFunc: func(any, tools.Result) tools.RenderHints {
			return tools.RenderHints{Title: name, Summary: name}
		},
	})
}
